package web

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/konradk/gotamusique/internal/config"
	"github.com/konradk/gotamusique/internal/status"
)

func playingSnap() status.Snapshot {
	return status.Snapshot{
		Version: 7,
		Queue: []status.Entry{
			{ID: 3, Title: "[File] a", Duration: 90 * time.Second, Seekable: true},
			{ID: 5, Title: "[Radio] b"},
		},
		Index: 0, Playing: true, Position: 12*time.Second + 400*time.Millisecond, PositionKnown: true,
		Volume: 0.5, Mode: "repeat", Scan: status.Scan{Available: true},
	}
}

type env struct {
	srv  *Server
	auth *Auth
	bot  *fakeBot
	ts   *httptest.Server
	c    *http.Client
}

// newEnv starts a server. The client keeps cookies and does not follow redirects.
func newEnv(t *testing.T, o Options) *env {
	t.Helper()
	e := &env{auth: NewAuth(AuthOptions{}), bot: &fakeBot{snap: playingSnap()}}
	e.srv = NewServer(e.bot, e.auth, o)
	e.ts = httptest.NewServer(e.srv.Handler())
	t.Cleanup(e.ts.Close)
	jar := newJar()
	e.c = &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return e
}

func (e *env) get(t *testing.T, path string, hdr ...string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("GET", e.ts.URL+path, nil)
	for i := 0; i+1 < len(hdr); i += 2 {
		if hdr[i] == "Host" { // net/http ignores a Host entry in Header
			req.Host = hdr[i+1]
			continue
		}
		req.Header.Set(hdr[i], hdr[i+1])
	}
	resp, err := e.c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func (e *env) login(t *testing.T, id Identity) {
	t.Helper()
	tok, err := e.auth.IssueToken(id)
	if err != nil {
		t.Fatal(err)
	}
	if resp := e.get(t, "/login?token="+tok); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login status = %d", resp.StatusCode)
	}
}

func decodeState(t *testing.T, resp *http.Response) stateJSON {
	t.Helper()
	var s stateJSON
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestState_RequiresLogin(t *testing.T) {
	e := newEnv(t, Options{AuthMethod: config.AuthToken})
	resp := e.get(t, "/api/v1/state")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	var body errorBody
	json.NewDecoder(resp.Body).Decode(&body)
	if body.Code != "unauthenticated" {
		t.Errorf("error body = %+v", body)
	}
}

func TestLogin_SetsHardenedCookieAndOpensTheAPI(t *testing.T) {
	e := newEnv(t, Options{AuthMethod: config.AuthToken, AccessAddress: "https://music.example.com"})
	tok, _ := e.auth.IssueToken(alice)
	resp := e.get(t, "/login?token="+tok)
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/" {
		t.Fatalf("login = %d -> %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	var ck *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == SessionCookie {
			ck = c
		}
	}
	if ck == nil || !ck.HttpOnly || !ck.Secure || ck.SameSite != http.SameSiteLaxMode || ck.Path != "/" || ck.MaxAge <= 0 {
		t.Fatalf("cookie = %+v", ck)
	}
	if resp.Header.Get("Cache-Control") != "no-store" || resp.Header.Get("Referrer-Policy") != "no-referrer" {
		t.Errorf("login response must not be cached or leak the token via Referer: %v", resp.Header)
	}

	// The jar ignores Secure cookies over plain http, so replay it by hand.
	req, _ := http.NewRequest("GET", e.ts.URL+"/api/v1/state", nil)
	req.AddCookie(ck)
	r2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer r2.Body.Close()
	if r2.StatusCode != http.StatusOK {
		t.Fatalf("state with the session = %d", r2.StatusCode)
	}
}

func TestLogin_CookieNotSecureOverHTTPAccessAddress(t *testing.T) {
	e := newEnv(t, Options{AuthMethod: config.AuthToken, AccessAddress: "http://192.168.1.5:8181"})
	tok, _ := e.auth.IssueToken(alice)
	for _, c := range e.get(t, "/login?token="+tok).Cookies() {
		if c.Name == SessionCookie && c.Secure {
			t.Error("Secure on an http access_address would make the cookie unusable")
		}
	}
}

