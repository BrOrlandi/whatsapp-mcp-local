// Package panel is the local control page: pair WhatsApp by QR code, check
// the sync against the phone, and connect Claude, step by step, the way the
// hosted v1's panel does.
//
// Its actions run commands and edit client configuration on this machine, so
// every state-changing request must come from the page itself: same-origin,
// never from another site or another localhost app open in the browser.
package panel

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode"

	qrcode "github.com/skip2/go-qrcode"

	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/brand"
	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/index"
	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/localasr"
	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/mcp"
	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/state"
	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/wacli"
)

//go:embed assets
var assets embed.FS

type Panel struct {
	Server     *mcp.Server
	Supervisor *wacli.Supervisor
	Index      *index.Index
	State      *state.State
	Token      string
	// Endpoint is the MCP's address now: it changes when the port does.
	Endpoint func() string
	// Desktop is what Claude Desktop runs to reach the MCP: the command
	// line's bridge subcommand, or the app's bridge program.
	Desktop DesktopCommand
	// LogPath is where the daemon writes its log, shown when sync stops.
	LogPath string
	// MCPProblem says why the MCP is not answering, or nil when it is.
	MCPProblem func() *PortProblem
	// Host is the desktop app around the panel; nil on the command line.
	Host Host

	pages *template.Template
	code  codeCache
}

// PortProblem is the MCP's port taken by another program: WhatsApp keeps
// running, and the MCP waits for a free port.
type PortProblem struct {
	Port int `json:"port"`
	// OtherGateway is set when what holds the port is another WhatsApp MCP,
	// such as the command-line service.
	OtherGateway bool `json:"other_gateway"`
	// Suggest is a free port, already tried, or 0 when none was found.
	Suggest int `json:"suggest,omitempty"`
}

// DesktopCommand is the stdio server entry written into Claude Desktop.
type DesktopCommand struct {
	Command string
	Args    []string
	Env     map[string]string
}

func (p *Panel) endpoint() string { return p.Endpoint() }

type internalKey struct{}

// Internal marks every request as coming from the app's own window, which
// reaches the panel in memory rather than over the network: no other page
// can send a request down that path, and the window's webview sends neither
// Sec-Fetch-Site nor, on a fetch, Origin.
//
// The window's webview does not follow an HTTP redirect from the app, so a
// redirect is answered with a page that navigates there itself.
func Internal(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw := &redirectWriter{ResponseWriter: w}
		next.ServeHTTP(rw, r.WithContext(context.WithValue(r.Context(), internalKey{}, true)))
	})
}

type redirectWriter struct {
	http.ResponseWriter
	redirected bool
}

func (w *redirectWriter) WriteHeader(code int) {
	to := w.Header().Get("Location")
	if code < 300 || code >= 400 || to == "" {
		w.ResponseWriter.WriteHeader(code)
		return
	}
	w.redirected = true
	h := w.Header()
	h.Del("Location")
	h.Del("Content-Length")
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	w.ResponseWriter.WriteHeader(http.StatusOK)
	to = template.HTMLEscapeString(to)
	_, _ = io.WriteString(w.ResponseWriter, `<!doctype html><meta http-equiv="refresh" content="0;url=`+to+`"><a href="`+to+`">…</a>`)
}

func (w *redirectWriter) Write(b []byte) (int, error) {
	if w.redirected {
		return len(b), nil // the redirect's own body
	}
	return w.ResponseWriter.Write(b)
}

func isInternal(r *http.Request) bool {
	v, _ := r.Context().Value(internalKey{}).(bool)
	return v
}

const (
	setupSetting     = "setup_step" // "done" once the first run is over
	recentChats      = 10
	conversationSize = 10
)

