package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/konradk/gotamusique/internal/config"
	"github.com/konradk/gotamusique/internal/library"
)

// ---- browse and search ----

func TestLibrary_MapsThePage(t *testing.T) {
	e := loggedIn(t)
	e.bot.libPage = LibraryPage{
		Dirs:   []LibraryDir{{Name: "rock", Path: "rock", Tracks: 12}},
		Tracks: []library.Track{{Path: "a.mp3", Title: "A", Artist: "Band", Tags: []string{"x"}, Duration: 90 * time.Second}, {Path: "b.mp3", Title: "B"}},
		Total:  2,
		Tags:   []string{"x", "y"},
	}
	resp := e.do(t, "GET", "/api/v1/library", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("= %d", resp.StatusCode)
	}
	raw, _ := io.ReadAll(resp.Body)
	var out libraryJSON
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Dirs) != 1 || out.Dirs[0] != (dirJSON{Name: "rock", Path: "rock", Tracks: 12}) || out.Total != 2 || len(out.Tracks) != 2 {
		t.Errorf("page = %+v", out)
	}
	if out.Tracks[0].DurationMS != 90000 || out.Tracks[0].Artist != "Band" || out.Tracks[0].Tags[0] != "x" {
		t.Errorf("track = %+v", out.Tracks[0])
	}
	// A track with no tags is [], never null: clients iterate without a nil check.
	if !strings.Contains(string(raw), `"tags":[]`) || strings.Contains(string(raw), "null") {
		t.Errorf("empty lists must be [] not null: %s", raw)
	}
	if out.Limit != defaultLibraryLimit || out.Offset != 0 {
		t.Errorf("limit %d offset %d", out.Limit, out.Offset)
	}
}

func TestLibrary_EmptyResultHasEmptyListsNotNull(t *testing.T) {
	e := loggedIn(t)
	raw, _ := io.ReadAll(e.do(t, "GET", "/api/v1/library", "").Body)
	if strings.Contains(string(raw), "null") {
		t.Errorf("= %s", raw)
	}
}

func TestLibrary_PassesTheQuery(t *testing.T) {
	e := loggedIn(t)
	e.do(t, "GET", "/api/v1/library?dir=/rock/metal/&q=+dark+night+&tag=a,b&tag=c&limit=9999&offset=40", "")
	q := e.bot.libQuery
	if q.Dir != "rock/metal" || q.Search != "dark night" || strings.Join(q.Tags, ",") != "a,b,c" || q.Limit != maxLibraryLimit || q.Offset != 40 {
		t.Errorf("query = %+v", q)
	}
}

func TestLibrary_Rejections(t *testing.T) {
	e := loggedIn(t)
	long := strings.Repeat("a", maxLibraryText+1)
	for _, qs := range []string{
		"limit=0", "limit=-1", "limit=abc", "offset=-1", "offset=x",
		"q=" + long, "dir=" + long,
		"tag=" + strings.Repeat("t", maxTagLen+1),
		"tag=" + strings.Repeat("a,", maxLibraryTags+1) + "z",
	} {
		if resp := e.do(t, "GET", "/api/v1/library?"+qs, ""); resp.StatusCode != http.StatusBadRequest {
			t.Errorf("?%.40s = %d, want 400", qs, resp.StatusCode)
		}
	}
	wantCalls(t, e)
}

func TestLibrary_NoLibraryIs409(t *testing.T) {
	e := loggedIn(t)
	e.bot.libErr = library.ErrUnavailable
	resp := e.do(t, "GET", "/api/v1/library", "")
	if resp.StatusCode != http.StatusConflict || errCode(t, resp) != "library_unavailable" {
		t.Errorf("= %d", resp.StatusCode)
	}
}

// ---- upload ----

// multipartBody builds a multipart body with the given (field, filename, content)
// parts; an empty filename makes a plain form field.
func multipartBody(t *testing.T, parts ...[3]string) (string, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, p := range parts {
		var w io.Writer
		var err error
		if p[1] == "" {
			w, err = mw.CreateFormField(p[0])
		} else {
			w, err = mw.CreateFormFile(p[0], p[1])
		}
		if err != nil {
			t.Fatal(err)
		}
		io.WriteString(w, p[2])
	}
	mw.Close()
	return buf.String(), mw.FormDataContentType()
}

