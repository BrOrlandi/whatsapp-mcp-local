// Package httpserver serves the MCP endpoint over Streamable HTTP on the
// loopback interface only.
//
// Listening on 127.0.0.1 keeps the network out, but not the browser: any page
// open on this machine can send requests to localhost. The Host check stops
// DNS rebinding and the Origin check stops cross-site requests, so only a
// local program, not a web page, can reach the tools. An optional bearer
// token closes the endpoint to other local programs as well.
package httpserver

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-v2/internal/mcp"
)

const maxBody = 1 << 20

type Options struct {
	Addr   string // host:port, loopback
	Token  string // optional bearer token
	Logger *slog.Logger
	// Register adds further routes, such as the control panel.
	Register func(*http.ServeMux)
}

func Handler(server *mcp.Server, opts Options) http.Handler {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			// Streamable HTTP allows a GET for a server-initiated stream; this
			// server never pushes, so it says so instead of holding a connection.
			w.Header().Set("Allow", "POST")
			http.Error(w, "this MCP endpoint accepts POST", http.StatusMethodNotAllowed)
			return
		}
		if opts.Token != "" && !validToken(r, opts.Token) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="whatsapp-mcp-v2"`)
			http.Error(w, "missing or wrong bearer token", http.StatusUnauthorized)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, maxBody+1))
		if err != nil || len(body) > maxBody {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		// Tool calls that pause sync (history, live lookups) can take minutes;
		// the context ends when the client hangs up.
		// The bridge names the client it speaks for; a direct HTTP client is
		// known by its User-Agent.
		hint := r.Header.Get("X-MCP-Client")
		if hint == "" {
			hint = "ua:" + r.UserAgent()
		}
		response := server.Handle(mcp.WithClientHint(r.Context(), hint), body)
		if response == nil {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(response)
	})
	mux.Handle("/media/", server.Links())
	if opts.Register != nil {
		opts.Register(mux)
	}
	// /health is the same report as the health tool, for scripts and curl;
	// it answers 503 when a check fails, so a monitor needs no JSON parsing.
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if opts.Token != "" && !validToken(r, opts.Token) {
			http.Error(w, "missing or wrong bearer token", http.StatusUnauthorized)
			return
		}
		h := server.Health(r.Context(), 0)
		w.Header().Set("Content-Type", "application/json")
		if h.Status == "fail" {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(h)
	})
	// /healthz only says the process is up.
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "ok\n")
	})
	return guard(mux, logger)
}

// guard rejects any request that did not come from a local program.
func guard(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !loopbackHost(r.Host) {
			logger.Warn("rejected request with foreign Host", "host", r.Host)
			http.Error(w, "this server answers only on localhost", http.StatusForbidden)
			return
		}
		// Another localhost app open in the browser is still a foreign origin:
		// the Origin must be this server itself.
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || !loopbackHost(u.Host) || u.Host != r.Host {
				logger.Warn("rejected cross-origin request", "origin", origin)
				http.Error(w, "cross-origin requests are not accepted", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func loopbackHost(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func validToken(r *http.Request, token string) bool {
	got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	return ok && subtle.ConstantTimeCompare([]byte(strings.TrimSpace(got)), []byte(token)) == 1
}

// Serve listens on the loopback address until ctx ends.
func Serve(ctx context.Context, handler http.Handler, addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}