func (p *Panel) Register(mux *http.ServeMux) {
	p.pages = parseTemplates()

	mux.HandleFunc("GET /{$}", p.page(p.conectar))
	mux.HandleFunc("GET /instalacao", p.page(p.instalacao))
	mux.HandleFunc("POST /instalacao/avancar", p.form(p.advance))
	mux.HandleFunc("GET /whatsapp", p.page(p.whatsapp))
	mux.HandleFunc("POST /whatsapp/sair", p.form(p.logout))
	mux.HandleFunc("GET /estado", p.page(p.estado))
	mux.HandleFunc("GET /transcricao", p.page(p.transcricao))
	mux.HandleFunc("GET /api/asr", p.api(p.apiASR))
	mux.HandleFunc("POST /api/asr/install", p.api(p.apiASRInstall))
	mux.HandleFunc("GET /documentacao", p.page(p.documentacao))
	mux.HandleFunc("GET /receitas", p.page(p.receitas))
	mux.HandleFunc("POST /conexoes/remover", p.form(p.removeConnection))

	mux.HandleFunc("GET /api/state", p.api(p.apiState))
	mux.HandleFunc("GET /api/chats", p.api(p.apiChats))
	mux.HandleFunc("GET /api/chats/{jid}", p.api(p.apiConversation))
	mux.HandleFunc("POST /api/history", p.api(p.apiHistory))
	mux.HandleFunc("GET /api/pair/qr.png", p.qr)
	mux.HandleFunc("POST /api/pair", p.api(p.apiPair))
	mux.HandleFunc("POST /api/pair/cancel", p.api(p.apiCancelPair))
	mux.HandleFunc("POST /api/clients/{client}", p.api(p.apiAddClient))
	if p.Host != nil {
		p.registerApp(mux)
	}
	registerAssets(mux)
}

// registerAssets serves the scripts and icons every page loads.
func registerAssets(mux *http.ServeMux) {
	sub, _ := fs.Sub(assets, "assets")
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(sub))))
	icon := func(ctype string, body []byte) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", ctype)
			w.Header().Set("Cache-Control", "public, max-age=86400")
			_, _ = w.Write(body)
		}
	}
	mux.HandleFunc("GET /favicon.svg", icon("image/svg+xml", brand.FaviconSVG()))
	mux.HandleFunc("GET /favicon.ico", icon("image/x-icon", brand.FaviconICO()))
	mux.HandleFunc("GET /apple-touch-icon.png", icon("image/png", brand.AppleTouchIcon()))
}

func parseTemplates() *template.Template {
	return template.Must(template.New("panel").Funcs(funcs()).Parse(pageSource))
}

// ---- plumbing ----

// sameOrigin reports whether a request was made by this page. Browsers mark
// every fetch and form post with Sec-Fetch-Site and send Origin on POST; a
// request from any other site, or from another app on another localhost port,
// fails one of the two. A POST carrying neither is not from a browser page of
// ours, so it is refused too.
func sameOrigin(r *http.Request) bool {
	if isInternal(r) {
		return true
	}
	site := r.Header.Get("Sec-Fetch-Site")
	origin := r.Header.Get("Origin")
	if site != "" && site != "same-origin" && !(site == "none" && r.Method == http.MethodGet) {
		return false
	}
	if origin != "" && origin != "http://"+r.Host {
		return false
	}
	if r.Method == http.MethodPost && site == "" && origin == "" {
		return false
	}
	return true
}

type userError struct{ msg string }

func (e userError) Error() string { return e.msg }

