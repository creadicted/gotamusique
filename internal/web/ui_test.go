package web

import (
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/konradk/gotamusique/internal/config"
)

// ---- serving ----

func TestUI_IndexIsPublicWithAStrictPolicy(t *testing.T) {
	e := newEnv(t, Options{AuthMethod: config.AuthToken}) // not logged in
	resp := e.get(t, "/")
	page, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") {
		t.Fatalf("/ = %d %q: the interface must load before login, it shows the login hint itself", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	csp := resp.Header.Get("Content-Security-Policy")
	for _, must := range []string{"default-src 'none'", "script-src 'self'", "style-src 'self'", "connect-src 'self'", "font-src 'self'", "frame-ancestors 'none'"} {
		if !strings.Contains(csp, must) {
			t.Errorf("CSP lacks %q: %s", must, csp)
		}
	}
	for _, never := range []string{"unsafe-inline", "unsafe-eval", "*", "http:", "https:", "data:"} {
		if strings.Contains(csp, never) {
			t.Errorf("CSP must not contain %q: %s", never, csp)
		}
	}
	if !strings.Contains(string(page), "/ui/app.css") || !strings.Contains(string(page), `type="module"`) {
		t.Errorf("index does not load the stylesheet and the module: %s", page)
	}
	if resp.Header.Get("X-Frame-Options") != "DENY" {
		t.Error("the page must not be frameable")
	}
}

func TestUI_PublicInNoLoginModeToo(t *testing.T) {
	e := newEnv(t, Options{AuthMethod: config.AuthNone, Addr: "127.0.0.1:8181"})
	if resp := e.get(t, "/", "Host", "127.0.0.1:8181"); resp.StatusCode != 200 {
		t.Errorf("/ = %d", resp.StatusCode)
	}
}

// The page must not depend on anything the server does not serve: every stylesheet,
// module, font and icon it names has to exist, with the right content type.
func TestUI_EveryReferencedFileIsServed(t *testing.T) {
	e := newEnv(t, Options{AuthMethod: config.AuthToken})
	wantType := map[string]string{".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".woff2": "font/woff2"}

	seen := map[string]bool{}
	var visit func(url string)
	visit = func(url string) {
		if seen[url] {
			return
		}
		seen[url] = true
		resp := e.get(t, url)
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != 200 {
			t.Errorf("%s = %d", url, resp.StatusCode)
			return
		}
		ext := path.Ext(url)
		if want, ok := wantType[ext]; ok && !strings.HasPrefix(resp.Header.Get("Content-Type"), want) {
			t.Errorf("%s Content-Type = %q, want %s", url, resp.Header.Get("Content-Type"), want)
		}
		src := string(body)
		switch ext {
		case ".js":
			for _, m := range regexp.MustCompile(`(?m)^\s*import\s+(?:[^'"]*?\s+from\s+)?['"](\./[^'"]+)['"]`).FindAllStringSubmatch(src, -1) {
				visit(path.Join(path.Dir(url), m[1]))
			}
		case ".css":
			for _, m := range regexp.MustCompile(`url\(([^)]+)\)`).FindAllStringSubmatch(src, -1) {
				ref := strings.Trim(m[1], `"'`)
				if !strings.HasPrefix(ref, "/") { // relative to the stylesheet, as the browser reads it
					ref = path.Join(path.Dir(url), ref)
				}
				visit(ref)
			}
		}
	}
	index := string(mustRead(t, e.get(t, "/")))
	refs := regexp.MustCompile(`(?:src|href)="(/ui/[^"]+)"`).FindAllStringSubmatch(index, -1)
	if len(refs) < 3 {
		t.Fatalf("index references only %d /ui/ files", len(refs))
	}
	for _, m := range refs {
		visit(m[1])
	}
	if !seen["/ui/js/app.js"] || !seen["/ui/fonts/archivo-latin-wdth-normal.woff2"] {
		t.Errorf("did not reach the app and the font through the page: %v", keys(seen))
	}
	if len(seen) < 15 {
		t.Errorf("only %d files were reached; the module graph looks broken", len(seen))
	}
}

func mustRead(t *testing.T, resp *http.Response) []byte {
	t.Helper()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func keys(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestUI_CachingRevalidatesAndTheFontIsCached(t *testing.T) {
	e := newEnv(t, Options{AuthMethod: config.AuthToken})
	resp := e.get(t, "/ui/app.css")
	etag := resp.Header.Get("ETag")
	if etag == "" || resp.Header.Get("Cache-Control") != "no-cache" {
		t.Fatalf("ETag %q, Cache-Control %q: scripts and styles must revalidate so an upgrade shows at once", etag, resp.Header.Get("Cache-Control"))
	}
	if r := e.get(t, "/ui/app.css", "If-None-Match", etag); r.StatusCode != http.StatusNotModified {
		t.Errorf("revalidating = %d, want 304", r.StatusCode)
	}
	if r := e.get(t, "/ui/app.css", "If-None-Match", `"other"`); r.StatusCode != 200 {
		t.Errorf("a different ETag = %d, want 200", r.StatusCode)
	}
	if cc := e.get(t, "/ui/fonts/archivo-latin-wdth-normal.woff2").Header.Get("Cache-Control"); !strings.Contains(cc, "max-age") {
		t.Errorf("the font Cache-Control = %q, want a long cache", cc)
	}
}

// Only the listed files are served: no directory listing, no READMEs or licence texts
// from the tree, no way out through "..".
func TestUI_OnlyTheListedFilesAreServed(t *testing.T) {
	e := newEnv(t, Options{AuthMethod: config.AuthToken})
	for _, p := range []string{
		"/ui/", "/ui", "/ui/js/", "/ui/js", "/ui/fonts/", "/ui/README.md", "/ui/fonts/OFL.txt", "/ui/nope.js",
		"/ui/index.html", "/ui/../openapi.yaml", "/ui/..%2fopenapi.yaml", "/ui/%2e%2e/ui.go", "/ui/js/../../ui.go",
		"/ui/js/app.js/", "/ui/JS/app.js", "/ui/js/APP.JS",
	} {
		resp := e.get(t, p)
		if loc := resp.Header.Get("Location"); resp.StatusCode/100 == 3 && loc != "" {
			resp = e.get(t, strings.TrimPrefix(loc, e.ts.URL))
		}
		if resp.StatusCode == 200 {
			t.Errorf("%s was served", p)
		}
	}
	if !uiFilesComplete() {
		t.Error("the file table is missing the interface")
	}
}

func uiFilesComplete() bool {
	for _, f := range []string{"index.html", "app.css", "js/app.js", "icon.svg", "fonts/archivo-latin-wdth-normal.woff2"} {
		if _, ok := uiFiles[f]; !ok {
			return false
		}
	}
	return true
}

// ---- the rules the interface keeps ----

func uiSources(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	for name, f := range uiFiles {
		if strings.HasSuffix(name, ".js") || strings.HasSuffix(name, ".css") || strings.HasSuffix(name, ".html") || strings.HasSuffix(name, ".svg") {
			out[name] = string(f.body)
		}
	}
	if len(out) < 10 {
		t.Fatalf("found only %d interface sources", len(out))
	}
	return out
}

// Text from the bot (station names, file tags, user names) must reach the page as text.
// These constructs would turn it into markup or code, and the policy would not save us
// from all of them, so they are banned outright.
func TestUI_NeverBuildsMarkupOrCodeFromText(t *testing.T) {
	banned := regexp.MustCompile(`\.innerHTML|\.outerHTML|insertAdjacentHTML|document\.write|\beval\s*\(|new\s+Function\s*\(|setAttribute\(\s*['"]style['"]|setAttribute\(\s*['"]on\w+['"]|srcdoc|createContextualFragment|DOMParser`)
	for name, src := range uiSources(t) {
		if !strings.HasSuffix(name, ".js") {
			continue
		}
		if m := banned.FindString(src); m != "" {
			t.Errorf("%s uses %q", name, m)
		}
	}
}

// The policy forbids inline scripts and styles, so the page would silently break.
func TestUI_HasNoInlineScriptOrStyle(t *testing.T) {
	index := uiSources(t)["index.html"]
	for _, tag := range regexp.MustCompile(`(?i)<script[^>]*>`).FindAllString(index, -1) {
		if !strings.Contains(tag, "src=") {
			t.Errorf("inline script %q", tag)
		}
	}
	if regexp.MustCompile(`(?i)<style|\sstyle\s*=|\son\w+\s*=`).MatchString(index) {
		t.Error("index.html has an inline style or event handler")
	}
}

// The interface loads nothing from the internet: no CDN, no web fonts, no analytics.
func TestUI_LoadsNothingFromTheInternet(t *testing.T) {
	external := regexp.MustCompile(`(?i)(?:src|href)\s*=\s*["']https?://|url\(\s*["']?https?://|from\s+["']https?://|import\(\s*["']https?://|fetch\(\s*["']https?://|@import\s+["']?https?://|\.open\(\s*["'][A-Z]+["']\s*,\s*["']https?://`)
	for name, src := range uiSources(t) {
		if m := external.FindString(src); m != "" {
			t.Errorf("%s loads from the internet: %q", name, m)
		}
	}
}

// ---- the pure logic, tested with Node when it is available ----

// uitest/*.test.mjs run with `node --test`. They cover the logic that has no DOM
// (formatting, the playhead clock, queue moves, decoding frames). Skipped without
// Node; the interface itself never needs Node.
func TestUI_LogicUnitTests(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	files, _ := filepath.Glob("uitest/*.test.mjs")
	if len(files) == 0 {
		t.Fatal("no uitest/*.test.mjs files")
	}
	cmd := exec.Command(node, append([]string{"--test"}, files...)...) // a directory argument is not accepted by every Node version
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node --test failed: %v\n%s", err, out)
	}
	t.Logf("%s", lastLines(string(out), 8))
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
