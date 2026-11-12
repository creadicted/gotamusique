package web

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/konradk/gotamusique/internal/config"
	"github.com/konradk/gotamusique/internal/library"
)

// handleRescan starts an incremental library scan and answers at once with 202.
// The state's scan field shows it running and, later, how it ended. A scan that
// is already running is a 409. There is deliberately no force option here: it
// clears the library when the folder looks empty.
func (s *Server) handleRescan(w http.ResponseWriter, r *http.Request, id Identity) {
	if err := s.bot.StartRescan(); err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeState(w, http.StatusAccepted, id)
}

// ---- browse and search ----

const (
	defaultLibraryLimit = 100
	maxLibraryLimit     = 500
	maxLibraryText      = 200 // dir and search text
	maxLibraryTags      = 10
	maxTagLen           = 100
)

type trackJSON struct {
	Path       string   `json:"path"`
	Title      string   `json:"title"`
	Artist     string   `json:"artist"`
	Tags       []string `json:"tags"`
	DurationMS int64    `json:"duration_ms"` // 0 when unknown
}

type dirJSON struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Tracks int    `json:"tracks"`
}

type libraryJSON struct {
	Dir    string      `json:"dir"`
	Dirs   []dirJSON   `json:"dirs"`
	Tracks []trackJSON `json:"tracks"`
	Total  int         `json:"total"`
	Offset int         `json:"offset"`
	Limit  int         `json:"limit"`
	Tags   []string    `json:"tags"`
}

func toTrackJSON(t library.Track) trackJSON {
	tags := t.Tags
	if tags == nil {
		tags = []string{}
	}
	return trackJSON{Path: t.Path, Title: t.Title, Artist: t.Artist, Tags: tags, DurationMS: t.Duration.Milliseconds()}
}

// handleLibrary lists the library. Without q and tag it browses `dir` (folders and
// the tracks directly in it); with either it searches everything below `dir`.
// Queue a result with {"type":"library","path":...}.
func (s *Server) handleLibrary(w http.ResponseWriter, r *http.Request, id Identity) {
	q := r.URL.Query()
	query := LibraryQuery{
		Dir:    strings.Trim(strings.TrimSpace(q.Get("dir")), "/"),
		Search: strings.TrimSpace(q.Get("q")),
		Limit:  defaultLibraryLimit,
	}
	if len(query.Dir) > maxLibraryText || len(query.Search) > maxLibraryText {
		writeError(w, http.StatusBadRequest, "bad_request", "dir and q are limited to 200 characters")
		return
	}
	for _, t := range q["tag"] {
		for _, tag := range strings.Split(t, ",") {
			if tag = strings.TrimSpace(tag); tag != "" {
				query.Tags = append(query.Tags, tag)
			}
		}
	}
	if len(query.Tags) > maxLibraryTags {
		writeError(w, http.StatusBadRequest, "bad_request", "at most 10 tags")
		return
	}
	for _, tag := range query.Tags {
		if len(tag) > maxTagLen {
			writeError(w, http.StatusBadRequest, "bad_request", "a tag is limited to 100 characters")
			return
		}
	}
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "bad_request", "limit must be a positive number")
			return
		}
		query.Limit = min(n, maxLibraryLimit)
	}
	if raw := q.Get("offset"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "bad_request", "offset must be zero or more")
			return
		}
		query.Offset = n
	}

	page, err := s.bot.Library(query)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := libraryJSON{
		Dir: query.Dir, Offset: query.Offset, Limit: query.Limit, Total: page.Total,
		Dirs: make([]dirJSON, len(page.Dirs)), Tracks: make([]trackJSON, len(page.Tracks)),
		Tags: page.Tags,
	}
	if out.Tags == nil {
		out.Tags = []string{}
	}
	for i, d := range page.Dirs {
		out.Dirs[i] = dirJSON{Name: d.Name, Path: d.Path, Tracks: d.Tracks}
	}
	for i, t := range page.Tracks {
		out.Tracks[i] = toTrackJSON(t)
	}
	writeJSON(w, http.StatusOK, out)
}

// ---- upload ----

const (
	defaultMaxUpload = 30 << 20
	// multipartOverhead is what a multipart body may add to the file itself
	// (boundaries, headers, small form fields).
	multipartOverhead = 1 << 20
	// uploadTimeout bounds one upload, however slow the client. The server-wide
	// timeouts are far too short for a large file.
	uploadTimeout = 10 * time.Minute
	// maxConcurrentUploads caps uploads in progress at once.
	maxConcurrentUploads = 4
)

// handleUpload takes one audio file as multipart/form-data in the field "file"
// and adds it to the library. The body is streamed to disk, never held in memory.
// It answers 201 with the new library entry; queue it with a follow-up add.
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request, id Identity) {
	if !s.opts.UploadEnabled {
		writeError(w, http.StatusForbidden, "upload_disabled", "uploads are disabled on this server")
		return
	}
	if s.opts.AuthMethod == config.AuthNone && !s.opts.AllowUploadWithoutAuth {
		writeError(w, http.StatusForbidden, "upload_requires_login",
			"uploads need a login; this server has none (see allow_upload_without_auth)")
		return
	}
	// Each upload holds a connection, a file descriptor and disk bandwidth for as
	// long as the client takes; a handful at once is plenty for a music bot.
	select {
	case s.uploadSlots <- struct{}{}:
		defer func() { <-s.uploadSlots }()
	default:
		writeError(w, http.StatusServiceUnavailable, "busy", "too many uploads in progress, try again in a moment")
		return
	}
	limit := s.opts.MaxUploadBytes
	if limit <= 0 {
		limit = defaultMaxUpload
	}
	if r.ContentLength > limit+multipartOverhead {
		writeError(w, http.StatusRequestEntityTooLarge, "too_large", "the file is too large")
		return
	}

	// net/http applies the server-wide timeouts to the whole request; give this one
	// its own. Not every ResponseWriter supports it (tests), which is fine.
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Now().Add(uploadTimeout))
	_ = rc.SetWriteDeadline(time.Now().Add(uploadTimeout + 30*time.Second))

	r.Body = http.MaxBytesReader(w, r.Body, limit+multipartOverhead)
	mr, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "send the file as multipart/form-data in the field \"file\"")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), uploadTimeout)
	defer cancel()

	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, "bad_request", "no file: use the form field \"file\"")
			return
		}
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if part.FormName() != "file" || part.FileName() == "" {
			io.Copy(io.Discard, part) //nolint:errcheck // other fields are ignored; the body is size-limited
			continue
		}
		track, err := s.bot.Upload(ctx, id, part.FileName(), part)
		if err != nil {
			// Answering while the client is still sending makes net/http drop the
			// connection, and the browser then shows a reset instead of our error.
			// The body is already size-limited, so reading the rest is bounded.
			io.Copy(io.Discard, r.Body) //nolint:errcheck
			s.fail(w, r, err)
			return
		}
		s.log.Info("upload stored", slog.String("user", id.Name), slog.String("path", track.Path))
		writeJSON(w, http.StatusCreated, struct {
			Track trackJSON `json:"track"`
		}{toTrackJSON(track)})
		return
	}
}