func (p *Panel) page(f func(http.ResponseWriter, *http.Request) (string, any)) http.HandlerFunc {
	// A page is a plain navigation, which may come from anywhere (the
	// installer opening the browser, a link in the README): another origin can
	// neither read nor frame it, so only the requests that act are guarded.
	return func(w http.ResponseWriter, r *http.Request) {
		name, data := f(w, r)
		if name == "" {
			return // f answered already, with a redirect
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'; frame-ancestors 'none'")
		// Render first, so a template error is a clean 500 rather than half a
		// page followed by an error.
		var body bytes.Buffer
		if err := p.pages.ExecuteTemplate(&body, name, data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(body.Bytes())
	}
}

// form handles a plain HTML form post and redirects to where f says.
func (p *Panel) form(f func(*http.Request) (string, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !sameOrigin(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		to, err := f(r)
		if err != nil {
			sep := "?"
			if strings.Contains(to, "?") {
				sep = "&"
			}
			to += sep + "erro=" + url.QueryEscape(err.Error())
		}
		http.Redirect(w, r, to, http.StatusSeeOther)
	}
}

func (p *Panel) api(f func(*http.Request) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if !sameOrigin(r) {
			http.Error(w, "this API answers only its own page", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		v, err := f(r)
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			status := http.StatusInternalServerError
			var ue userError
			if errors.As(err, &ue) {
				status = http.StatusConflict
			}
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(v)
	}
}

// ---- shared page data ----

type layout struct {
	Title      string
	Active     string
	HealthTone string
	Error      string
	OK         string
	// App is set inside the desktop app, whose window has no address bar and
	// opens links in the browser.
	App        bool
	MCPProblem *PortProblem
	Settings   HostSettings
	Update     UpdateState
}

// snapshot is what most pages need about the account, gathered once.
type snapshot struct {
	Account   wacli.Account
	Name      string
	Phone     string
	Sync      wacli.Status
	SyncTone  string
	SyncLabel string
	Health    mcp.Health
	Checks    []check
	Activity  index.Activity
	Coverage  index.Coverage
	Gaps      []index.Gap
	Pairing   wacli.Pairing
	History   *mcp.HistoryJob
	// Arriving says the phone is still sending the history it sends after
	// pairing, with how many messages have come in so far.
	Arriving      bool
	ArrivingCount int64
	Endpoint      string
	LogPath       string
	InApp         bool
}

type check struct {
	Name   string
	Status string
	Title  string
	Text   string
}

func (p *Panel) snapshot(ctx context.Context) snapshot {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	s := snapshot{Endpoint: p.endpoint(), LogPath: p.LogPath, InApp: p.Host != nil, Pairing: p.Supervisor.Pairing(), History: p.Server.HistoryStatus()}
	s.Account, _ = p.Supervisor.Account(ctx)
	s.Sync = p.Supervisor.Status()
	s.Health = p.Server.Health(ctx, 0)
	s.Activity = s.Health.Activity
	s.Coverage, _ = p.Index.Coverage(ctx)
	s.Gaps, _ = p.Index.Gaps(ctx, 10*time.Hour, 7*24*time.Hour, 20)
	if s.Account.Authenticated {
		s.Name = p.Index.OwnName(ctx, s.Account.JID)
		s.Phone = formatPhone(s.Account.Phone)
	}
	s.SyncTone, s.SyncLabel = syncLabel(s.Sync)
	if s.Pairing.State == "syncing" || (!s.Sync.History.LastAt.IsZero() && time.Since(s.Sync.History.LastAt) < 90*time.Second) {
		s.Arriving = true
		s.ArrivingCount = s.Pairing.Synced + s.Sync.History.Messages
	}
	for _, c := range s.Health.Checks {
		title, text := describeCheck(c, s)
		s.Checks = append(s.Checks, check{Name: c.Name, Status: c.Status, Title: title, Text: text})
	}
	return s
}

func (p *Panel) layout(r *http.Request, title, active string, s snapshot) layout {
	l := layout{Title: title, Active: active, HealthTone: s.Health.Status, Error: r.URL.Query().Get("erro"), OK: r.URL.Query().Get("ok")}
	if p.MCPProblem != nil {
		l.MCPProblem = p.MCPProblem()
	}
	if p.Host != nil {
		l.App = true
		l.Settings = p.Host.Settings()
		l.Update = p.Host.Update()
	}
	return l
}

func syncLabel(s wacli.Status) (tone, label string) {
	switch s.State {
	case "connected":
		return "ok", "Conectado"
	case "paused":
		if s.PausedFor == "pairing" {
			return "warn", "Conectando"
		}
		return "warn", "Pausado por instantes"
	case "starting":
		return "warn", "Iniciando"
	case "reconnecting":
		return "warn", "Reconectando"
	case "not_paired":
		return "off", "Não conectado"
	case "logged_out":
		return "off", "Desconectado pelo WhatsApp"
	}
	return "off", "Parado"
}

// describeCheck says each health check in Portuguese, from the report's own
// data, so the panel does not show the model-facing English.
func describeCheck(c mcp.Check, s snapshot) (string, string) {
	a := s.Activity
	switch c.Name {
	case "daemon":
		return "WhatsApp MCP", "Funcionando neste computador."
	case "wacli":
		if s.InApp {
			return "Componente do WhatsApp", "A parte do app que conecta ao WhatsApp não respondeu. Reinstale o WhatsApp MCP."
		}
		return "Componente do WhatsApp", "O wacli não respondeu. Instale com brew install openclaw/tap/wacli."
	case "paired":
		if c.Status == "ok" {
			return "WhatsApp pareado", "Este computador é um dispositivo conectado da sua conta."
		}
		return "WhatsApp pareado", "Nenhuma conta conectada. Conecte o seu WhatsApp pelo QR code."
	case "sync":
		switch s.Sync.State {
		case "connected":
			return "Conexão", "Conectado ao WhatsApp e recebendo em tempo real."
		case "paused":
			return "Conexão", "Pausada por alguns segundos para " + pauseReason(s.Sync.PausedFor) + ". Volta sozinha."
		case "starting":
			return "Conexão", "Iniciando."
		case "reconnecting":
			return "Conexão", "Reconectando ao WhatsApp. Confira a internet deste computador."
		case "not_paired":
			return "Conexão", "Aguardando o WhatsApp ser conectado."
		case "logged_out":
			return "Conexão", "O WhatsApp desconectou este computador. Conecte de novo pelo QR code."
		}
		text := "Parada"
		if s.Sync.LastError != "" {
			text += ": " + s.Sync.LastError
		}
		if s.LogPath != "" {
			text += ". Veja o log em " + s.LogPath
		}
		return "Conexão", text + "."
	case "receiving":
		if a.NewestIncoming == nil {
			return "Recebendo mensagens", "Nenhuma mensagem guardada ainda. A primeira sincronização pode estar em andamento."
		}
		text := fmt.Sprintf("Última mensagem recebida %s; %d na última hora, %d em 24 horas.", relativeSince(*a.NewestIncoming), a.LastHour, a.LastDay)
		switch c.Status {
		case "warn":
			text += " Mais quieto que o normal: comum de madrugada. Para ter certeza, peça para alguém mandar uma mensagem."
		case "fail":
			text += " Um dia inteiro sem mensagens costuma indicar que não está recebendo."
		}
		return "Recebendo mensagens", text
	case "coverage":
		if c.Status == "ok" {
			return "Cobertura", "Sem lacunas nos últimos 7 dias."
		}
		return "Cobertura", "Houve períodos nos últimos 7 dias sem mensagem em nenhuma conversa, o rastro de um computador desligado."
	case "history_request":
		return "Pedido de histórico", "O último pedido de histórico falhou em algumas conversas. O celular precisa estar com internet."
	}
	return c.Name, c.Detail
}

func pauseReason(r string) string {
	if strings.HasPrefix(r, "history backfill") {
		return "buscar histórico antigo no celular"
	}
	reasons := map[string]string{"revoke message": "apagar uma mensagem", "check numbers": "verificar números", "profile picture": "buscar uma foto de perfil",
		"group info": "atualizar um grupo", "logout": "desconectar", "pairing": "conectar o WhatsApp",
		"archive chat": "arquivar uma conversa", "unarchive chat": "desarquivar uma conversa", "pin chat": "fixar uma conversa",
		"unpin chat": "desafixar uma conversa", "mute chat": "silenciar uma conversa", "unmute chat": "reativar uma conversa"}
	if v, ok := reasons[r]; ok {
		return v
	}
	return "uma operação"
}

// ---- the setup flow ----

type wizardStep struct {
	Number int
	Label  string
	State  string // done, now, todo
}

// setupStep is where the first run stands: 1 until WhatsApp is paired, 2
// while a client is connected, 0 once it is over. The history keeps arriving
// in the background meanwhile; it does not need a step of its own.
func (p *Panel) setupStep(ctx context.Context, s snapshot) int {
	if !s.Account.Authenticated {
		return 1
	}
	if v, _ := p.State.Setting(ctx, setupSetting); v == "done" {
		return 0
	}
	return 2
}

func steps(now int) []wizardStep {
	labels := []string{"WhatsApp", "Claude"}
	out := make([]wizardStep, len(labels))
	for i, l := range labels {
		st := "todo"
		switch {
		case i+1 < now:
			st = "done"
		case i+1 == now:
			st = "now"
		}
		out[i] = wizardStep{Number: i + 1, Label: l, State: st}
	}
	return out
}

func (p *Panel) instalacao(w http.ResponseWriter, r *http.Request) (string, any) {
	s := p.snapshot(r.Context())
	step := p.setupStep(r.Context(), s)
	if step == 0 {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return "", nil
	}
	set := p.setup(r.Context())
	conns := p.connections(r.Context(), set)
	live := 0
	for _, c := range conns {
		if c.Live {
			live++
		}
	}
	l := p.layout(r, "Instalação", "", s)
	// Setting up is not a problem to be alarmed about.
	l.HealthTone = ""
	return "instalacao", struct {
		layout
		snapshot
		Step         int
		Steps        []wizardStep
		Setup        setup
		Connections  []Connection
		LiveCount    int
		Verification string
	}{l, s, step, steps(step), set, conns, live, verificationPrompt}
}

func (p *Panel) advance(r *http.Request) (string, error) {
	if r.FormValue("to") == "done" {
		return "/", p.State.SetSetting(r.Context(), setupSetting, "done")
	}
	return "/instalacao", nil
}

// ---- pages ----

func (p *Panel) conectar(w http.ResponseWriter, r *http.Request) (string, any) {
	s := p.snapshot(r.Context())
	if p.setupStep(r.Context(), s) != 0 {
		http.Redirect(w, r, "/instalacao", http.StatusSeeOther)
		return "", nil
	}
	set := p.setup(r.Context())
	conns := p.connections(r.Context(), set)
	live := 0
	var last time.Time
	for _, c := range conns {
		if c.Live {
			live++
			if c.LastUsed.After(last) {
				last = c.LastUsed
			}
		}
	}
	return "conectar", struct {
		layout
		snapshot
		Setup       setup
		Connections []Connection
		LiveCount   int
		LastUse     time.Time
		Prompts     []string
	}{p.layout(r, "Conectar", "conectar", s), s, set, conns, live, last, suggestedPrompts}
}

func (p *Panel) whatsapp(w http.ResponseWriter, r *http.Request) (string, any) {
	s := p.snapshot(r.Context())
	if !s.Account.Authenticated {
		http.Redirect(w, r, "/instalacao", http.StatusSeeOther)
		return "", nil
	}
	return "whatsapp", struct {
		layout
		snapshot
	}{p.layout(r, "WhatsApp", "whatsapp", s), s}
}

func (p *Panel) estado(w http.ResponseWriter, r *http.Request) (string, any) {
	s := p.snapshot(r.Context())
	return "estado", struct {
		layout
		snapshot
	}{p.layout(r, "Estado", "estado", s), s}
}

func (p *Panel) transcricao(w http.ResponseWriter, r *http.Request) (string, any) {
	s := p.snapshot(r.Context())
	var local localasr.Status
	if asr := p.Server.ASR(); asr != nil {
		local = asr.Status()
	}
	total, corrected, _ := p.State.TranscriptCount(r.Context())
	return "transcricao", struct {
		layout
		Local     localasr.Status
		Total     int
		Corrected int
	}{p.layout(r, "Transcrição de áudios", "transcricao", s), local, total, corrected}
}

func (p *Panel) apiASR(r *http.Request) (any, error) {
	asr := p.Server.ASR()
	if asr == nil {
		return localasr.Status{}, nil
	}
	return map[string]any{"status": asr.Status()}, nil
}

func (p *Panel) apiASRInstall(r *http.Request) (any, error) {
	asr := p.Server.ASR()
	if asr == nil {
		return nil, userError{"a transcrição local não está disponível"}
	}
	if err := asr.StartInstall(); err != nil && !errors.Is(err, localasr.ErrInstalling) {
		return nil, err
	}
	return asr.Status(), nil
}

func (p *Panel) documentacao(w http.ResponseWriter, r *http.Request) (string, any) {
	s := p.snapshot(r.Context())
	tools := mcp.Catalogue()
	return "documentacao", struct {
		layout
		Tools []mcp.Tool
		Count int
	}{p.layout(r, "Documentação", "documentacao", s), tools, len(tools)}
}

func (p *Panel) receitas(w http.ResponseWriter, r *http.Request) (string, any) {
	s := p.snapshot(r.Context())
	return "receitas", struct {
		layout
		Recipes []recipe
	}{p.layout(r, "Receitas", "receitas", s), recipeBook()}
}

func (p *Panel) logout(r *http.Request) (string, error) {
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	if err := p.Supervisor.Logout(ctx); err != nil {
		return "/whatsapp", err
	}
	_ = p.State.SetSetting(r.Context(), setupSetting, "")
	return "/instalacao", nil
}

func (p *Panel) removeConnection(r *http.Request) (string, error) {
	key := r.FormValue("client")
	var err error
	switch {
	case key == "claude-desktop":
		err = p.removeClaudeDesktop()
		_ = p.State.ForgetClient(r.Context(), "claude-ai")
	case key == "claude-code":
		err = p.removeClaudeCode(r.Context())
		_ = p.State.ForgetClient(r.Context(), "claude-code")
	case strings.HasPrefix(key, "client:"):
		err = p.State.ForgetClient(r.Context(), strings.TrimPrefix(key, "client:"))
	}
	if err != nil {
		return "/", err
	}
	return "/", nil
}

// ---- API ----

func (p *Panel) apiState(r *http.Request) (any, error) {
	s := p.snapshot(r.Context())
	set := setup{Desktop: p.desktopInfo(), Code: p.code.info}
	live := 0
	for _, c := range p.connections(r.Context(), set) {
		if c.Live {
			live++
		}
	}
	sync := s.Sync
	sync.Recent = nil
	return map[string]any{
		"arriving": s.Arriving, "arriving_count": s.ArrivingCount,
		"account": s.Account, "name": s.Name, "phone": s.Phone,
		"sync": sync, "pairing": s.Pairing, "health": s.Health,
		"history": s.History, "clients_live": live, "endpoint": p.endpoint(), "version": mcp.Version,
	}, nil
}

func (p *Panel) apiChats(r *http.Request) (any, error) {
	chats, err := p.Index.RecentChats(r.Context(), recentChats)
	if err != nil {
		return nil, err
	}
	if chats == nil {
		chats = []index.ChatPreview{}
	}
	return map[string]any{"chats": chats, "history": p.Server.HistoryStatus()}, nil
}

func (p *Panel) apiConversation(r *http.Request) (any, error) {
	jid := r.PathValue("jid")
	msgs, err := p.Index.Conversation(r.Context(), jid, conversationSize)
	if err != nil {
		return nil, err
	}
	type bubble struct {
		index.Bubble
		Transcript string `json:"transcript,omitempty"`
		Corrected  bool   `json:"transcript_corrected,omitempty"`
	}
	out := make([]bubble, len(msgs))
	for i, m := range msgs {
		out[i] = bubble{Bubble: m}
		if m.Media == "audio" {
			if t, err := p.State.Transcript(r.Context(), jid, m.ID); err == nil {
				out[i].Transcript = t.Text
				out[i].Corrected = t.Raw != "" && t.Raw != t.Text
			}
		}
	}
	oldest, _ := p.Index.ChatOldest(r.Context(), jid)
	return map[string]any{"messages": out, "oldest": oldest}, nil
}

func (p *Panel) apiHistory(r *http.Request) (any, error) {
	var body struct {
		ChatJID string `json:"chat_jid"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	rounds := 1
	if body.ChatJID != "" {
		rounds = 3
	}
	job, started, err := p.Server.RequestHistory(r.Context(), body.ChatJID, 50, rounds, recentChats)
	if err != nil {
		return nil, userError{"não há de onde partir: a busca começa pela mensagem mais antiga que este computador já tem"}
	}
	return map[string]any{"started": started, "job": job}, nil
}

func (p *Panel) apiPair(r *http.Request) (any, error) {
	var body struct {
		Phone string `json:"phone"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	account, _ := p.Supervisor.Account(r.Context())
	if account.Authenticated {
		return nil, userError{"este computador já está conectado a um WhatsApp; desconecte antes de conectar outro"}
	}
	phone := strings.Map(func(r rune) rune {
		if unicode.IsDigit(r) {
			return r
		}
		return -1
	}, body.Phone)
	if body.Phone != "" && (len(phone) < 8 || len(phone) > 15) {
		return nil, userError{"informe o número com DDI e DDD, por exemplo +55 11 91234-5678"}
	}
	if err := p.Supervisor.StartPairing(phone); err != nil && !errors.Is(err, wacli.ErrPairingRunning) {
		return nil, err
	}
	return p.Supervisor.Pairing(), nil
}

func (p *Panel) apiCancelPair(r *http.Request) (any, error) {
	p.Supervisor.CancelPairing()
	return p.Supervisor.Pairing(), nil
}

func (p *Panel) apiAddClient(r *http.Request) (any, error) {
	var body struct {
		Replace bool `json:"replace"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	var err error
	switch r.PathValue("client") {
	case "claude-code":
		err = p.addClaudeCode(r.Context(), body.Replace)
	case "claude-desktop":
		err = p.addClaudeDesktop(body.Replace)
	default:
		return nil, userError{"cliente desconhecido"}
	}
	var conflict errConflict
	if errors.As(err, &conflict) {
		// Not a failure: a question for the person, which the page asks.
		return map[string]any{"ok": false, "conflict": conflict.other, "client": conflict.client}, nil
	}
	if err != nil {
		return nil, err
	}
	return map[string]bool{"ok": true}, nil
}

func (p *Panel) qr(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	pairing := p.Supervisor.Pairing()
	if pairing.State != "qr" || pairing.QR == "" {
		http.Error(w, "no QR code right now", http.StatusNotFound)
		return
	}
	png, err := qrcode.Encode(pairing.QR, qrcode.Medium, 520)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(png)
}

// ---- template helpers ----

func funcs() template.FuncMap {
	return template.FuncMap{
		"css":           func() template.CSS { return template.CSS(stylesheet) },
		"logo":          brand.LogoSVG,
		"author":        func() string { return brand.Author },
		"authorURL":     func() string { return brand.AuthorURL },
		"repositoryURL": func() string { return brand.RepositoryURL },
		"supportURL":    func() string { return brand.SupportURL },
		"version":       func() string { return strings.TrimPrefix(mcp.Version, "v") },
		"serverName":    func() string { return ServerName },
		"tray":          trayName,
		"accel": func(a string) string {
			switch a {
			case "metal":
				return "na GPU do seu Mac"
			case "cuda":
				return "na placa de vídeo NVIDIA"
			}
			return "no processador"
		},
		"baseOf":        func(endpoint string) string { return strings.TrimSuffix(endpoint, "/mcp") },
		"relativeSince": relativeSinceAny,
		"moment":        momentAny,
		"count":         countFormat,
		"plural": func(n int, one, many string) string {
			if n == 1 {
				return "1 " + one
			}
			return strconv.Itoa(n) + " " + many
		},
		"initial": func(s string) string {
			for _, r := range s {
				if unicode.IsLetter(r) || unicode.IsDigit(r) {
					return strings.ToUpper(string(r))
				}
			}
			return "W"
		},
		"hours": func(h float64) string {
			if h >= 48 {
				return fmt.Sprintf("%.0f dias", h/24)
			}
			return fmt.Sprintf("%.0f horas", h)
		},
	}
}

// trayName is what each system calls the place the app's icon lives.
func trayName() string {
	switch runtime.GOOS {
	case "darwin":
		return "barra de menus"
	case "windows":
		return "área de notificação"
	}
	return "bandeja do sistema"
}

func timeOf(v any) (time.Time, bool) {
	switch t := v.(type) {
	case time.Time:
		return t, !t.IsZero()
	case *time.Time:
		if t == nil {
			return time.Time{}, false
		}
		return *t, !t.IsZero()
	}
	return time.Time{}, false
}

func relativeSinceAny(v any) string {
	t, ok := timeOf(v)
	if !ok {
		return "—"
	}
	return relativeSince(t)
}

func relativeSince(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "agora"
	case d < time.Hour:
		return fmt.Sprintf("há %d min", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("há %d h", int(d.Hours()))
	}
	return fmt.Sprintf("há %d dias", int(d.Hours()/24))
}

func momentAny(v any) string {
	t, ok := timeOf(v)
	if !ok {
		return "—"
	}
	return t.Local().Format("02/01/2006 15:04")
}

func countFormat(v any) string {
	var n int64
	switch x := v.(type) {
	case int:
		n = int64(x)
	case int64:
		n = x
	}
	s := strconv.FormatInt(n, 10)
	var out []byte
	for i := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, '.')
		}
		out = append(out, s[i])
	}
	return string(out)
}

// formatPhone writes a number the way it is read in Brazil, and leaves others
// in international form.
func formatPhone(p string) string {
	d := strings.Map(func(r rune) rune {
		if unicode.IsDigit(r) {
			return r
		}
		return -1
	}, p)
	if d == "" {
		return ""
	}
	if strings.HasPrefix(d, "55") && (len(d) == 12 || len(d) == 13) {
		local := d[4:]
		return fmt.Sprintf("+55 (%s) %s-%s", d[2:4], local[:len(local)-4], local[len(local)-4:])
	}
	return "+" + d
}
