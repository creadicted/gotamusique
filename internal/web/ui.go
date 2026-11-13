package web

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
)

// uiFS is the browser interface: plain HTML, CSS and ES modules plus an embedded font.
// There is no build step; see ui/README.md.
//
//go:embed ui
var uiFS embed.FS

// uiCSP is the policy of the interface page. Scripts, styles, fonts and requests
// all have to come from the bot itself; nothing inline runs, and nothing is loaded
// from the internet.
const uiCSP = "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self'; " +
	"font-src 'self'; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

type uiFile struct {
	body []byte
	etag string
	mime string
	// immutable files (the font) can be cached for a day; the rest is revalidated, so a
	// new version of the bot is picked up at once and an unchanged file costs a 304.
	longCache bool
}

// uiFiles maps a request path below /ui/ to its file. It is built once, and is the
// whole list of what is served: README files and anything else in the tree are not in it.
var uiFiles = loadUI()

func loadUI() map[string]uiFile {
	out := map[string]uiFile{}
	fs.WalkDir(uiFS, "ui", func(p string, d fs.DirEntry, err error) error { //nolint:errcheck // embedded: cannot fail
		if err != nil || d.IsDir() || strings.HasSuffix(p, ".md") || strings.HasSuffix(p, ".txt") {
			return nil
		}
		body, err := uiFS.ReadFile(p)
		if err != nil {
			return nil
		}
		sum := sha256.Sum256(body)
		name := strings.TrimPrefix(p, "ui/")
		ctype := mime.TypeByExtension(path.Ext(name))
		switch path.Ext(name) {
		case ".js":
			ctype = "text/javascript; charset=utf-8"
		case ".css":
			ctype = "text/css; charset=utf-8"
		case ".html":
			ctype = "text/html; charset=utf-8"
		case ".svg":
			ctype = "image/svg+xml"
		case ".woff2":
			ctype = "font/woff2"
		}
		out[name] = uiFile{body: body, etag: `"` + hex.EncodeToString(sum[:12]) + `"`, mime: ctype, longCache: path.Ext(name) == ".woff2"}
		return nil
	})
	return out
}

func serveUI(w http.ResponseWriter, r *http.Request, f uiFile) {
	h := w.Header()
	h.Set("Content-Type", f.mime)
	h.Set("ETag", f.etag)
	if f.longCache {
		h.Set("Cache-Control", "public, max-age=86400")
	} else {
		h.Set("Cache-Control", "no-cache")
	}
	if etagMatches(r.Header.Get("If-None-Match"), f.etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Write(f.body) //nolint:errcheck // the client went away
}

// handleIndex serves the interface. It is public: the page holds nothing private, and
// asks the API itself, showing how to log in when the API answers 401.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	f, ok := uiFiles["index.html"]
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Security-Policy", uiCSP)
	serveUI(w, r, f)
}

// handleUIAsset serves the interface's scripts, stylesheet, font and icon.
func (s *Server) handleUIAsset(w http.ResponseWriter, r *http.Request) {
	f, ok := uiFiles[r.PathValue("path")]
	if !ok || r.PathValue("path") == "index.html" {
		http.NotFound(w, r)
		return
	}
	serveUI(w, r, f)
}
