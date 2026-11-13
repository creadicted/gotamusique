package web

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/konradk/gotamusique/internal/config"
)

// The bot behind a reverse proxy that serves it under a path: access_address carries the path.
func prefixEnv(t *testing.T) *env {
	return newEnv(t, Options{AuthMethod: config.AuthToken, AccessAddress: "https://example.org/music/"})
}

func body(t *testing.T, resp *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestPrefix_BothProxyStylesReachTheSameHandlers(t *testing.T) {
	e := prefixEnv(t)
	// A proxy that strips the path asks for /…; one that keeps it asks for /music/….
	for _, p := range []string{"/", "/music", "/music/"} {
		resp := e.get(t, p)
		if resp.StatusCode != http.StatusOK || !strings.Contains(body(t, resp), "<title>gotamusique</title>") {
			t.Errorf("GET %s = %d, want the page", p, resp.StatusCode)
		}
	}
	for _, p := range []string{"/api/v1/state", "/music/api/v1/state"} {
		if resp := e.get(t, p); resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("GET %s = %d, want 401 from the API", p, resp.StatusCode)
		}
	}
	if resp := e.get(t, "/musicology"); resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET /musicology = %d: only /music and /music/... belong to the prefix", resp.StatusCode)
	}
}

func TestPrefix_PageNamesItsFilesUnderThePath(t *testing.T) {
	e := prefixEnv(t)
	page := body(t, e.get(t, "/"))
	for _, want := range []string{`href="/music/ui/app.css"`, `src="/music/ui/js/app.js"`, `href="/music/ui/icon.svg"`} {
		if !strings.Contains(page, want) {
			t.Errorf("the page lacks %s:\n%s", want, page)
		}
	}
	if strings.Contains(page, `"/ui/`) {
		t.Errorf("a file is still named without the path:\n%s", page)
	}
	// The cache validator follows the rewritten page, so a changed prefix is not served from cache.
	root := newEnv(t, Options{AuthMethod: config.AuthToken})
	a, b := e.get(t, "/").Header.Get("ETag"), root.get(t, "/").Header.Get("ETag")
	if a == "" || a == b {
		t.Errorf("ETags %q and %q must differ", a, b)
	}
}

func TestPrefix_AtTheRootNothingChanges(t *testing.T) {
	e := newEnv(t, Options{AuthMethod: config.AuthToken, AccessAddress: "https://example.org"})
	if page := body(t, e.get(t, "/")); !strings.Contains(page, `href="/ui/app.css"`) {
		t.Errorf("page = %s", page)
	}
}

func TestPrefix_LoginCookieAndRedirectStayUnderThePath(t *testing.T) {
	e := prefixEnv(t)
	tok, _ := e.auth.IssueToken(alice)
	resp := e.get(t, "/login?token="+tok)
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/music/" {
		t.Fatalf("login = %d -> %q, want a redirect to /music/", resp.StatusCode, resp.Header.Get("Location"))
	}
	var path string
	for _, c := range resp.Cookies() {
		if c.Name == SessionCookie {
			path = c.Path
		}
	}
	if path != "/music" {
		t.Errorf("cookie path = %q, want /music (other sites on the domain must not get it)", path)
	}
	if e.srv.root() != "/music/" || LoginURL("https://example.org/music/", "t") != "https://example.org/music/login?token=t" {
		t.Errorf("root %q, login url %q", e.srv.root(), LoginURL("https://example.org/music/", "t"))
	}
}

func TestPrefix_DocsAndSpecPointBackUnderThePath(t *testing.T) {
	// http, not https: the test client's cookie jar drops Secure cookies on a plain connection.
	e := newEnv(t, Options{AuthMethod: config.AuthToken, AccessAddress: "http://example.org/music"})
	e.login(t, alice)
	docs := body(t, e.get(t, "/music/api/docs"))
	for _, want := range []string{`"/music/api/docs/assets/swagger-ui.css"`, `"/music/api/docs/assets/init.js"`} {
		if !strings.Contains(docs, want) {
			t.Errorf("docs page lacks %s:\n%s", want, docs)
		}
	}
	spec := body(t, e.get(t, "/music/api/v1/openapi.yaml"))
	if !strings.Contains(spec, "\n  - url: /music\n") {
		t.Errorf("the spec's server is not the path:\n%.300s", spec)
	}
	root := newEnv(t, Options{AuthMethod: config.AuthToken})
	root.login(t, alice)
	if s := body(t, root.get(t, "/api/v1/openapi.yaml")); !strings.Contains(s, "\n  - url: /\n") {
		t.Error("at the root the spec's server must stay /")
	}
}

// Files the page loads name each other relatively, or the proxy's path would be lost: a stylesheet's
// url(/ui/fonts/...) would ask the root of the domain for the font.
func TestPrefix_FilesDoNotNameEachOtherByAbsolutePath(t *testing.T) {
	for name, f := range uiFiles {
		if !strings.HasSuffix(name, ".css") && !strings.HasSuffix(name, ".js") {
			continue
		}
		for _, bad := range []string{"url(/", `url("/`, `from '/`, `from "/`, `import('/`, `src = '/`} {
			if strings.Contains(string(f.body), bad) {
				t.Errorf("%s contains %q: use a path relative to the file", name, bad)
			}
		}
	}
}
