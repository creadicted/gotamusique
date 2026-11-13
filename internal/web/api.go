package web

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/konradk/gotamusique/internal/library"
	urlitem "github.com/konradk/gotamusique/internal/media/url"
	"github.com/konradk/gotamusique/internal/queue"
)

// maxBody bounds a request body. Every request here is a small JSON object.
const maxBody = 64 << 10

// decodeJSON reads one JSON object from the body into v, refusing unknown fields,
// trailing data and oversize bodies. It writes the 400 itself and reports whether
// the handler should go on. An empty body is allowed when allowEmpty is set (all
// fields optional).
func decodeJSON(w http.ResponseWriter, r *http.Request, v any, allowEmpty bool) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		if errors.Is(err, io.EOF) && allowEmpty {
			return true
		}
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "too_large", "request body too large")
			return false
		}
		writeError(w, http.StatusBadRequest, "bad_json", "the body must be a JSON object with known fields: "+err.Error())
		return false
	}
	if dec.More() {
		writeError(w, http.StatusBadRequest, "bad_json", "unexpected data after the JSON object")
		return false
	}
	return true
}

// writeState answers with the bot's state as it is right now. Every mutating
// endpoint ends here, so a client can apply the response instead of polling.
// A control that acts asynchronously (a skip interrupts audio first) may not be
// visible yet; the next poll shows it.
func (s *Server) writeState(w http.ResponseWriter, code int, id Identity) {
	st := toStateJSON(s.bot.Snapshot(), id)
	w.Header().Set("ETag", `"`+st.Version+`"`)
	writeJSON(w, code, st)
}

// fail turns an error from the bot into the matching HTTP error. Unknown errors
// are logged and reported without detail.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, queue.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "that queue entry no longer exists")
	case errors.Is(err, ErrNothingPlaying):
		writeError(w, http.StatusConflict, "nothing_playing", err.Error())
	case errors.Is(err, ErrNotSeekable):
		writeError(w, http.StatusConflict, "not_seekable", err.Error())
	case errors.Is(err, ErrSeekRange):
		writeError(w, http.StatusBadRequest, "seek_range", err.Error())
	case errors.Is(err, ErrBadSource):
		writeError(w, http.StatusUnprocessableEntity, "invalid_source", err.Error())
	case errors.Is(err, ErrBusy):
		writeError(w, http.StatusServiceUnavailable, "busy", err.Error())
	case errors.Is(err, library.ErrUnavailable):
		writeError(w, http.StatusConflict, "library_unavailable", "no music library is configured")
	case errors.Is(err, library.ErrEmpty):
		writeError(w, http.StatusConflict, "library_empty", err.Error())
	case errors.Is(err, library.ErrNotInLibrary):
		writeError(w, http.StatusNotFound, "not_in_library", "that file is not in the library")
	case errors.Is(err, library.ErrNotIndexable):
		writeError(w, http.StatusUnprocessableEntity, "invalid_file", err.Error())
	case errors.Is(err, library.ErrTooLarge), isTooBig(err):
		writeError(w, http.StatusRequestEntityTooLarge, "too_large", "the file is too large")
	case errors.Is(err, library.ErrFolderUnavailable):
		writeError(w, http.StatusConflict, "library_unavailable", "the music folder is not available")
	case errors.Is(err, library.ErrScanRunning):
		writeError(w, http.StatusConflict, "scan_running", "a scan is already running")
	case errors.Is(err, urlitem.ErrBinaryMissing):
		s.log.Error("web: yt-dlp is not available", slog.String("err", err.Error()))
		writeError(w, http.StatusServiceUnavailable, "unavailable", "URL playback is not available on this server")
	case errors.Is(err, context.DeadlineExceeded):
		writeError(w, http.StatusGatewayTimeout, "timeout", "the check took too long")
	case errors.Is(err, context.Canceled):
		// The client went away; nobody reads the answer.
		writeError(w, http.StatusRequestTimeout, "canceled", "request canceled")
	default:
		s.log.Error("web: request failed", slog.String("method", r.Method), slog.String("path", r.URL.Path),
			slog.String("err", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal", "internal error")
	}
}

// entryID reads the {id} path value. A malformed ID cannot name an entry, so it is
// reported like a missing one.
func entryID(r *http.Request) (uint64, bool) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	return id, err == nil && id != 0
}

func isNotFound(err error) bool { return errors.Is(err, queue.ErrNotFound) }

// isTooBig reports whether err comes from http.MaxBytesReader.
func isTooBig(err error) bool {
	var tooBig *http.MaxBytesError
	return errors.As(err, &tooBig)
}
