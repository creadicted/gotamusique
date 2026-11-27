package web

import (
	"bytes"
	"embed"
	"net/http"
)

// specYAML is the OpenAPI description of the API. It is written by hand; the tests
// compare it with the route table and validate the handlers' real responses
// against it, so it cannot drift without a test failing.
//
//go:embed openapi.yaml
var specYAML []byte

// docsFS holds the interactive documentation page: Swagger UI (vendored, see
// swaggerui/README.md) plus our two small files.
//
//go:embed swaggerui/index.html swaggerui/init.js swaggerui/swagger-ui-bundle.js swaggerui/swagger-ui.css
var docsFS embed.FS

// docsCSP is what the documentation page needs and no more: its own scripts and
// styles (Swagger UI sets inline style attributes), images and fonts as data URLs,
// and requests to this server only. Everywhere else the strict default applies.
const docsCSP = "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data:; font-src 'self' data:; connect-src 'self'; " +
	"base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

// handleSpec serves the OpenAPI description.
func (s *Server) handleSpec(w http.ResponseWriter, r *http.Request, id Identity) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("Cache-Control", "private, no-cache")
	body := specYAML
	if s.prefix != "" {
		// "Try it out" builds its requests from the server address, which has to include the proxy's path.
		body = bytes.Replace(body, []byte("  - url: /\n"), []byte("  - url: "+s.prefix+"\n"), 1)
	}
	w.Write(body) //nolint:errcheck // the client went away
}

// handleDocs serves the Swagger UI page. It needs a login like the API does, since
// its purpose is to call the API with the visitor's session.
func (s *Server) handleDocs(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.identify(r); !ok {
		writePage(w, http.StatusUnauthorized, "API documentation",
			"Log in first: type the web command in Mumble, open the link you get, then reload this page.")
		return
	}
	page, err := docsFS.ReadFile("swaggerui/index.html")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	if s.prefix != "" {
		page = bytes.ReplaceAll(page, []byte(`"/api/docs/assets/`), []byte(`"`+s.prefix+`/api/docs/assets/`))
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "private, no-cache")
	w.Header().Set("Content-Security-Policy", docsCSP)
	w.Write(page) //nolint:errcheck
}

// docsAssets are the only files served under /api/docs/assets/. An allowlist keeps
// the handler from ever listing or reading anything else in the embedded tree.
var docsAssets = map[string]bool{
	"swagger-ui-bundle.js": true,
	"swagger-ui.css":       true,
	"init.js":              true,
}

// handleDocsAsset serves the page's scripts and stylesheet. They are public static
// files (open source, nothing secret), so they need no login.
func (s *Server) handleDocsAsset(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("file")
	if !docsAssets[name] {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=3600")
	http.ServeFileFS(w, r, docsFS, "swaggerui/"+name)
}