func TestLogin_BadTokenIsRefusedAndTokenIsSingleUse(t *testing.T) {
	e := newEnv(t, Options{AuthMethod: config.AuthToken})
	for _, path := range []string{"/login", "/login?token=", "/login?token=bogus"} {
		resp := e.get(t, path)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s = %d, want 401", path, resp.StatusCode)
		}
		if len(resp.Cookies()) != 0 {
			t.Errorf("%s set a cookie", path)
		}
	}
	tok, _ := e.auth.IssueToken(alice)
	if resp := e.get(t, "/login?token="+tok); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("first use = %d", resp.StatusCode)
	}
	if resp := e.get(t, "/login?token="+tok); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("reusing a token = %d, want 401", resp.StatusCode)
	}
}

func TestState_Content(t *testing.T) {
	e := newEnv(t, Options{AuthMethod: config.AuthToken})
	e.login(t, Identity{Subject: "h", Name: "alice", Roles: []Role{RoleUser, RoleAdmin}})
	resp := e.get(t, "/api/v1/state")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q", ct)
	}
	if resp.Header.Get("ETag") != `"7"` {
		t.Errorf("ETag = %q", resp.Header.Get("ETag"))
	}
	// The body names the caller; a shared cache must not store it or reuse it for
	// someone else, yet the browser may revalidate it.
	cc := resp.Header.Get("Cache-Control")
	if !strings.Contains(cc, "private") || !strings.Contains(cc, "no-cache") || strings.Contains(cc, "public") {
		t.Errorf("Cache-Control = %q, want private, no-cache", cc)
	}
	if !strings.Contains(resp.Header.Get("Vary"), "Cookie") {
		t.Errorf("Vary = %q, want Cookie", resp.Header.Get("Vary"))
	}
	s := decodeState(t, resp)
	if s.Version != "7" || s.Me.Name != "alice" || len(s.Me.Roles) != 2 || s.Me.Roles[1] != "admin" {
		t.Errorf("state = %+v", s)
	}
	if len(s.Queue) != 2 || s.Queue[0].ID != 3 || s.Queue[0].DurationMS != 90000 || !s.Queue[0].Seekable || s.Queue[1].DurationMS != 0 {
		t.Errorf("queue = %+v", s.Queue)
	}
	if !s.Playing || s.PositionMS == nil || *s.PositionMS != 12400 || s.Volume != 0.5 || s.Mode != "repeat" || !s.Scan.Available {
		t.Errorf("state = %+v", s)
	}
}

func TestState_PositionIsNullWhenUnknown(t *testing.T) {
	e := newEnv(t, Options{AuthMethod: config.AuthToken})
	e.bot.snap = status.Snapshot{Version: 1, Mode: "one-shot"}
	e.login(t, alice)
	resp := e.get(t, "/api/v1/state")
	var raw map[string]any
	json.NewDecoder(resp.Body).Decode(&raw)
	if v, present := raw["position_ms"]; !present || v != nil {
		t.Errorf("position_ms = %v (present %v), want null", v, present)
	}
	if q, ok := raw["queue"].([]any); !ok || len(q) != 0 {
		t.Errorf("an empty queue must be [], got %v", raw["queue"])
	}
}

func TestState_ConditionalRequests(t *testing.T) {
	e := newEnv(t, Options{AuthMethod: config.AuthToken})
	e.login(t, alice)
	for name, hdr := range map[string][]string{
		"strong":   {"If-None-Match", `"7"`},
		"weak":     {"If-None-Match", `W/"7"`},
		"list":     {"If-None-Match", `"3", "7"`},
		"wildcard": {"If-None-Match", `*`},
	} {
		resp := e.get(t, "/api/v1/state", hdr...)
		if resp.StatusCode != http.StatusNotModified {
			t.Errorf("%s: status = %d, want 304", name, resp.StatusCode)
		}
	}
	if resp := e.get(t, "/api/v1/state", "If-None-Match", `"7"`); resp.Header.Get("Cache-Control") != "private, no-cache" {
		t.Errorf("a 304 must carry the same cache policy, got %q", resp.Header.Get("Cache-Control"))
	}
	if resp := e.get(t, "/api/v1/state?since=7"); resp.StatusCode != http.StatusNotModified {
		t.Errorf("since=current = %d, want 304", resp.StatusCode)
	}
	if resp := e.get(t, "/api/v1/state?since=6"); resp.StatusCode != http.StatusOK {
		t.Errorf("since=old = %d, want 200", resp.StatusCode)
	}
	if resp := e.get(t, "/api/v1/state", "If-None-Match", `"6"`); resp.StatusCode != http.StatusOK {
		t.Errorf("stale etag = %d, want 200", resp.StatusCode)
	}
	e.bot.snap.Version = 8
	if resp := e.get(t, "/api/v1/state", "If-None-Match", `"7"`); resp.StatusCode != http.StatusOK {
		t.Errorf("after a change = %d, want 200", resp.StatusCode)
	}
}

