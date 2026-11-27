package web

import (
	"net/http"
)

// route is one HTTP endpoint. Every route is declared here and nowhere else, so a
// test can compare the table with the OpenAPI spec (openapi.yaml) and fail when
// they drift apart.
type route struct {
	method  string
	path    string // ServeMux pattern path; {name} wildcards match OpenAPI's syntax
	handler http.HandlerFunc
	// internal routes serve pages and static files rather than API operations, and
	// are left out of the spec.
	internal bool
}

func (s *Server) routes() []route {
	user := func(h func(http.ResponseWriter, *http.Request, Identity)) http.HandlerFunc {
		return s.require(RoleUser, h)
	}
	return []route{
		{"GET", "/{$}", s.handleIndex, true},
		{"GET", "/ui/{path...}", s.handleUIAsset, true},
		{"GET", "/login", s.handleLogin, false},
		{"POST", "/api/v1/auth/logout", s.handleLogout, false},

		{"GET", "/api/v1/openapi.yaml", user(s.handleSpec), false},
		{"GET", "/api/docs", s.handleDocs, true},
		{"GET", "/api/docs/assets/{file}", s.handleDocsAsset, true},

		{"GET", "/api/v1/state", user(s.handleState), false},

		{"POST", "/api/v1/playback/pause", s.userAction(func() { s.bot.Pause() }), false},
		{"POST", "/api/v1/playback/resume", s.userAction(func() { s.bot.Resume() }), false},
		{"POST", "/api/v1/playback/stop", s.userAction(s.bot.Stop), false},
		{"POST", "/api/v1/playback/next", s.userAction(s.bot.Skip), false},
		{"POST", "/api/v1/playback/seek", user(s.handleSeek), false},
		{"PUT", "/api/v1/volume", user(s.handleVolume), false},
		{"PUT", "/api/v1/mode", user(s.handleMode), false},

		{"POST", "/api/v1/queue", user(s.handleAdd), false},
		{"DELETE", "/api/v1/queue", s.userAction(s.bot.Clear), false},
		{"POST", "/api/v1/queue/shuffle", s.userAction(s.bot.RandomizeQueue), false},
		{"DELETE", "/api/v1/queue/{id}", user(s.handleRemove), false},
		{"POST", "/api/v1/queue/{id}/play", user(s.handlePlayEntry), false},
		{"POST", "/api/v1/queue/{id}/move", user(s.handleMove), false},

		{"GET", "/api/v1/radio/presets", user(s.handleRadioPresets), false},
		{"GET", "/api/v1/radio/search", user(s.handleRadioSearch), false},
		{"GET", "/api/v1/library", user(s.handleLibrary), false},
		{"POST", "/api/v1/library/rescan", user(s.handleRescan), false},
		{"POST", "/api/v1/library/upload", user(s.handleUpload), false},
	}
}

// mux builds the router from the route table.
func (s *Server) mux() *http.ServeMux {
	mux := http.NewServeMux()
	for _, r := range s.routes() {
		mux.HandleFunc(r.method+" "+r.path, r.handler)
	}
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "no such endpoint")
	})
	return mux
}