func (e *env) upload(t *testing.T, body, contentType string, hdr ...string) *http.Response {
	t.Helper()
	return e.do(t, "POST", "/api/v1/library/upload", body, append([]string{"Content-Type", contentType}, hdr...)...)
}

func uploadEnv(t *testing.T, o Options) *env {
	t.Helper()
	o.UploadEnabled = true
	e := newEnv(t, o)
	e.login(t, alice)
	e.bot.uploadRead = true
	e.bot.uploadTrack = library.Track{Path: "uploads/alice/song.mp3", Title: "Song", Duration: 3 * time.Second}
	return e
}

func TestUpload_StoresAndReturnsTheEntry(t *testing.T) {
	e := uploadEnv(t, Options{AuthMethod: config.AuthToken})
	body, ct := multipartBody(t, [3]string{"note", "", "ignored form field"}, [3]string{"file", "My Song.mp3", "AUDIO-BYTES"})
	resp := e.upload(t, body, ct)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("= %d", resp.StatusCode)
	}
	var out struct{ Track trackJSON }
	json.NewDecoder(resp.Body).Decode(&out)
	if out.Track.Path != "uploads/alice/song.mp3" || out.Track.DurationMS != 3000 {
		t.Errorf("track = %+v", out.Track)
	}
	if e.bot.uploadName != "My Song.mp3" || e.bot.uploadBody != "AUDIO-BYTES" || e.bot.uploadWho.Name != "alice" {
		t.Errorf("bot got name %q body %q user %q", e.bot.uploadName, e.bot.uploadBody, e.bot.uploadWho.Name)
	}
}

func TestUpload_Gates(t *testing.T) {
	body, ct := multipartBody(t, [3]string{"file", "a.mp3", "x"})
	for name, c := range map[string]struct {
		opts Options
		code string
	}{
		"disabled": {Options{AuthMethod: config.AuthToken}, "upload_disabled"},
		"no login": {Options{AuthMethod: config.AuthNone, Addr: "127.0.0.1:8181", UploadEnabled: true}, "upload_requires_login"},
	} {
		o := c.opts
		e := newEnv(t, o)
		e.srv.opts.UploadEnabled = o.UploadEnabled // newEnv's login() is not needed in none mode
		if o.AuthMethod == config.AuthToken {
			e.login(t, alice)
		}
		args := []string{"Content-Type", ct}
		if o.AuthMethod == config.AuthNone {
			args = append(args, "Host", "127.0.0.1:8181")
		}
		resp := e.do(t, "POST", "/api/v1/library/upload", body, args...)
		if resp.StatusCode != http.StatusForbidden || errCode(t, resp) != c.code {
			t.Errorf("%s: %d, want 403 %s", name, resp.StatusCode, c.code)
		}
		wantCalls(t, e)
	}

	// Explicitly allowed without a login.
	e := newEnv(t, Options{AuthMethod: config.AuthNone, Addr: "127.0.0.1:8181", UploadEnabled: true, AllowUploadWithoutAuth: true})
	e.bot.uploadRead = true
	resp := e.do(t, "POST", "/api/v1/library/upload", body, "Content-Type", ct, "Host", "127.0.0.1:8181")
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("allow_upload_without_auth: %d, want 201", resp.StatusCode)
	}
}

func TestUpload_NeedsALoginInTokenMode(t *testing.T) {
	e := newEnv(t, Options{AuthMethod: config.AuthToken, UploadEnabled: true})
	body, ct := multipartBody(t, [3]string{"file", "a.mp3", "x"})
	if resp := e.upload(t, body, ct); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("= %d, want 401", resp.StatusCode)
	}
	wantCalls(t, e)
}