func TestLogout_EndsTheSession(t *testing.T) {
	e := newEnv(t, Options{AuthMethod: config.AuthToken})
	e.login(t, alice)
	req, _ := http.NewRequest("POST", e.ts.URL+"/api/v1/auth/logout", nil)
	resp, err := e.c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("logout = %d", resp.StatusCode)
	}
	if r := e.get(t, "/api/v1/state"); r.StatusCode != http.StatusUnauthorized {
		t.Errorf("state after logout = %d, want 401", r.StatusCode)
	}
}

func TestRequire_ForbidsAMissingRole(t *testing.T) {
	e := newEnv(t, Options{AuthMethod: config.AuthToken})
	e.srv.handler = e.srv.guard(http.HandlerFunc(e.srv.require(RoleAdmin, func(w http.ResponseWriter, r *http.Request, id Identity) {
		w.WriteHeader(http.StatusNoContent)
	})))
	ts := httptest.NewServer(e.srv.handler)
	defer ts.Close()
	tok, _ := e.auth.IssueToken(alice)
	sid, _, _ := e.auth.Redeem(tok)
	req, _ := http.NewRequest("GET", ts.URL, nil)
	req.AddCookie(&http.Cookie{Name: SessionCookie, Value: sid})
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("a user hitting an admin handler = %d, want 403", resp.StatusCode)
	}
}

func TestNoLogin_EveryoneIsAnonymousUser(t *testing.T) {
	e := newEnv(t, Options{AuthMethod: config.AuthNone, Addr: "127.0.0.1:8181"})
	e.ts.Config.Handler = e.srv.Handler()
	resp := e.get(t, "/api/v1/state", "Host", "127.0.0.1:8181")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if s := decodeState(t, resp); s.Me.Name != "anonymous" || len(s.Me.Roles) != 1 || s.Me.Roles[0] != "user" {
		t.Errorf("me = %+v", s.Me)
	}
	if r := e.get(t, "/login", "Host", "127.0.0.1:8181"); r.StatusCode != http.StatusSeeOther {
		t.Errorf("/login without a login = %d, want a redirect", r.StatusCode)
	}
}

func TestHostGuard_OnlyWithoutLogin(t *testing.T) {
	none := NewServer(&fakeBot{}, nil, Options{AuthMethod: config.AuthNone, Addr: "0.0.0.0:8181", AccessAddress: "https://music.example.com"})
	token := NewServer(&fakeBot{}, NewAuth(AuthOptions{}), Options{AuthMethod: config.AuthToken, Addr: "127.0.0.1:8181"})

	do := func(s *Server, host string) int {
		req := httptest.NewRequest("GET", "/api/v1/state", nil)
		req.Host = host
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		return rec.Code
	}
	for host, want := range map[string]int{
		"music.example.com":   http.StatusOK,
		"MUSIC.example.com":   http.StatusOK,
		"localhost:8181":      http.StatusOK,
		"127.0.0.1:8181":      http.StatusOK,
		"[::1]:8181":          http.StatusOK,
		"attacker.example":    http.StatusMisdirectedRequest, // DNS rebinding
		"127.0.0.1.evil.test": http.StatusMisdirectedRequest,
		"localhost:9999":      http.StatusMisdirectedRequest,
	} {
		if got := do(none, host); got != want {
			t.Errorf("no-login Host %q = %d, want %d", host, got, want)
		}
	}
	// Token mode: the cookie already ties the request to the right origin.
	if got := do(token, "attacker.example"); got != http.StatusUnauthorized {
		t.Errorf("token mode must not host-filter (401 for no cookie), got %d", got)
	}
}

func TestUnknownAPIPathIsJSON404(t *testing.T) {
	e := newEnv(t, Options{AuthMethod: config.AuthToken})
	resp := e.get(t, "/api/v1/nope")
	if resp.StatusCode != http.StatusNotFound || !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		t.Errorf("= %d %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
}

func TestServe_StopsOnContextCancel(t *testing.T) {
	s := NewServer(&fakeBot{}, NewAuth(AuthOptions{}), Options{AuthMethod: config.AuthToken})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx, ln) }()

	resp, err := http.Get("http://" + ln.Addr().String() + "/api/v1/state")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Serve = %v after cancel, want nil", err)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("Serve did not return after the context was cancelled")
	}
}
