package panel

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"html/template"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/caller"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/httpserver"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/i18n"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/public"
)

// With WHATSAPP_MCP_TOKEN set, the token keeps other programs on this
// computer from acting through the local listener. Headers cannot tell a
// browser from such a program (anything can send Origin and
// Sec-Fetch-Site), so an action there must prove the token: a program
// sends it as a Bearer, and a browser gets a session cookie once the token
// is typed on TokenPath. Reads stay open, as docs/api.md says, and the app's
// own window, which reaches the panel in memory, needs neither.

// TokenPath is the local page where the token is typed.
const TokenPath = "/token"

const (
	localCookie     = "whatsapp_mcp_local"
	localSessionAge = 30 * 24 * time.Hour
)

// needsToken reports whether the request must prove the token to act: a
// token is set and the caller is a program or browser on this computer, not
// the app's window.
func (p *Panel) needsToken(r *http.Request) bool {
	return p.Token != "" && caller.From(r.Context()).Kind == caller.Local && !isInternal(r)
}

// tokenProved reports whether the request carries the token, as a Bearer,
// or a session made by typing it; true when none is needed.
func (p *Panel) tokenProved(r *http.Request) bool {
	if !p.needsToken(r) {
		return true
	}
	if got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok &&
		subtle.ConstantTimeCompare([]byte(strings.TrimSpace(got)), []byte(p.Token)) == 1 {
		return true
	}
	c, err := r.Cookie(localCookieName(r))
	return err == nil && p.validLocalSession(c.Value, time.Now())
}

// mayAct reports whether the request may change something: a read always
// may, an action only with the token proved.
func (p *Panel) mayAct(r *http.Request) bool {
	return r.Method == http.MethodGet || r.Method == http.MethodHead || p.tokenProved(r)
}

// fromOwnPage reports whether a request comes from the panel's own page and,
// when it acts, carries the token that a token asks for.
func (p *Panel) fromOwnPage(r *http.Request) bool { return sameOrigin(r) && p.mayAct(r) }

// sessionKey signs the local sessions; a new token ends them all.
func (p *Panel) sessionKey() []byte {
	sum := sha256.Sum256([]byte("whatsapp-mcp local panel session\x00" + p.Token))
	return sum[:]
}

// newLocalSession is a session cookie's value: when it was made, a random
// part, and their HMAC under the session key. It holds nothing of the token.
func (p *Panel) newLocalSession(now time.Time) string {
	nonce := make([]byte, 16)
	_, _ = rand.Read(nonce)
	payload := strconv.FormatInt(now.Unix(), 10) + "." + hex.EncodeToString(nonce)
	mac := hmac.New(sha256.New, p.sessionKey())
	mac.Write([]byte(payload))
	return payload + "." + hex.EncodeToString(mac.Sum(nil))
}

func (p *Panel) validLocalSession(v string, now time.Time) bool {
	i := strings.LastIndexByte(v, '.')
	if i < 0 || p.Token == "" {
		return false
	}
	payload, sig := v[:i], v[i+1:]
	got, err := hex.DecodeString(sig)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, p.sessionKey())
	mac.Write([]byte(payload))
	if !hmac.Equal(got, mac.Sum(nil)) {
		return false
	}
	at, _, _ := strings.Cut(payload, ".")
	made, err := strconv.ParseInt(at, 10, 64)
	if err != nil {
		return false
	}
	age := now.Sub(time.Unix(made, 0))
	return age >= -time.Minute && age < localSessionAge
}

// askForToken sends a browser to the token page, coming back to next.
func askForToken(w http.ResponseWriter, r *http.Request, next string) {
	http.Redirect(w, r, TokenPath+"?next="+url.QueryEscape(public.SafeReturn(next)), http.StatusSeeOther)
}

// returnTo is where a form sent without the token comes back to: the page
// it was posted from, when the browser says.
func returnTo(r *http.Request) string {
	if ref, err := url.Parse(r.Referer()); err == nil && ref.Path != "" {
		return ref.RequestURI()
	}
	return "/"
}

// localGate is what the local token's sign-in keeps between requests: the
// single-use codes a command hands the browser, and the wrong tries.
type localGate struct {
	mu    sync.Mutex
	codes map[string]time.Time // a code's hash, and when it lapses
	fails int
	until time.Time
}

