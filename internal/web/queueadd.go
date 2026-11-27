package web

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/konradk/gotamusique/internal/queue"
)

// addTimeout bounds the network checks of one add. It stays below the server's
// write timeout so the client gets an answer instead of a dropped connection.
const addTimeout = 25 * time.Second

// addResponse is the state plus the entry that was just created.
type addResponse struct {
	stateJSON
	Added entryRefJSON `json:"added"`
}

type entryRefJSON struct {
	ID    uint64 `json:"id"`
	Title string `json:"title"`
}

// handleAdd queues something: a library file, a radio station or a URL.
func (s *Server) handleAdd(w http.ResponseWriter, r *http.Request, id Identity) {
	var req struct {
		Type     string `json:"type"`
		Path     string `json:"path"`
		Preset   string `json:"preset"`
		URL      string `json:"url"`
		UUID     string `json:"uuid"`
		Position string `json:"position"` // end (default), next or now
	}
	if !decodeJSON(w, r, &req, false) {
		return
	}
	var pos queue.Position
	playNow := false
	switch req.Position {
	case "", "end":
		pos = queue.AtEnd
	case "next":
		pos = queue.AfterCurrent
	case "now":
		pos, playNow = queue.AfterCurrent, true
	default:
		writeError(w, http.StatusBadRequest, "bad_request", `position must be "end", "next" or "now"`)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), addTimeout)
	defer cancel()
	added, err := s.bot.Add(ctx, Source{Type: req.Type, Path: req.Path, Preset: req.Preset, URL: req.URL, UUID: req.UUID}, pos)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if playNow {
		// Someone may have removed the entry in the meantime; then there is
		// simply nothing to start and the state tells the story.
		if err := s.bot.PlayEntry(added.ID); err != nil && !isNotFound(err) {
			s.fail(w, r, err)
			return
		}
	}

	st := toStateJSON(s.bot.Snapshot(), id)
	w.Header().Set("ETag", `"`+st.Version+`"`)
	writeJSON(w, http.StatusCreated, addResponse{stateJSON: st, Added: entryRefJSON{ID: added.ID, Title: added.Title}})
}

type stationJSON struct {
	UUID    string `json:"uuid"`
	Name    string `json:"name"`
	Codec   string `json:"codec"`
	Bitrate int    `json:"bitrate"`
	Country string `json:"country"`
	Tags    string `json:"tags"`
}

const (
	defaultSearchLimit = 10
	maxSearchLimit     = 50
	maxSearchQuery     = 100
	searchTimeout      = 10 * time.Second
)

// handleRadioSearch looks stations up on radio-browser.info. Add a result with
// {"type":"radio","uuid":...}.
func (s *Server) handleRadioSearch(w http.ResponseWriter, r *http.Request, id Identity) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" || len(q) > maxSearchQuery {
		writeError(w, http.StatusBadRequest, "bad_request", "q is required (at most 100 characters)")
		return
	}
	limit := defaultSearchLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "bad_request", "limit must be a positive number")
			return
		}
		limit = min(n, maxSearchLimit)
	}

	ctx, cancel := context.WithTimeout(r.Context(), searchTimeout)
	defer cancel()
	stations, err := s.bot.SearchRadio(ctx, q, limit)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]stationJSON, len(stations))
	for i, st := range stations {
		out[i] = stationJSON{UUID: st.UUID, Name: st.Name, Codec: st.Codec, Bitrate: st.Bitrate, Country: st.Country, Tags: st.Tags}
	}
	writeJSON(w, http.StatusOK, struct {
		Stations []stationJSON `json:"stations"`
	}{out})
}
