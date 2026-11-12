package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/konradk/gotamusique/internal/config"
	"github.com/konradk/gotamusique/internal/library"
	"github.com/konradk/gotamusique/internal/queue"
	"github.com/konradk/gotamusique/internal/radio"
	"github.com/konradk/gotamusique/internal/status"
)

// do sends a request with the logged-in session and an optional JSON body.
func (e *env) do(t *testing.T, method, path, body string, hdr ...string) *http.Response {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req, _ := http.NewRequest(method, e.ts.URL+path, rd)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		if hdr[i] == "Host" { // net/http ignores a Host entry in Header
			req.Host = hdr[i+1]
			continue
		}
		req.Header.Set(hdr[i], hdr[i+1])
	}
	resp, err := e.c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func errCode(t *testing.T, resp *http.Response) string {
	t.Helper()
	var b errorBody
	if err := json.NewDecoder(resp.Body).Decode(&b); err != nil {
		t.Fatalf("not an error body: %v", err)
	}
	return b.Code
}

func loggedIn(t *testing.T) *env {
	t.Helper()
	e := newEnv(t, Options{AuthMethod: config.AuthToken})
	e.login(t, alice)
	return e
}

func wantCalls(t *testing.T, e *env, want ...string) {
	t.Helper()
	got := e.bot.called()
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("bot calls = %v, want %v", got, want)
	}
}

// ---- access ----

func TestMutations_RequireLogin(t *testing.T) {
	e := newEnv(t, Options{AuthMethod: config.AuthToken})
	for _, ep := range [][2]string{
		{"POST", "/api/v1/playback/pause"}, {"POST", "/api/v1/playback/resume"},
		{"POST", "/api/v1/playback/stop"}, {"POST", "/api/v1/playback/next"},
		{"POST", "/api/v1/playback/seek"}, {"PUT", "/api/v1/volume"}, {"PUT", "/api/v1/mode"},
		{"POST", "/api/v1/queue"}, {"DELETE", "/api/v1/queue"}, {"POST", "/api/v1/queue/shuffle"},
		{"DELETE", "/api/v1/queue/1"}, {"POST", "/api/v1/queue/1/play"}, {"POST", "/api/v1/queue/1/move"},
		{"GET", "/api/v1/radio/search?q=jazz"}, {"POST", "/api/v1/library/rescan"},
	} {
		if resp := e.do(t, ep[0], ep[1], "{}"); resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s without a session = %d, want 401", ep[0], ep[1], resp.StatusCode)
		}
	}
	if got := e.bot.called(); len(got) != 0 {
		t.Errorf("the bot was called without a login: %v", got)
	}
}

// ---- playback ----

func TestPlaybackActionsCallTheBotAndReturnTheState(t *testing.T) {
	e := loggedIn(t)
	for path, call := range map[string]string{
		"/api/v1/playback/pause":  "pause",
		"/api/v1/playback/resume": "resume",
		"/api/v1/playback/stop":   "stop",
		"/api/v1/playback/next":   "next",
		"/api/v1/queue/shuffle":   "shuffle",
	} {
		e.bot.calls = nil
		resp := e.do(t, "POST", path, "")
		if resp.StatusCode != http.StatusOK {
			t.Errorf("%s = %d", path, resp.StatusCode)
			continue
		}
		if s := decodeState(t, resp); s.Version != "7" || len(s.Queue) != 2 {
			t.Errorf("%s did not return the state: %+v", path, s)
		}
		if resp.Header.Get("ETag") != `"7"` {
			t.Errorf("%s: ETag = %q", path, resp.Header.Get("ETag"))
		}
		wantCalls(t, e, call)
	}
	e.bot.calls = nil
	if resp := e.do(t, "DELETE", "/api/v1/queue", ""); resp.StatusCode != http.StatusOK {
		t.Errorf("clear = %d", resp.StatusCode)
	}
	wantCalls(t, e, "clear")
}