const (
	codeLife       = time.Minute
	tokenFreeTries = 3
	tokenMaxWait   = time.Minute
)

// newCode is a single-use code that signs a browser in, for a minute.
func (g *localGate) newCode(now time.Time) string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	code := hex.EncodeToString(b)
	sum := sha256.Sum256([]byte(code))
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.codes == nil {
		g.codes = map[string]time.Time{}
	}
	for k, until := range g.codes {
		if now.After(until) {
			delete(g.codes, k)
		}
	}
	g.codes[string(sum[:])] = now.Add(codeLife)
	return code
}

// useCode reports whether code is live, and spends it.
func (g *localGate) useCode(code string, now time.Time) bool {
	if code == "" {
		return false
	}
	sum := sha256.Sum256([]byte(code))
	g.mu.Lock()
	defer g.mu.Unlock()
	until, ok := g.codes[string(sum[:])]
	delete(g.codes, string(sum[:]))
	return ok && now.Before(until)
}

// try counts a wrong token: how long the next wrong one must wait, or zero
// when this one was not held back. A wrong one sent while waiting is not
// counted. Every try on this computer comes from the same address, so the
// wait is short, and the right token, never held back, clears it.
func (g *localGate) try(now time.Time) time.Duration {
	g.mu.Lock()
	defer g.mu.Unlock()
	if wait := g.until.Sub(now); wait > 0 {
		return wait
	}
	g.fails++
	if g.fails >= tokenFreeTries {
		g.until = now.Add(min(time.Second<<(g.fails-tokenFreeTries), tokenMaxWait))
	}
	return 0
}

func (g *localGate) ok() {
	g.mu.Lock()
	g.fails, g.until = 0, time.Time{}
	g.mu.Unlock()
}

// SignInURL is the panel's address for a browser opened by a command that
// holds the token: with a single-use code, it lands signed in. POST
// /api/token/codigo, which only the token reaches, hands one out.
const signInCodePath = "/api/token/codigo"

// localCookieName is the session cookie's name for the listener a request
// reached: browsers keep cookies by host, not port, so the app and the
// command line on 127.0.0.1 each keep their own.
func localCookieName(r *http.Request) string {
	if _, port, err := net.SplitHostPort(r.Host); err == nil && port != "" {
		return localCookie + "_" + port
	}
	return localCookie
}

func (p *Panel) setLocalSession(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: localCookieName(r), Value: p.newLocalSession(time.Now()), Path: "/", HttpOnly: true,
		SameSite: http.SameSiteStrictMode, MaxAge: int(localSessionAge / time.Second)})
}