func TestUpload_BadRequests(t *testing.T) {
	e := uploadEnv(t, Options{AuthMethod: config.AuthToken})
	noFile, ct1 := multipartBody(t, [3]string{"note", "", "just text"})
	wrongField, ct2 := multipartBody(t, [3]string{"upload", "a.mp3", "x"})
	noName, ct3 := multipartBody(t, [3]string{"file", "", "a plain field named file"})
	for name, c := range map[string][2]string{
		"not multipart": {`{"file":"x"}`, "application/json"},
		"plain text":    {"hello", "text/plain"},
		"no file part":  {noFile, ct1},
		"wrong field":   {wrongField, ct2},
		"file w/o name": {noName, ct3},
		"empty body":    {"", "multipart/form-data; boundary=x"},
		"no boundary":   {"--x\r\n", "multipart/form-data"},
	} {
		resp := e.upload(t, c[0], c[1])
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", name, resp.StatusCode)
		}
	}
	wantCalls(t, e)
}

func TestUpload_BotErrorsMapToStatuses(t *testing.T) {
	body, ct := multipartBody(t, [3]string{"file", "a.mp3", "x"})
	for _, c := range []struct {
		err    error
		status int
		code   string
	}{
		{fmt.Errorf("%w: only audio files are accepted", library.ErrNotIndexable), http.StatusUnprocessableEntity, "invalid_file"},
		{library.ErrTooLarge, http.StatusRequestEntityTooLarge, "too_large"},
		{fmt.Errorf("receiving the file: %w", &http.MaxBytesError{Limit: 1}), http.StatusRequestEntityTooLarge, "too_large"},
		{library.ErrUnavailable, http.StatusConflict, "library_unavailable"},
		{library.ErrFolderUnavailable, http.StatusConflict, "library_unavailable"},
		{errors.New("write /srv/music/uploads: disk full"), http.StatusInternalServerError, "internal"},
	} {
		e := uploadEnv(t, Options{AuthMethod: config.AuthToken})
		e.bot.uploadErr = c.err
		resp := e.upload(t, body, ct)
		raw, _ := io.ReadAll(resp.Body)
		var b errorBody
		json.Unmarshal(raw, &b)
		if resp.StatusCode != c.status || b.Code != c.code {
			t.Errorf("%v: %d %s, want %d %s", c.err, resp.StatusCode, b.Code, c.status, c.code)
		}
		if strings.Contains(string(raw), "/srv/music") {
			t.Errorf("an internal error leaked a path: %s", raw)
		}
	}
}

func TestUpload_RefusesAnOversizeBodyBeforeReadingIt(t *testing.T) {
	e := uploadEnv(t, Options{AuthMethod: config.AuthToken, MaxUploadBytes: 1000})
	big := strings.Repeat("x", 1000+multipartOverhead+100)
	body, ct := multipartBody(t, [3]string{"file", "a.mp3", big})
	resp := e.upload(t, body, ct) // Content-Length is set by the client
	if resp.StatusCode != http.StatusRequestEntityTooLarge || errCode(t, resp) != "too_large" {
		t.Errorf("= %d, want 413", resp.StatusCode)
	}
	wantCalls(t, e)
}

// Without a Content-Length (chunked) the limit still holds while reading.
func TestUpload_BodyLimitHoldsWithoutContentLength(t *testing.T) {
	e := uploadEnv(t, Options{AuthMethod: config.AuthToken, MaxUploadBytes: 1000})
	big := strings.Repeat("x", 1000+multipartOverhead+4096)
	body, ct := multipartBody(t, [3]string{"file", "a.mp3", big})
	req, _ := http.NewRequest("POST", e.ts.URL+"/api/v1/library/upload", io.NopCloser(strings.NewReader(body))) // hides the length
	req.Header.Set("Content-Type", ct)
	resp, err := e.c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("= %d, want 413", resp.StatusCode)
	}
}

func TestUpload_DefaultLimitAppliesWhenNoneIsConfigured(t *testing.T) {
	e := uploadEnv(t, Options{AuthMethod: config.AuthToken}) // MaxUploadBytes 0
	big := strings.Repeat("x", defaultMaxUpload+multipartOverhead+10)
	body, ct := multipartBody(t, [3]string{"file", "a.mp3", big})
	if resp := e.upload(t, body, ct); resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("= %d, want 413", resp.StatusCode)
	}
}

