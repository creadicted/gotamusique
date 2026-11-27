// Package web serves the bot's HTTP API (under /api/v1) and, later, its UI.
//
// It depends only on the narrow Bot interface below and on the plain data types
// in internal/status, never on the bot package, so handlers are tested with a
// fake bot and a front end other than the web one can describe the bot the same way.
package web

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/konradk/gotamusique/internal/config"
	"github.com/konradk/gotamusique/internal/status"
)

// Bot is what the web layer needs from the bot.
type Bot interface {
	Snapshot() status.Snapshot
}

// Options configure a Server.
type Options struct {
	// Addr is the listen address, host:port.
	Addr string
	// AccessAddress is the public URL users open. An https:// URL makes the
	// session cookie Secure; the host is also accepted in the Host header.
	AccessAddress string
	// AuthMethod is config.AuthToken or config.AuthNone.
	AuthMethod string
	Log        *slog.Logger
}

// Server is the HTTP front end.
type Server struct {
	bot  Bot
	auth *Auth
	opts Options
	log  *slog.Logger

	secureCookie bool
	allowedHosts map[string]bool // only enforced without login, see hostGuard
	handler      http.Handler
}

// NewServer builds a Server. auth must be non-nil when AuthMethod is token.
func NewServer(bot Bot, auth *Auth, o Options) *Server {
	if o.Log == nil {
		o.Log = slog.New(slog.DiscardHandler)
	}
	if o.AuthMethod == "" {
		o.AuthMethod = config.AuthToken
	}
	s := &Server{
		bot: bot, auth: auth, opts: o, log: o.Log,
		secureCookie: strings.HasPrefix(strings.ToLower(o.AccessAddress), "https://"),
		allowedHosts: allowedHosts(o),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /login", s.handleLogin)
	mux.HandleFunc("POST /api/v1/auth/logout", s.handleLogout)
	mux.HandleFunc("GET /api/v1/state", s.require(RoleUser, s.handleState))
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "no such endpoint")
	})
	s.handler = s.guard(mux)
	return s
}

// Handler returns the HTTP handler, for tests and for embedding.
func (s *Server) Handler() http.Handler { return s.handler }

// ListenAndServe listens on the configured address and serves until ctx is
// cancelled, then shuts down gracefully.
func (s *Server) ListenAndServe(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.opts.Addr)
	if err != nil {
		return err
	}
	return s.Serve(ctx, ln)
}

// Serve serves on ln until ctx is cancelled.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	s.warnAboutExposure(ln.Addr().String())
	srv := &http.Server{
		Handler:           s.handler,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	s.log.Info("web interface listening", slog.String("addr", ln.Addr().String()),
		slog.String("auth", s.opts.AuthMethod))

	done := make(chan struct{})
	go func() {
		defer close(done)
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			srv.Close() //nolint:errcheck
		}
	}()
	err := srv.Serve(ln)
	if errors.Is(err, http.ErrServerClosed) {
		<-done
		return nil
	}
	return err
}

// warnAboutExposure logs the two setups that are easy to get wrong.
func (s *Server) warnAboutExposure(listen string) {
	host, _, _ := net.SplitHostPort(listen)
	loopback := isLoopbackHost(host)
	if s.opts.AuthMethod == config.AuthNone && !loopback {
		s.log.Warn("web interface has no login and listens beyond loopback: anyone who can reach it controls the bot",
			slog.String("addr", listen))
	}
	if s.opts.AuthMethod == config.AuthToken && !loopback && isLoopbackURL(s.opts.AccessAddress) {
		s.log.Warn("web interface listens beyond loopback but access_address is still a loopback URL: " +
			"the links sent by !web will not work for other machines")
	}
}

// guard wraps every response with security headers and, when there is no login,
// the Host check.
func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
		if !s.hostAllowed(r.Host) {
			writeError(w, http.StatusMisdirectedRequest, "bad_host",
				"unexpected Host header; set access_address to the address you use")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// hostAllowed defends the login-less mode against DNS rebinding: a hostile page
// can make its own name resolve to the bot's address, and without a cookie
// requirement the browser would let it drive the API. With token auth the
// session cookie is host-scoped, so that attack gets nothing and any Host is fine.
func (s *Server) hostAllowed(host string) bool {
	if s.opts.AuthMethod != config.AuthNone {
		return true
	}
	return s.allowedHosts[strings.ToLower(host)]
}

func allowedHosts(o Options) map[string]bool {
	set := map[string]bool{}
	add := func(h string) {
		if h != "" {
			set[strings.ToLower(h)] = true
		}
	}
	if u, err := url.Parse(o.AccessAddress); err == nil {
		add(u.Host)
	}
	add(o.Addr)
	if host, port, err := net.SplitHostPort(o.Addr); err == nil {
		for _, h := range []string{"localhost", "127.0.0.1", "[::1]"} {
			add(h + ":" + port)
		}
		if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
			add(host + ":" + port)
		}
	}
	return set
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

func isLoopbackURL(raw string) bool {
	u, err := url.Parse(raw)
	return err != nil || isLoopbackHost(u.Hostname())
}

// ---- JSON helpers ----

type errorBody struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v) //nolint:errcheck // the client went away
}

// writeJSONKeepCache is writeJSON without overriding the caller's Cache-Control.
func writeJSONKeepCache(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v) //nolint:errcheck // the client went away
}

func writeError(w http.ResponseWriter, code int, slug, msg string) {
	writeJSON(w, code, errorBody{Error: msg, Code: slug})
}