func (p *Panel) registerLocalToken(mux httpserver.Mux) {
	mux.HandleFunc("GET "+TokenPath, func(w http.ResponseWriter, r *http.Request) {
		if callerRefused(w, r) {
			return
		}
		next := public.SafeReturn(r.URL.Query().Get("next"))
		if p.tokenProved(r) {
			http.Redirect(w, r, next, http.StatusSeeOther)
			return
		}
		// A command that holds the token opened this browser with a code.
		if p.local.useCode(r.URL.Query().Get("codigo"), time.Now()) {
			p.setLocalSession(w, r)
			http.Redirect(w, r, next, http.StatusSeeOther)
			return
		}
		p.tokenPage(w, next, "", http.StatusOK)
	})
	mux.HandleFunc("POST "+TokenPath, func(w http.ResponseWriter, r *http.Request) {
		if callerRefused(w, r) {
			return
		}
		if !sameOrigin(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
		next := public.SafeReturn(r.PostFormValue("next"))
		if !p.needsToken(r) {
			http.Redirect(w, r, next, http.StatusSeeOther)
			return
		}
		// The right token always gets in, even while wrong ones wait: a
		// program here guessing cannot keep the owner out.
		typed := strings.TrimSpace(r.PostFormValue("token"))
		if typed == "" || subtle.ConstantTimeCompare([]byte(typed), []byte(p.Token)) != 1 {
			if wait := p.local.try(time.Now()); wait > 0 {
				secs := int(wait.Round(time.Second)/time.Second) + 1
				w.Header().Set("Retry-After", strconv.Itoa(secs))
				p.tokenPage(w, next, i18n.Tf("Muitas tentativas com o token errado. Tente de novo em %d segundos.", secs), http.StatusTooManyRequests)
				return
			}
			p.tokenPage(w, next, i18n.T("Esse não é o token deste computador. Confira e digite de novo."), http.StatusUnauthorized)
			return
		}
		p.local.ok()
		p.setLocalSession(w, r)
		http.Redirect(w, r, next, http.StatusSeeOther)
	})
	mux.HandleFunc("POST "+signInCodePath, p.api(func(r *http.Request) (any, error) {
		if p.Token == "" {
			return map[string]string{"url": "/"}, nil
		}
		return map[string]string{"url": TokenPath + "?codigo=" + p.local.newCode(time.Now())}, nil
	}))
}

// tokenHint says where the person finds the token: the variable it was set
// with, or the file in the data folder.
func (p *Panel) tokenHint() string {
	if os.Getenv("WHATSAPP_MCP_TOKEN") != "" {
		return i18n.T("Ele foi definido na variável WHATSAPP_MCP_TOKEN, quando o WhatsApp MCP foi instalado ou iniciado.")
	}
	if p.DataDir != "" {
		return i18n.Tf("Ele está no arquivo %s. No terminal, whatsapp-mcp token show mostra o token.", filepath.Join(p.DataDir, "token"))
	}
	return i18n.T("No terminal, whatsapp-mcp token show mostra o token.")
}

var tokenTemplate = template.Must(template.New("token").Parse(`<!doctype html>
<html lang="{{.Lang}}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title><link rel="icon" href="/favicon.svg">
<style>
body{margin:0;min-height:100vh;display:grid;place-items:center;font:16px/1.5 system-ui,-apple-system,"Segoe UI",sans-serif;background:#f4f1ea;color:#1f2a24}
main{width:min(380px,calc(100% - 32px));background:#fff;border-radius:14px;padding:28px;box-shadow:0 2px 14px rgba(0,0,0,.08)}
h1{font-size:20px;margin:0 0 4px}p{margin:0 0 18px;color:#5b6660}
label{display:block;font-weight:600;margin-bottom:6px}
input{box-sizing:border-box;width:100%;padding:10px 12px;font:inherit;border:1px solid #c9cfc9;border-radius:8px}
button{margin-top:16px;width:100%;padding:11px;font:inherit;font-weight:600;border:0;border-radius:8px;background:#1f7a4d;color:#fff;cursor:pointer}
.error{background:#fbeed2;color:#6b4a00;border-radius:8px;padding:10px 12px;margin-bottom:14px}
.hint{margin:14px 0 0;font-size:14px}
@media (prefers-color-scheme:dark){body{background:#151a17;color:#e6ebe7}main{background:#1f2622;box-shadow:none}p{color:#a6b0aa}input{background:#151a17;color:inherit;border-color:#3a443e}}
</style></head>
<body><main>
<h1>{{.Title}}</h1><p>{{.Lead}}</p>
{{if .Error}}<div class="error" role="alert">{{.Error}}</div>{{end}}
<form method="post" action="` + TokenPath + `">
<input type="hidden" name="next" value="{{.Return}}">
<label for="token">{{.Label}}</label>
<input id="token" name="token" type="password" autocomplete="current-password" required autofocus>
<button type="submit">{{.Button}}</button>
</form>
<p class="hint">{{.Hint}}</p>
</main></body></html>`))

func (p *Panel) tokenPage(w http.ResponseWriter, next, problem string, status int) {
	var body bytes.Buffer
	_ = tokenTemplate.Execute(&body, map[string]string{
		"Lang": i18n.Lang(), "Title": i18n.T("Entrar no painel"),
		"Lead":  i18n.T("O WhatsApp MCP deste computador foi protegido com um token, uma senha longa. Digite o token para abrir o painel."),
		"Label": i18n.T("Token"), "Button": i18n.T("Entrar"), "Error": problem, "Return": next,
		"Hint": p.tokenHint(),
	})
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Content-Security-Policy", "default-src 'self'; style-src 'unsafe-inline'; frame-ancestors 'none'; form-action 'self'")
	w.WriteHeader(status)
	_, _ = w.Write(body.Bytes())
}
