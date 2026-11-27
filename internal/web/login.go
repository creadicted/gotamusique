package web

import (
	"html"
	"log/slog"
	"net/http"
	"time"

	"github.com/konradk/gotamusique/internal/config"
)

// SessionCookie is the name of the session cookie.
const SessionCookie = "gotamusique_session"

// anonymous is the identity when there is no login.
var anonymous = Identity{Subject: "anonymous", Name: "anonymous", Roles: []Role{RoleUser}}

// identify returns who is calling: the session behind the cookie, or the
// anonymous user when the server has no login.
func (s *Server) identify(r *http.Request) (Identity, bool) {
	if s.opts.AuthMethod == config.AuthNone {
		return anonymous, true
	}
	c, err := r.Cookie(SessionCookie)
	if err != nil || c.Value == "" || s.auth == nil {
		return Identity{}, false
	}
	return s.auth.Lookup(c.Value)
}

// require wraps a handler so it only runs for a caller holding role.
func (s *Server) require(role Role, h func(http.ResponseWriter, *http.Request, Identity)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := s.identify(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthenticated", "log in with the link from !web")
			return
		}
		if !id.Has(role) {
			writeError(w, http.StatusForbidden, "forbidden", "you are not allowed to do that")
			return
		}
		h(w, r, id)
	}
}

func (s *Server) setSessionCookie(w http.ResponseWriter, value string, maxAge time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name: SessionCookie, Value: value, Path: s.cookiePath(),
		MaxAge:   int(maxAge / time.Second),
		HttpOnly: true, Secure: s.secureCookie, SameSite: http.SameSiteLaxMode,
	})
}

// handleLogin consumes the one-time token from a !web link and starts a session.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.opts.AuthMethod == config.AuthNone {
		http.Redirect(w, r, s.root(), http.StatusSeeOther)
		return
	}
	token := r.URL.Query().Get("token")
	if token == "" || s.auth == nil {
		s.loginFailed(w, r, "missing token")
		return
	}
	sessionID, id, err := s.auth.Redeem(token)
	if err != nil {
		s.loginFailed(w, r, err.Error())
		return
	}
	s.setSessionCookie(w, sessionID, DefaultSessionMax)
	s.log.Info("web login", slog.String("user", id.Name), slog.String("subject", id.Subject))
	http.Redirect(w, r, s.root(), http.StatusSeeOther)
}

func (s *Server) loginFailed(w http.ResponseWriter, r *http.Request, why string) {
	s.log.Info("web login refused", slog.String("reason", why), slog.String("remote", r.RemoteAddr))
	writePage(w, http.StatusUnauthorized, "Login failed",
		"This link is invalid, expired or was already used. Type the web command in Mumble again for a new one.")
}

// handleLogout ends the caller's session. It always succeeds.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(SessionCookie); err == nil && s.auth != nil {
		s.auth.Logout(c.Value)
	}
	s.setSessionCookie(w, "", -time.Second)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

func writePage(w http.ResponseWriter, code int, title, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	w.Write([]byte("<!doctype html><meta charset=utf-8><title>" + html.EscapeString(title) + //nolint:errcheck
		"</title><body style=\"font-family:sans-serif;max-width:32em;margin:3em auto\"><h1>" +
		html.EscapeString(title) + "</h1><p>" + body + "</p>"))
}
