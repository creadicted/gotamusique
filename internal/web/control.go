package web

import (
	"math"
	"net/http"
	"time"

	"github.com/konradk/gotamusique/internal/queue"
)

// userAction wraps a handler body that needs no request data: it runs act and
// answers with the new state.
func (s *Server) userAction(act func()) http.HandlerFunc {
	return s.require(RoleUser, func(w http.ResponseWriter, r *http.Request, id Identity) {
		act()
		s.writeState(w, http.StatusOK, id)
	})
}

func (s *Server) handleSeek(w http.ResponseWriter, r *http.Request, id Identity) {
	var req struct {
		PositionMS *int64 `json:"position_ms"`
	}
	if !decodeJSON(w, r, &req, false) {
		return
	}
	if req.PositionMS == nil {
		writeError(w, http.StatusBadRequest, "bad_request", "position_ms is required")
		return
	}
	// Check before converting: a position past what a time.Duration holds would wrap
	// around in the multiplication (math.MinInt64 ms comes out as exactly 0) and
	// silently seek to the start. Both ends are out of range anyway.
	if *req.PositionMS < 0 || *req.PositionMS > math.MaxInt64/int64(time.Millisecond) {
		writeError(w, http.StatusBadRequest, "seek_range", ErrSeekRange.Error())
		return
	}
	if err := s.bot.Seek(time.Duration(*req.PositionMS) * time.Millisecond); err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeState(w, http.StatusOK, id)
}

// handleVolume sets the volume, the mute state, or both. Setting a volume ends a
// mute, so with both fields the mute is applied last.
func (s *Server) handleVolume(w http.ResponseWriter, r *http.Request, id Identity) {
	var req struct {
		Volume *float64 `json:"volume"` // 0..1
		Muted  *bool    `json:"muted"`
	}
	if !decodeJSON(w, r, &req, false) {
		return
	}
	if req.Volume == nil && req.Muted == nil {
		writeError(w, http.StatusBadRequest, "bad_request", "give volume, muted or both")
		return
	}
	if req.Volume != nil && !(*req.Volume >= 0 && *req.Volume <= 1) { // also rejects NaN
		writeError(w, http.StatusBadRequest, "bad_request", "volume must be between 0 and 1")
		return
	}
	if req.Volume != nil {
		s.bot.SetVolume(int(math.Round(*req.Volume * 100)))
	}
	if req.Muted != nil {
		if *req.Muted {
			s.bot.Mute()
		} else {
			s.bot.Unmute()
		}
	}
	s.writeState(w, http.StatusOK, id)
}

func (s *Server) handleMode(w http.ResponseWriter, r *http.Request, id Identity) {
	var req struct {
		Mode string `json:"mode"`
	}
	if !decodeJSON(w, r, &req, false) {
		return
	}
	m, err := queue.ParseMode(req.Mode)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_mode", "unknown mode; use "+queue.ModeList())
		return
	}
	if err := s.bot.SetPlaybackMode(string(m)); err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeState(w, http.StatusOK, id)
}

func (s *Server) handleRemove(w http.ResponseWriter, r *http.Request, id Identity) {
	entry, ok := entryID(r)
	if !ok {
		s.fail(w, r, queue.ErrNotFound)
		return
	}
	if err := s.bot.RemoveEntry(entry); err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeState(w, http.StatusOK, id)
}

func (s *Server) handlePlayEntry(w http.ResponseWriter, r *http.Request, id Identity) {
	entry, ok := entryID(r)
	if !ok {
		s.fail(w, r, queue.ErrNotFound)
		return
	}
	if err := s.bot.PlayEntry(entry); err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeState(w, http.StatusOK, id)
}

// handleMove puts an entry directly after another one; after 0 means the front.
func (s *Server) handleMove(w http.ResponseWriter, r *http.Request, id Identity) {
	entry, ok := entryID(r)
	if !ok {
		s.fail(w, r, queue.ErrNotFound)
		return
	}
	var req struct {
		After *uint64 `json:"after"`
	}
	if !decodeJSON(w, r, &req, false) {
		return
	}
	if req.After == nil {
		writeError(w, http.StatusBadRequest, "bad_request", "after is required (an entry id, or 0 for the front)")
		return
	}
	if err := s.bot.MoveEntry(entry, *req.After); err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeState(w, http.StatusOK, id)
}
