package web

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/konradk/gotamusique/internal/status"
)

// stateJSON is the wire form of the bot state. Durations are milliseconds.
type stateJSON struct {
	// Version is opaque; it changes exactly when something other than the
	// playhead position changed. Send it back as If-None-Match (or ?since=).
	Version    string      `json:"version"`
	Me         meJSON      `json:"me"`
	Queue      []entryJSON `json:"queue"`
	Index      int         `json:"index"` // len(queue) when idle
	Playing    bool        `json:"playing"`
	Pause      string      `json:"pause"` // "", "manual" or "auto"
	PositionMS *int64      `json:"position_ms"`
	Volume     float64     `json:"volume"` // 0..1
	Muted      bool        `json:"muted"`
	Mode       string      `json:"mode"`
	Scan       scanJSON    `json:"scan"`
}

type meJSON struct {
	Name  string   `json:"name"`
	Roles []string `json:"roles"`
}

type entryJSON struct {
	ID         uint64 `json:"id"`
	Title      string `json:"title"`
	DurationMS int64  `json:"duration_ms"` // 0 when unknown
	Seekable   bool   `json:"seekable"`
}

type scanJSON struct {
	Available  bool   `json:"available"`
	Running    bool   `json:"running"`
	LastResult string `json:"last_result"` // summary of the last finished scan, or ""
	LastError  string `json:"last_error"`  // why it failed, or ""
}

func toStateJSON(s status.Snapshot, id Identity) stateJSON {
	out := stateJSON{
		Version: version(s),
		Me:      meJSON{Name: id.Name, Roles: make([]string, len(id.Roles))},
		Queue:   make([]entryJSON, len(s.Queue)),
		Index:   s.Index, Playing: s.Playing, Pause: s.Pause,
		Volume: s.Volume, Muted: s.Muted, Mode: s.Mode,
		Scan: scanJSON{Available: s.Scan.Available, Running: s.Scan.Running,
			LastResult: s.Scan.LastResult, LastError: s.Scan.LastError},
	}
	for i, r := range id.Roles {
		out.Me.Roles[i] = string(r)
	}
	for i, e := range s.Queue {
		out.Queue[i] = entryJSON{ID: e.ID, Title: e.Title, DurationMS: e.Duration.Milliseconds(), Seekable: e.Seekable}
	}
	if s.PositionKnown {
		ms := s.Position.Milliseconds()
		out.PositionMS = &ms
	}
	return out
}

func version(s status.Snapshot) string { return strconv.FormatUint(s.Version, 10) }

// handleState returns the full state. A client that sends the version it last saw
// (If-None-Match, or ?since=) gets 304 while nothing changed, which makes polling cheap.
func (s *Server) handleState(w http.ResponseWriter, r *http.Request, id Identity) {
	snap := s.bot.Snapshot()
	v := version(snap)
	etag := `"` + v + `"`
	w.Header().Set("ETag", etag)
	// The body names the caller (me), while the ETag is only the bot's state
	// version, so a shared cache must never store or revalidate it for someone
	// else: private keeps it to the user's own browser, no-cache keeps polling
	// conditional.
	w.Header().Set("Cache-Control", "private, no-cache")
	w.Header().Set("Vary", "Cookie")
	if since := r.URL.Query().Get("since"); since == v || etagMatches(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	writeJSONKeepCache(w, http.StatusOK, toStateJSON(snap, id))
}

// etagMatches reports whether an If-None-Match header lists etag (weak or strong).
func etagMatches(header, etag string) bool {
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if part == "*" || strings.TrimPrefix(part, "W/") == etag {
			return true
		}
	}
	return false
}