func TestUpload_CrossSiteIsRefused(t *testing.T) {
	e := uploadEnv(t, Options{AuthMethod: config.AuthToken})
	body, ct := multipartBody(t, [3]string{"file", "a.mp3", "x"})
	resp := e.upload(t, body, ct, "Origin", "https://evil.example")
	if resp.StatusCode != http.StatusForbidden || errCode(t, resp) != "cross_origin" {
		t.Errorf("= %d, want 403 cross_origin", resp.StatusCode)
	}
	wantCalls(t, e)
}

// A rejection that never reads the body still answers with the error. The handler
// also drains the rest of the (size-limited) body first, because answering while a
// browser is still sending can make net/http drop the connection and the browser
// then shows a reset instead of our error. Go's own client reads an early response
// while it is still writing, so this test cannot tell whether the drain is there;
// it only pins the answer.
func TestUpload_AnEarlyRejectionStillReachesTheClient(t *testing.T) {
	e := uploadEnv(t, Options{AuthMethod: config.AuthToken, MaxUploadBytes: 64 << 20})
	e.bot.uploadRead = false // rejects at once, like a bad file name
	e.bot.uploadErr = fmt.Errorf("%w: only audio files are accepted", library.ErrNotIndexable)
	body, ct := multipartBody(t, [3]string{"file", "evil.exe", strings.Repeat("x", 600<<10)})
	resp := e.upload(t, body, ct)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("= %d, want 422", resp.StatusCode)
	}
}

// Uploads in progress are capped, so a flood cannot exhaust descriptors or disk I/O.
func TestUpload_ConcurrentUploadsAreCapped(t *testing.T) {
	e := uploadEnv(t, Options{AuthMethod: config.AuthToken})
	for i := 0; i < maxConcurrentUploads; i++ {
		e.srv.uploadSlots <- struct{}{} // as if that many uploads were running
	}
	body, ct := multipartBody(t, [3]string{"file", "a.mp3", "x"})
	resp := e.upload(t, body, ct)
	if resp.StatusCode != http.StatusServiceUnavailable || errCode(t, resp) != "busy" {
		t.Errorf("= %d, want 503 busy", resp.StatusCode)
	}
	wantCalls(t, e)

	<-e.srv.uploadSlots // one finishes
	body, ct = multipartBody(t, [3]string{"file", "a.mp3", "x"})
	if resp := e.upload(t, body, ct); resp.StatusCode != http.StatusCreated {
		t.Errorf("after a slot freed up = %d, want 201", resp.StatusCode)
	}
	if got := len(e.srv.uploadSlots); got != maxConcurrentUploads-1 {
		t.Errorf("a finished upload must give its slot back; %d held", got)
	}
}

// The limit plus the multipart headroom must not wrap around: with the largest
// possible setting an ordinary upload still works, and the arithmetic saturates.
func TestUpload_AHugeLimitDoesNotOverflow(t *testing.T) {
	if got := bodyLimit(math.MaxInt64); got != math.MaxInt64 {
		t.Errorf("bodyLimit(MaxInt64) = %d, want it to saturate", got)
	}
	if got := bodyLimit(math.MaxInt64 - multipartOverhead + 1); got != math.MaxInt64 {
		t.Errorf("just past the edge = %d, want saturation", got)
	}
	if got := bodyLimit(1000); got != 1000+multipartOverhead {
		t.Errorf("bodyLimit(1000) = %d", got)
	}

	e := uploadEnv(t, Options{AuthMethod: config.AuthToken, MaxUploadBytes: math.MaxInt64})
	body, ct := multipartBody(t, [3]string{"file", "a.mp3", "audio"})
	if resp := e.upload(t, body, ct); resp.StatusCode != http.StatusCreated {
		t.Errorf("an ordinary upload with the largest limit = %d, want 201 (the limit wrapped negative?)", resp.StatusCode)
	}
}