func TestSeek(t *testing.T) {
	e := loggedIn(t)
	if resp := e.do(t, "POST", "/api/v1/playback/seek", `{"position_ms": 90500}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("seek = %d", resp.StatusCode)
	}
	wantCalls(t, e, "seek 90500ms")

	for name, body := range map[string]string{
		"missing":   `{}`,
		"not json":  `seek please`,
		"unknown":   `{"position_ms": 1, "extra": 2}`,
		"a string":  `{"position_ms": "5"}`,
		"too large": `{"position_ms": 9223372036854775807}`,
		"negative":  `{"position_ms": -1}`,
		// -2^63 ms overflows time.Duration to exactly 0, which would restart the track.
		"min int64": `{"position_ms": -9223372036854775808}`,
		"trailing":  `{"position_ms": 1} {"x":1}`,
		"empty":     ``,
	} {
		e.bot.calls = nil
		resp := e.do(t, "POST", "/api/v1/playback/seek", body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", name, resp.StatusCode)
		}
		if got := e.bot.called(); len(got) != 0 {
			t.Errorf("%s: a bad request reached the bot: %v", name, got)
		}
	}
}

func TestSeek_BotErrorsMapToStatuses(t *testing.T) {
	for err, want := range map[error][2]any{
		ErrNothingPlaying: {http.StatusConflict, "nothing_playing"},
		ErrNotSeekable:    {http.StatusConflict, "not_seekable"},
		ErrSeekRange:      {http.StatusBadRequest, "seek_range"},
	} {
		e := loggedIn(t)
		e.bot.seekErr = err
		resp := e.do(t, "POST", "/api/v1/playback/seek", `{"position_ms": 1000}`)
		if resp.StatusCode != want[0] || errCode(t, resp) != want[1] {
			t.Errorf("%v: status %d, want %v", err, resp.StatusCode, want)
		}
	}
}

// ---- volume and mode ----

func TestVolume(t *testing.T) {
	e := loggedIn(t)
	for body, calls := range map[string][]string{
		`{"volume": 0.4}`:                 {"volume 40"},
		`{"volume": 0.456}`:               {"volume 46"},
		`{"volume": 0}`:                   {"volume 0"},
		`{"volume": 1}`:                   {"volume 100"},
		`{"muted": true}`:                 {"mute"},
		`{"muted": false}`:                {"unmute"},
		`{"volume": 0.5, "muted": true}`:  {"volume 50", "mute"}, // volume first: setting it ends a mute
		`{"volume": 0.5, "muted": false}`: {"volume 50", "unmute"},
	} {
		e.bot.calls = nil
		if resp := e.do(t, "PUT", "/api/v1/volume", body); resp.StatusCode != http.StatusOK {
			t.Errorf("%s = %d", body, resp.StatusCode)
		}
		wantCalls(t, e, calls...)
	}
	for _, body := range []string{`{}`, `{"volume": 1.5}`, `{"volume": -0.1}`, `{"volume": "loud"}`, `{"vol": 1}`} {
		e.bot.calls = nil
		if resp := e.do(t, "PUT", "/api/v1/volume", body); resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", body, resp.StatusCode)
		}
		wantCalls(t, e)
	}
}

func TestMode(t *testing.T) {
	e := loggedIn(t)
	for _, m := range []string{"one-shot", "repeat", "random", "autoplay"} {
		e.bot.calls = nil
		if resp := e.do(t, "PUT", "/api/v1/mode", fmt.Sprintf(`{"mode":%q}`, m)); resp.StatusCode != http.StatusOK {
			t.Errorf("%s = %d", m, resp.StatusCode)
		}
		wantCalls(t, e, "mode "+m)
	}
	e.bot.calls = nil
	resp := e.do(t, "PUT", "/api/v1/mode", `{"mode":"shuffle-all"}`)
	if resp.StatusCode != http.StatusBadRequest || errCode(t, resp) != "invalid_mode" {
		t.Errorf("unknown mode = %d", resp.StatusCode)
	}
	wantCalls(t, e)

	e.bot.modeErr = library.ErrUnavailable // autoplay without a library
	resp = e.do(t, "PUT", "/api/v1/mode", `{"mode":"autoplay"}`)
	if resp.StatusCode != http.StatusConflict || errCode(t, resp) != "library_unavailable" {
		t.Errorf("autoplay without a library = %d", resp.StatusCode)
	}
}

// ---- queue entries ----

func TestQueueEntryEndpoints(t *testing.T) {
	e := loggedIn(t)
	for _, c := range []struct{ method, path, body, call string }{
		{"DELETE", "/api/v1/queue/5", "", "remove 5"},
		{"POST", "/api/v1/queue/5/play", "", "play 5"},
		{"POST", "/api/v1/queue/5/move", `{"after": 9}`, "move 5 after 9"},
		{"POST", "/api/v1/queue/5/move", `{"after": 0}`, "move 5 after 0"}, // to the front
	} {
		e.bot.calls = nil
		resp := e.do(t, c.method, c.path, c.body)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("%s %s = %d", c.method, c.path, resp.StatusCode)
		}
		wantCalls(t, e, c.call)
	}
}

// A missing entry is "already done" for the client: 404, and nothing else changed.
func TestQueueEntryEndpoints_MissingEntryIs404(t *testing.T) {
	e := loggedIn(t)
	e.bot.removeErr, e.bot.playErr, e.bot.moveErr = queue.ErrNotFound, queue.ErrNotFound, queue.ErrNotFound
	for _, c := range []struct{ method, path, body string }{
		{"DELETE", "/api/v1/queue/5", ""},
		{"POST", "/api/v1/queue/5/play", ""},
		{"POST", "/api/v1/queue/5/move", `{"after": 1}`},
	} {
		resp := e.do(t, c.method, c.path, c.body)
		if resp.StatusCode != http.StatusNotFound || errCode(t, resp) != "not_found" {
			t.Errorf("%s %s = %d, want 404 not_found", c.method, c.path, resp.StatusCode)
		}
	}
}

func TestQueueEntryEndpoints_MalformedIDNeverReachesTheBot(t *testing.T) {
	e := loggedIn(t)
	for _, id := range []string{"abc", "0", "-1", "1.5", "99999999999999999999999"} {
		for _, c := range [][2]string{{"DELETE", ""}, {"POST", "/play"}, {"POST", "/move"}} {
			resp := e.do(t, c[0], "/api/v1/queue/"+id+c[1], `{"after":1}`)
			if resp.StatusCode != http.StatusNotFound {
				t.Errorf("%s /queue/%s%s = %d, want 404", c[0], id, c[1], resp.StatusCode)
			}
		}
	}
	wantCalls(t, e)
}

func TestMove_NeedsAfter(t *testing.T) {
	e := loggedIn(t)
	for _, body := range []string{`{}`, ``, `{"after": -1}`, `{"after": "x"}`} {
		if resp := e.do(t, "POST", "/api/v1/queue/5/move", body); resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%q = %d, want 400", body, resp.StatusCode)
		}
	}
	wantCalls(t, e)
}

// ---- add ----

func TestAdd_BuildsTheSourceAndReturnsStateAndEntry(t *testing.T) {
	e := loggedIn(t)
	e.bot.added = Added{ID: 42, Title: "[Radio] Jazz"}

	resp := e.do(t, "POST", "/api/v1/queue", `{"type":"radio","preset":"jazz"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("add = %d", resp.StatusCode)
	}
	var body struct {
		stateJSON
		Added entryRefJSON `json:"added"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Added.ID != 42 || body.Added.Title != "[Radio] Jazz" || body.Version != "7" || len(body.Queue) != 2 {
		t.Errorf("body = %+v", body)
	}
	if e.bot.addSrc != (Source{Type: "radio", Preset: "jazz"}) || e.bot.addPos != queue.AtEnd {
		t.Errorf("source %+v at %v", e.bot.addSrc, e.bot.addPos)
	}
	if !e.bot.addDeadline {
		t.Error("the bot must get a deadline: the checks hit the network")
	}
	wantCalls(t, e, "add radio")
}

func TestAdd_PositionsAndSourceFields(t *testing.T) {
	cases := []struct {
		body    string
		src     Source
		pos     queue.Position
		playNow bool
	}{
		{`{"type":"library","path":"a/b.mp3"}`, Source{Type: "library", Path: "a/b.mp3"}, queue.AtEnd, false},
		{`{"type":"url","url":"https://x/y","position":"end"}`, Source{Type: "url", URL: "https://x/y"}, queue.AtEnd, false},
		{`{"type":"radio","uuid":"u-1","position":"next"}`, Source{Type: "radio", UUID: "u-1"}, queue.AfterCurrent, false},
		{`{"type":"radio","url":"http://r/s","position":"now"}`, Source{Type: "radio", URL: "http://r/s"}, queue.AfterCurrent, true},
	}
	for _, c := range cases {
		e := loggedIn(t)
		e.bot.added = Added{ID: 9, Title: "t"}
		if resp := e.do(t, "POST", "/api/v1/queue", c.body); resp.StatusCode != http.StatusCreated {
			t.Fatalf("%s = %d", c.body, resp.StatusCode)
		}
		if e.bot.addSrc != c.src || e.bot.addPos != c.pos {
			t.Errorf("%s: source %+v at %v", c.body, e.bot.addSrc, e.bot.addPos)
		}
		wantPlay := []string{"add " + c.src.Type}
		if c.playNow {
			wantPlay = append(wantPlay, "play 9")
		}
		wantCalls(t, e, wantPlay...)
	}
}

func TestAdd_NowIgnoresAnEntryThatVanished(t *testing.T) {
	e := loggedIn(t)
	e.bot.added = Added{ID: 9, Title: "t"}
	e.bot.playErr = queue.ErrNotFound // removed by someone else between add and play
	if resp := e.do(t, "POST", "/api/v1/queue", `{"type":"url","url":"https://x/y","position":"now"}`); resp.StatusCode != http.StatusCreated {
		t.Errorf("= %d, want 201: the add itself worked", resp.StatusCode)
	}
}

func TestAdd_Rejections(t *testing.T) {
	for _, body := range []string{
		`{"type":"url","url":"https://x/y","position":"first"}`,
		`{"type":"url","url":"https://x/y","extra":true}`,
		`not json`, ``, `[]`,
	} {
		e := loggedIn(t)
		if resp := e.do(t, "POST", "/api/v1/queue", body); resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%q = %d, want 400", body, resp.StatusCode)
		}
		wantCalls(t, e)
	}
}

func TestAdd_BotErrorsMapToStatuses(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{fmt.Errorf("%w: unknown radio preset %q", ErrBadSource, "x"), http.StatusUnprocessableEntity, "invalid_source"},
		{ErrBusy, http.StatusServiceUnavailable, "busy"},
		{library.ErrUnavailable, http.StatusConflict, "library_unavailable"},
		{library.ErrNotInLibrary, http.StatusNotFound, "not_in_library"},
		{context.DeadlineExceeded, http.StatusGatewayTimeout, "timeout"},
		{errors.New("disk on fire: /secret/path"), http.StatusInternalServerError, "internal"},
	}
	for _, c := range cases {
		e := loggedIn(t)
		e.bot.addErr = c.err
		resp := e.do(t, "POST", "/api/v1/queue", `{"type":"library","path":"a.mp3"}`)
		raw, _ := io.ReadAll(resp.Body)
		var b errorBody
		json.Unmarshal(raw, &b)
		if resp.StatusCode != c.status || b.Code != c.code {
			t.Errorf("%v: %d %s, want %d %s", c.err, resp.StatusCode, b.Code, c.status, c.code)
		}
		if strings.Contains(string(raw), "/secret/path") {
			t.Errorf("an internal error leaked its detail: %s", raw)
		}
	}
	// A bad source tells the user why.
	e := loggedIn(t)
	e.bot.addErr = fmt.Errorf("%w: unknown radio preset %q", ErrBadSource, "nope")
	resp := e.do(t, "POST", "/api/v1/queue", `{"type":"radio","preset":"nope"}`)
	var b errorBody
	json.NewDecoder(resp.Body).Decode(&b)
	if !strings.Contains(b.Error, `unknown radio preset "nope"`) {
		t.Errorf("message = %q", b.Error)
	}
}

// ---- radio search ----

func TestRadioSearch(t *testing.T) {
	e := loggedIn(t)
	e.bot.stations = []radio.Station{
		{UUID: "u1", Name: "Jazz FM", URL: "http://internal-detail", Codec: "MP3", Bitrate: 128, Country: "DE", Tags: "jazz,smooth"},
	}
	resp := e.do(t, "GET", "/api/v1/radio/search?q=+jazz+&limit=500", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("= %d", resp.StatusCode)
	}
	raw, _ := io.ReadAll(resp.Body)
	var out struct{ Stations []stationJSON }
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Stations) != 1 || out.Stations[0] != (stationJSON{UUID: "u1", Name: "Jazz FM", Codec: "MP3", Bitrate: 128, Country: "DE", Tags: "jazz,smooth"}) {
		t.Errorf("stations = %+v", out.Stations)
	}
	if e.bot.searchQ != "jazz" || e.bot.searchLimit != maxSearchLimit {
		t.Errorf("query %q limit %d, want trimmed query and the limit clamped to %d", e.bot.searchQ, e.bot.searchLimit, maxSearchLimit)
	}
	if strings.Contains(string(raw), "internal-detail") {
		t.Error("the station URL must not be exposed; clients add by uuid")
	}

	e.bot.stations = nil
	resp = e.do(t, "GET", "/api/v1/radio/search?q=nothing", "")
	var empty map[string]any
	json.NewDecoder(resp.Body).Decode(&empty)
	if s, ok := empty["stations"].([]any); !ok || len(s) != 0 || e.bot.searchLimit != defaultSearchLimit {
		t.Errorf("empty result = %v (limit %d); want [] and the default limit", empty, e.bot.searchLimit)
	}
}

func TestRadioSearch_Rejections(t *testing.T) {
	e := loggedIn(t)
	for _, q := range []string{"", "q=", "q=%20%20", "q=" + strings.Repeat("a", 101), "q=x&limit=0", "q=x&limit=-3", "q=x&limit=abc"} {
		if resp := e.do(t, "GET", "/api/v1/radio/search?"+q, ""); resp.StatusCode != http.StatusBadRequest {
			t.Errorf("?%s = %d, want 400", q, resp.StatusCode)
		}
	}
	wantCalls(t, e)

	e.bot.searchErr = errors.New("radio-browser is down: dial tcp 1.2.3.4")
	resp := e.do(t, "GET", "/api/v1/radio/search?q=x", "")
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusInternalServerError || strings.Contains(string(raw), "1.2.3.4") {
		t.Errorf("search failure = %d %s", resp.StatusCode, raw)
	}
}

// ---- rescan ----

func TestRescan(t *testing.T) {
	e := loggedIn(t)
	e.bot.snap.Scan = status.Scan{Available: true, Running: true}
	resp := e.do(t, "POST", "/api/v1/library/rescan", "")
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("= %d, want 202", resp.StatusCode)
	}
	if s := decodeState(t, resp); !s.Scan.Running {
		t.Errorf("the answer must already show the scan running: %+v", s.Scan)
	}
	wantCalls(t, e, "rescan")

	e.bot.rescanErr = library.ErrScanRunning
	resp = e.do(t, "POST", "/api/v1/library/rescan", "")
	if resp.StatusCode != http.StatusConflict || errCode(t, resp) != "scan_running" {
		t.Errorf("second scan = %d", resp.StatusCode)
	}
	e.bot.rescanErr = library.ErrUnavailable
	resp = e.do(t, "POST", "/api/v1/library/rescan", "")
	if resp.StatusCode != http.StatusConflict || errCode(t, resp) != "library_unavailable" {
		t.Errorf("no library = %d", resp.StatusCode)
	}
}

func TestState_ShowsScanOutcome(t *testing.T) {
	e := loggedIn(t)
	e.bot.snap.Scan = status.Scan{Available: true, LastResult: "3 added, 0 updated, 1 removed"}
	if s := decodeState(t, e.get(t, "/api/v1/state")); s.Scan.LastResult != "3 added, 0 updated, 1 removed" || s.Scan.LastError != "" {
		t.Errorf("scan = %+v", s.Scan)
	}
}

// ---- request hygiene ----

func TestBodyLimits(t *testing.T) {
	e := loggedIn(t)
	big := `{"type":"url","url":"https://x/` + strings.Repeat("a", maxBody) + `"}`
	if resp := e.do(t, "POST", "/api/v1/queue", big); resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("oversize body = %d, want 413", resp.StatusCode)
	}
	wantCalls(t, e)
}

// ---- cross-site requests ----

func TestCrossSiteMutationsAreRefused(t *testing.T) {
	for name, o := range map[string]Options{
		"no login": {AuthMethod: config.AuthNone, Addr: "127.0.0.1:8181", AccessAddress: "http://127.0.0.1:8181"},
		"token":    {AuthMethod: config.AuthToken, AccessAddress: "https://music.example.com"},
	} {
		e := newEnv(t, o)
		if o.AuthMethod == config.AuthToken {
			e.login(t, alice)
		}
		for _, hdr := range [][]string{
			{"Origin", "https://evil.example"},
			{"Origin", "null"},
			{"Origin", "not a url"},
			{"Sec-Fetch-Site", "cross-site"},
			{"Sec-Fetch-Site", "same-site"},
		} {
			args := append([]string{}, hdr...)
			if o.AuthMethod == config.AuthNone {
				args = append(args, "Host", "127.0.0.1:8181")
			}
			e.bot.calls = nil
			resp := e.do(t, "POST", "/api/v1/playback/pause", "", args...)
			if resp.StatusCode != http.StatusForbidden || errCode(t, resp) != "cross_origin" {
				t.Errorf("%s %v = %d, want 403 cross_origin", name, hdr, resp.StatusCode)
			}
			if got := e.bot.called(); len(got) != 0 {
				t.Errorf("%s %v: the action ran anyway: %v", name, hdr, got)
			}
		}
	}
}

func TestSameSiteMutationsAreAllowed(t *testing.T) {
	e := newEnv(t, Options{AuthMethod: config.AuthNone, Addr: "127.0.0.1:8181", AccessAddress: "https://music.example.com"})
	for _, hdr := range [][]string{
		{},                                      // curl and friends send neither header
		{"Origin", "http://127.0.0.1:8181"},     // the page served by this server
		{"Origin", "https://music.example.com"}, // the public address behind a proxy
		{"Origin", "HTTP://127.0.0.1:8181"},     // scheme/host case
		{"Sec-Fetch-Site", "same-origin"},
		{"Sec-Fetch-Site", "none"}, // typed into the address bar
		{"Origin", "http://127.0.0.1:8181", "Sec-Fetch-Site", "same-origin"},
	} {
		args := append([]string{"Host", "127.0.0.1:8181"}, hdr...)
		if resp := e.do(t, "POST", "/api/v1/playback/pause", "", args...); resp.StatusCode != http.StatusOK {
			t.Errorf("%v = %d, want 200", hdr, resp.StatusCode)
		}
	}
}

func TestCrossSiteReadsAreNotTheGuardsBusiness(t *testing.T) {
	e := loggedIn(t)
	if resp := e.get(t, "/api/v1/state", "Origin", "https://evil.example"); resp.StatusCode != http.StatusOK {
		t.Errorf("GET = %d: reads are protected by the cookie and by the browser not exposing the response", resp.StatusCode)
	}
}

// ---- wiring ----

func TestRoutesDoNotShadowEachOther(t *testing.T) {
	// "shuffle" is a literal under /queue/ next to the {id} wildcard.
	e := loggedIn(t)
	e.do(t, "POST", "/api/v1/queue/shuffle", "")
	e.do(t, "DELETE", "/api/v1/queue/7", "")
	wantCalls(t, e, "shuffle", "remove 7")
}
