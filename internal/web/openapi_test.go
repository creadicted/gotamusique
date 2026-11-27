package web

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"

	"github.com/konradk/gotamusique/internal/config"
	"github.com/konradk/gotamusique/internal/library"
	"github.com/konradk/gotamusique/internal/queue"
	"github.com/konradk/gotamusique/internal/radio"
	"github.com/konradk/gotamusique/internal/status"
)

// The OpenAPI description is written by hand. These tests are what keeps it true:
// it must be valid, list exactly the routes the server has, declare exactly the
// error codes the handlers produce, and describe the real responses.

func init() {
	// kin-openapi parses application/yaml into an object, but this API's YAML and HTML
	// responses are documents to be read as text, and the spec says `type: string`.
	text := func(body io.Reader, _ http.Header, _ *openapi3.SchemaRef, _ openapi3filter.EncodingFn) (any, error) {
		b, err := io.ReadAll(body)
		return string(b), err
	}
	openapi3filter.RegisterBodyDecoder("application/yaml", text)
	openapi3filter.RegisterBodyDecoder("text/html", text)
}

func loadSpec(t *testing.T) *openapi3.T {
	t.Helper()
	doc, err := openapi3.NewLoader().LoadFromData(specYAML)
	if err != nil {
		t.Fatalf("openapi.yaml does not load: %v", err)
	}
	return doc
}

func TestOpenAPI_IsValid(t *testing.T) {
	if err := loadSpec(t).Validate(context.Background()); err != nil {
		t.Fatalf("openapi.yaml is not a valid OpenAPI document: %v", err)
	}
}

type opKey struct{ method, path string }

func (k opKey) String() string { return k.method + " " + k.path }

func specOps(doc *openapi3.T) map[opKey]*openapi3.Operation {
	out := map[opKey]*openapi3.Operation{}
	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			out[opKey{method, path}] = op
		}
	}
	return out
}

// Every API route is in the spec and every operation in the spec is a route.
func TestOpenAPI_MatchesTheRouteTable(t *testing.T) {
	doc := loadSpec(t)
	spec := specOps(doc)
	s := NewServer(&fakeBot{}, NewAuth(AuthOptions{}), Options{AuthMethod: config.AuthToken})

	inCode := map[opKey]bool{}
	for _, r := range s.routes() {
		if r.internal {
			continue
		}
		k := opKey{r.method, r.path}
		inCode[k] = true
		if _, ok := spec[k]; !ok {
			t.Errorf("route %s is not described in openapi.yaml", k)
		}
	}
	for k := range spec {
		if !inCode[k] {
			t.Errorf("openapi.yaml describes %s, which is not a route", k)
		}
	}
	if len(inCode) == 0 {
		t.Fatal("no API routes found")
	}
}

// Every operation has what a client needs to find it: an id, a summary and a tag.
func TestOpenAPI_OperationsAreDescribed(t *testing.T) {
	seen := map[string]opKey{}
	for k, op := range specOps(loadSpec(t)) {
		if op.OperationID == "" || op.Summary == "" || len(op.Tags) == 0 {
			t.Errorf("%s needs an operationId, a summary and a tag", k)
		}
		if other, dup := seen[op.OperationID]; dup {
			t.Errorf("operationId %q is used by %s and %s", op.OperationID, other, k)
		}
		seen[op.OperationID] = k
	}
}

var errorCodeRE = regexp.MustCompile(`writeError\(\s*\w+,\s*http\.Status\w+,\s*"([a-z_]+)"`)

// The Error.code enum is exactly the set of codes the handlers can return.
func TestOpenAPI_ErrorCodesMatchTheHandlers(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil || len(files) == 0 {
		t.Fatal("cannot read the package sources", err)
	}
	inCode := map[string]bool{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, _ := os.ReadFile(f)
		for _, m := range errorCodeRE.FindAllStringSubmatch(string(src), -1) {
			inCode[m[1]] = true
		}
	}
	if len(inCode) < 10 {
		t.Fatalf("found only %d error codes in the sources; the pattern must be out of date", len(inCode))
	}

	prop := loadSpec(t).Components.Schemas["Error"].Value.Properties["code"].Value
	inSpec := map[string]bool{}
	for _, v := range prop.Enum {
		inSpec[v.(string)] = true
	}
	for c := range inCode {
		if !inSpec[c] {
			t.Errorf("the handlers return error code %q, which Error.code does not list", c)
		}
	}
	for c := range inSpec {
		if !inCode[c] {
			t.Errorf("Error.code lists %q, which no handler returns", c)
		}
	}
}

// Enumerations that exist in code are the same in the spec.
func TestOpenAPI_EnumsMatchTheCode(t *testing.T) {
	doc := loadSpec(t)
	enum := func(s *openapi3.SchemaRef) []string {
		var out []string
		for _, v := range s.Value.Enum {
			out = append(out, fmt.Sprint(v))
		}
		sort.Strings(out)
		return out
	}
	want := func(vals ...string) string { sort.Strings(vals); return strings.Join(vals, ",") }

	var modes []string
	for _, m := range queue.Modes {
		modes = append(modes, string(m))
	}
	sch := doc.Components.Schemas
	for name, got := range map[string][]string{
		"ModeRequest.mode": enum(sch["ModeRequest"].Value.Properties["mode"]),
		"State.mode":       enum(sch["State"].Value.Properties["mode"]),
	} {
		if strings.Join(got, ",") != want(modes...) {
			t.Errorf("%s = %v, the code has %v", name, got, modes)
		}
	}
	if got := strings.Join(enum(sch["AddRequest"].Value.Properties["type"]), ","); got != want(SourceLibrary, SourceRadio, SourceURL) {
		t.Errorf("AddRequest.type = %s", got)
	}
	if got := strings.Join(enum(sch["State"].Value.Properties["me"].Value.Properties["roles"].Value.Items), ","); got != want(string(RoleUser), string(RoleAdmin)) {
		t.Errorf("me.roles = %s", got)
	}
	if got := strings.Join(enum(sch["AddRequest"].Value.Properties["position"]), ","); got != want("end", "next", "now") {
		t.Errorf("AddRequest.position = %s (the handler accepts end, next, now)", got)
	}
	if got := strings.Join(enum(sch["State"].Value.Properties["pause"]), ","); got != want("", "manual", "auto") {
		t.Errorf("State.pause = %q", got)
	}
}

// ---- real responses against the schemas ----

// fullSnap is a state with every field filled, so the schema check is not satisfied
// by empty values.
func fullSnap() status.Snapshot {
	s := playingSnap()
	s.Pause = ""
	s.Scan = status.Scan{Available: true, Running: false, LastResult: "1 added, 0 updated, 0 removed"}
	return s
}

// exchange sends a request through the server with the logged-in session and
// validates both the request and the response against the spec.
type exchange struct {
	t      *testing.T
	doc    *openapi3.T
	ops    map[opKey]*openapi3.Operation
	e      *env
	tested map[opKey]bool
}

func newExchange(t *testing.T, e *env) *exchange {
	doc := loadSpec(t)
	return &exchange{t: t, doc: doc, ops: specOps(doc), e: e, tested: map[opKey]bool{}}
}

// do runs METHOD specPath (with {id} etc. already substituted into realPath) and
// returns the status and decoded body. wantStatus must be declared for the operation
// explicitly, not only through "default", so the spec keeps naming the errors it knows.
func (x *exchange) do(key opKey, realPath, contentType string, body []byte, wantStatus int, hdr ...string) []byte {
	x.t.Helper()
	op, ok := x.ops[key]
	if !ok {
		x.t.Fatalf("%s is not in the spec", key)
	}
	if op.Responses.Status(wantStatus) == nil {
		x.t.Errorf("%s: status %d is not declared in the spec (only `default`)", key, wantStatus)
	}
	x.tested[key] = true

	send := func() (*http.Request, error) {
		var rd io.Reader
		if body != nil {
			rd = bytes.NewReader(body)
		}
		req, err := http.NewRequest(key.method, x.e.ts.URL+realPath, rd)
		if err != nil {
			return nil, err
		}
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		for i := 0; i+1 < len(hdr); i += 2 {
			req.Header.Set(hdr[i], hdr[i+1])
		}
		return req, nil
	}

	route := &routers.Route{Spec: x.doc, Path: key.path, PathItem: x.doc.Paths.Find(key.path), Method: key.method, Operation: op}
	opts := &openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc, IncludeResponseStatus: true}

	// The request must satisfy the spec (a separate copy: validation reads the body).
	vreq, _ := send()
	pathParams := map[string]string{}
	if i := strings.Index(key.path, "{id}"); i >= 0 {
		pathParams["id"] = strings.Split(strings.TrimPrefix(realPath[i:], "/"), "/")[0]
	}
	reqIn := &openapi3filter.RequestValidationInput{Request: vreq, PathParams: pathParams, Route: route, Options: opts}
	// A request that provokes a 400 is invalid on purpose, so only the others have to
	// satisfy the spec.
	if wantStatus != http.StatusBadRequest {
		if err := openapi3filter.ValidateRequest(context.Background(), reqIn); err != nil {
			x.t.Errorf("%s: the request does not satisfy the spec: %v", key, err)
		}
	}

	req, _ := send()
	resp, err := x.e.c.Do(req)
	if err != nil {
		x.t.Fatalf("%s: %v", key, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != wantStatus {
		x.t.Fatalf("%s: status %d, want %d\n%s", key, resp.StatusCode, wantStatus, raw)
	}

	respIn := &openapi3filter.ResponseValidationInput{RequestValidationInput: reqIn, Status: resp.StatusCode, Header: resp.Header, Options: opts}
	respIn.SetBodyBytes(raw)
	if err := openapi3filter.ValidateResponse(context.Background(), respIn); err != nil {
		x.t.Errorf("%s -> %d: the response does not match the spec: %v\n%s", key, resp.StatusCode, err, raw)
	}
	return raw
}

func (x *exchange) json(key opKey, realPath string, body any, want int, hdr ...string) []byte {
	x.t.Helper()
	var raw []byte
	if body != nil {
		var err error
		if raw, err = json.Marshal(body); err != nil {
			x.t.Fatal(err)
		}
	}
	return x.do(key, realPath, "application/json", raw, want, hdr...)
}

// specExamples returns the request-body examples the spec gives for key, so the
// examples shown in the documentation are exactly what gets sent to the handlers.
func (x *exchange) specExamples(key opKey) []any {
	media := x.ops[key].RequestBody.Value.Content["application/json"]
	if media.Example != nil {
		return []any{media.Example}
	}
	var out []any
	var names []string
	for n := range media.Examples {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		out = append(out, media.Examples[n].Value.Value)
	}
	return out
}

func get(p string) opKey  { return opKey{"GET", p} }
func post(p string) opKey { return opKey{"POST", p} }
func put(p string) opKey  { return opKey{"PUT", p} }
func del(p string) opKey  { return opKey{"DELETE", p} }

func TestOpenAPI_SuccessResponsesMatchTheSchemas(t *testing.T) {
	e := uploadEnv(t, Options{AuthMethod: config.AuthToken, AccessAddress: "http://127.0.0.1:8181"})
	e.bot.snap = fullSnap()
	e.bot.added = Added{ID: 9, Title: "[Radio] Jazz"}
	e.bot.stations = []radio.Station{{UUID: "u1", Name: "Jazz FM", Codec: "MP3", Bitrate: 128, Country: "DE", Tags: "jazz"}}
	e.bot.libPage = LibraryPage{
		Dirs:   []LibraryDir{{Name: "rock", Path: "rock", Tracks: 3}},
		Tracks: []library.Track{{Path: "a.mp3", Title: "A", Artist: "Band", Tags: []string{"x"}, Duration: time.Minute}},
		Total:  1, Tags: []string{"x"},
	}
	x := newExchange(t, e)
	etag := `"7"`

	// state, with and without a conditional request
	raw := x.json(get("/api/v1/state"), "/api/v1/state", nil, 200)
	var st map[string]any
	json.Unmarshal(raw, &st)
	if st["position_ms"] == nil {
		t.Error("the fixture should have a known position so the schema is exercised with a number")
	}
	x.json(get("/api/v1/state"), "/api/v1/state?since=7", nil, 304, "If-None-Match", etag)
	e.bot.snap.PositionKnown = false
	x.json(get("/api/v1/state"), "/api/v1/state", nil, 200) // position_ms: null

	for _, p := range []string{"pause", "resume", "stop", "next"} {
		x.json(post("/api/v1/playback/"+p), "/api/v1/playback/"+p, nil, 200)
	}
	for _, ex := range x.specExamples(post("/api/v1/playback/seek")) {
		x.json(post("/api/v1/playback/seek"), "/api/v1/playback/seek", ex, 200)
	}
	for _, ex := range x.specExamples(put("/api/v1/volume")) {
		x.json(put("/api/v1/volume"), "/api/v1/volume", ex, 200)
	}
	x.json(put("/api/v1/volume"), "/api/v1/volume", map[string]any{"muted": true}, 200)
	for _, ex := range x.specExamples(put("/api/v1/mode")) {
		x.json(put("/api/v1/mode"), "/api/v1/mode", ex, 200)
	}

	for _, ex := range x.specExamples(post("/api/v1/queue")) {
		raw := x.json(post("/api/v1/queue"), "/api/v1/queue", ex, 201)
		var out struct{ Added struct{ ID uint64 } }
		json.Unmarshal(raw, &out)
		if out.Added.ID != 9 {
			t.Errorf("added = %s", raw)
		}
	}
	x.json(del("/api/v1/queue"), "/api/v1/queue", nil, 200)
	x.json(post("/api/v1/queue/shuffle"), "/api/v1/queue/shuffle", nil, 200)
	x.json(del("/api/v1/queue/{id}"), "/api/v1/queue/5", nil, 200)
	x.json(post("/api/v1/queue/{id}/play"), "/api/v1/queue/5/play", nil, 200)
	for _, ex := range x.specExamples(post("/api/v1/queue/{id}/move")) {
		x.json(post("/api/v1/queue/{id}/move"), "/api/v1/queue/5/move", ex, 200)
	}

	e.bot.presets = []RadioPreset{{Name: "jazz", Comment: "Jazz Yeah !"}, {Name: "rock"}}
	x.json(get("/api/v1/radio/presets"), "/api/v1/radio/presets", nil, 200)
	x.json(get("/api/v1/radio/search"), "/api/v1/radio/search?q=jazz&limit=5", nil, 200)
	x.json(get("/api/v1/library"), "/api/v1/library?dir=rock&q=a&tag=x&tag=y,z&limit=10&offset=0", nil, 200)
	x.json(get("/api/v1/library"), "/api/v1/library", nil, 200)
	e.bot.snap.Scan.Running = true
	x.json(post("/api/v1/library/rescan"), "/api/v1/library/rescan", nil, 202)

	body, ct := multipartBody(t, [3]string{"file", "song.mp3", "AUDIO"})
	x.do(post("/api/v1/library/upload"), "/api/v1/library/upload", ct, []byte(body), 201)

	if raw := x.do(get("/api/v1/openapi.yaml"), "/api/v1/openapi.yaml", "", nil, 200); !bytes.Equal(raw, specYAML) {
		t.Error("GET /api/v1/openapi.yaml does not return the embedded document")
	}

	// logout and login need no session; login redeems a real token.
	x.do(post("/api/v1/auth/logout"), "/api/v1/auth/logout", "", nil, 204)
	tok, _ := e.auth.IssueToken(alice)
	x.do(get("/login"), "/login?token="+tok, "", nil, 303)
	x.do(get("/login"), "/login?token=wrong", "", nil, 401)

	// Nothing may be left undescribed by a test: a new operation needs a case here.
	for k := range x.ops {
		if !x.tested[k] {
			t.Errorf("%s has no success case in TestOpenAPI_SuccessResponsesMatchTheSchemas", k)
		}
	}
}

// The errors the spec names are really what the handlers answer, in the documented shape.
func TestOpenAPI_ErrorResponsesMatchTheSchemas(t *testing.T) {
	new := func(mutate func(*fakeBot)) *exchange {
		e := uploadEnv(t, Options{AuthMethod: config.AuthToken, MaxUploadBytes: 1000})
		e.bot.snap = fullSnap()
		if mutate != nil {
			mutate(e.bot)
		}
		return newExchange(t, e)
	}
	code := func(raw []byte) string {
		var b errorBody
		json.Unmarshal(raw, &b)
		return b.Code
	}
	check := func(raw []byte, want string) {
		t.Helper()
		if got := code(raw); got != want {
			t.Errorf("code = %q, want %q\n%s", got, want, raw)
		}
	}
	big, bigCT := multipartBody(t, [3]string{"file", "a.mp3", strings.Repeat("x", 1000+multipartOverhead+10)})
	ok, okCT := multipartBody(t, [3]string{"file", "a.mp3", "x"})
	noFile, noFileCT := multipartBody(t, [3]string{"note", "", "text"})

	// seek
	x := new(nil)
	check(x.json(post("/api/v1/playback/seek"), "/api/v1/playback/seek", map[string]any{}, 400), "bad_request")
	x = new(func(b *fakeBot) { b.seekErr = ErrSeekRange })
	check(x.json(post("/api/v1/playback/seek"), "/api/v1/playback/seek", map[string]any{"position_ms": 1}, 400), "seek_range")
	x = new(func(b *fakeBot) { b.seekErr = ErrNothingPlaying })
	check(x.json(post("/api/v1/playback/seek"), "/api/v1/playback/seek", map[string]any{"position_ms": 1}, 409), "nothing_playing")
	x = new(func(b *fakeBot) { b.seekErr = ErrNotSeekable })
	check(x.json(post("/api/v1/playback/seek"), "/api/v1/playback/seek", map[string]any{"position_ms": 1}, 409), "not_seekable")
	// volume and mode
	x = new(nil)
	check(x.json(put("/api/v1/volume"), "/api/v1/volume", map[string]any{}, 400), "bad_request")
	check(x.json(put("/api/v1/mode"), "/api/v1/mode", map[string]any{"mode": "nope"}, 400), "invalid_mode")
	x = new(func(b *fakeBot) { b.modeErr = library.ErrUnavailable })
	check(x.json(put("/api/v1/mode"), "/api/v1/mode", map[string]any{"mode": "autoplay"}, 409), "library_unavailable")
	// queue add
	x = new(nil)
	check(x.json(post("/api/v1/queue"), "/api/v1/queue", map[string]any{"type": "url", "url": "https://x/y", "position": "first"}, 400), "bad_request")
	for _, c := range []struct {
		err    error
		status int
		code   string
	}{
		{library.ErrNotInLibrary, 404, "not_in_library"},
		{library.ErrUnavailable, 409, "library_unavailable"},
		{fmt.Errorf("%w: unknown radio preset", ErrBadSource), 422, "invalid_source"},
		{ErrBusy, 503, "busy"},
		{context.DeadlineExceeded, 504, "timeout"},
	} {
		x = new(func(b *fakeBot) { b.addErr = c.err })
		check(x.json(post("/api/v1/queue"), "/api/v1/queue", map[string]any{"type": "library", "path": "a.mp3"}, c.status), c.code)
	}
	// entries
	x = new(func(b *fakeBot) {
		b.removeErr, b.playErr, b.moveErr = queue.ErrNotFound, queue.ErrNotFound, queue.ErrNotFound
	})
	check(x.json(del("/api/v1/queue/{id}"), "/api/v1/queue/5", nil, 404), "not_found")
	check(x.json(post("/api/v1/queue/{id}/play"), "/api/v1/queue/5/play", nil, 404), "not_found")
	check(x.json(post("/api/v1/queue/{id}/move"), "/api/v1/queue/5/move", map[string]any{"after": 1}, 404), "not_found")
	check(x.json(post("/api/v1/queue/{id}/move"), "/api/v1/queue/5/move", map[string]any{}, 400), "bad_request")
	// radio, library, rescan
	x = new(func(b *fakeBot) { b.searchErr = context.DeadlineExceeded })
	check(x.json(get("/api/v1/radio/search"), "/api/v1/radio/search?q=x", nil, 504), "timeout")
	x = new(func(b *fakeBot) { b.libErr = library.ErrUnavailable })
	check(x.json(get("/api/v1/library"), "/api/v1/library", nil, 409), "library_unavailable")
	x = new(nil)
	check(x.json(get("/api/v1/library"), "/api/v1/library?limit=0", nil, 400), "bad_request")
	x = new(func(b *fakeBot) { b.rescanErr = library.ErrScanRunning })
	check(x.json(post("/api/v1/library/rescan"), "/api/v1/library/rescan", nil, 409), "scan_running")
	// upload
	x = new(nil)
	check(x.do(post("/api/v1/library/upload"), "/api/v1/library/upload", "application/json", []byte(`{}`), 400), "bad_request")
	check(x.do(post("/api/v1/library/upload"), "/api/v1/library/upload", noFileCT, []byte(noFile), 400), "bad_request")
	check(x.do(post("/api/v1/library/upload"), "/api/v1/library/upload", bigCT, []byte(big), 413), "too_large")
	x.e.srv.opts.UploadEnabled = false
	check(x.do(post("/api/v1/library/upload"), "/api/v1/library/upload", okCT, []byte(ok), 403), "upload_disabled")
	for _, c := range []struct {
		err    error
		status int
		code   string
	}{
		{library.ErrUnavailable, 409, "library_unavailable"},
		{fmt.Errorf("%w: only audio files are accepted", library.ErrNotIndexable), 422, "invalid_file"},
	} {
		x = new(func(b *fakeBot) { b.uploadErr = c.err })
		check(x.do(post("/api/v1/library/upload"), "/api/v1/library/upload", okCT, []byte(ok), c.status), c.code)
	}
	x = new(nil)
	for i := 0; i < maxConcurrentUploads; i++ {
		x.e.srv.uploadSlots <- struct{}{}
	}
	check(x.do(post("/api/v1/library/upload"), "/api/v1/library/upload", okCT, []byte(ok), 503), "busy")
}

// Without a session every protected operation answers 401 in the documented shape.
func TestOpenAPI_UnauthenticatedIsDocumentedEverywhere(t *testing.T) {
	e := newEnv(t, Options{AuthMethod: config.AuthToken, UploadEnabled: true}) // not logged in
	x := newExchange(t, e)
	body, ct := multipartBody(t, [3]string{"file", "a.mp3", "x"})
	n := 0
	for k, op := range x.ops {
		if op.Security != nil && len(*op.Security) == 0 {
			continue // login and logout need no session
		}
		path := strings.ReplaceAll(k.path, "{id}", "5")
		switch {
		case k.path == "/api/v1/radio/search":
			path += "?q=x"
		case k.path == "/api/v1/library/upload":
			raw := x.do(k, path, ct, []byte(body), 401)
			checkCode(t, raw, "unauthenticated")
			n++
			continue
		}
		var payload any
		switch k.path {
		case "/api/v1/playback/seek":
			payload = map[string]any{"position_ms": 1}
		case "/api/v1/volume":
			payload = map[string]any{"volume": 0.5}
		case "/api/v1/mode":
			payload = map[string]any{"mode": "repeat"}
		case "/api/v1/queue":
			if k.method == "POST" {
				payload = map[string]any{"type": "radio", "preset": "jazz"}
			}
		case "/api/v1/queue/{id}/move":
			payload = map[string]any{"after": 0}
		}
		raw := x.json(k, path, payload, 401)
		checkCode(t, raw, "unauthenticated")
		n++
	}
	if n < 15 {
		t.Errorf("only %d protected operations were checked", n)
	}
}

func checkCode(t *testing.T, raw []byte, want string) {
	t.Helper()
	var b errorBody
	if err := json.Unmarshal(raw, &b); err != nil || b.Code != want {
		t.Errorf("body = %s, want code %q", raw, want)
	}
}

// ---- the documentation page ----

func TestDocs_SpecIsServedBehindTheLogin(t *testing.T) {
	e := newEnv(t, Options{AuthMethod: config.AuthToken})
	if resp := e.get(t, "/api/v1/openapi.yaml"); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("without a session = %d, want 401", resp.StatusCode)
	}
	e.login(t, alice)
	resp := e.get(t, "/api/v1/openapi.yaml")
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || !bytes.Equal(raw, specYAML) {
		t.Errorf("= %d, %d bytes (want the embedded spec, %d bytes)", resp.StatusCode, len(raw), len(specYAML))
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/yaml") {
		t.Errorf("Content-Type = %q", ct)
	}
}

func TestDocs_PageNeedsALoginAndHasItsOwnCSP(t *testing.T) {
	e := newEnv(t, Options{AuthMethod: config.AuthToken})
	resp := e.get(t, "/api/docs")
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusUnauthorized || !strings.Contains(string(raw), "Log in first") {
		t.Errorf("without a session = %d %.80s", resp.StatusCode, raw)
	}
	if strings.Contains(resp.Header.Get("Content-Security-Policy"), "script-src") {
		t.Error("the login hint page must keep the strict default policy")
	}

	e.login(t, alice)
	resp = e.get(t, "/api/docs")
	page, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") {
		t.Fatalf("= %d %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	csp := resp.Header.Get("Content-Security-Policy")
	for _, must := range []string{"default-src 'none'", "script-src 'self'", "connect-src 'self'", "frame-ancestors 'none'"} {
		if !strings.Contains(csp, must) {
			t.Errorf("CSP lacks %q: %s", must, csp)
		}
	}
	for _, never := range []string{"unsafe-eval", "script-src 'unsafe-inline'", "*", "http:", "https:"} {
		if strings.Contains(csp, never) {
			t.Errorf("CSP must not contain %q: %s", never, csp)
		}
	}
	if !strings.Contains(string(page), "/api/docs/assets/swagger-ui-bundle.js") || !strings.Contains(string(page), "/api/docs/assets/init.js") {
		t.Errorf("the page does not load the scripts: %s", page)
	}
	// An inline script would be blocked by the policy, and the page would stay blank.
	if regexp.MustCompile(`(?i)<script(\s[^>]*)?>`).FindAllString(string(page), -1) == nil {
		t.Fatal("no script tags found")
	}
	for _, tag := range regexp.MustCompile(`(?i)<script[^>]*>`).FindAllString(string(page), -1) {
		if !strings.Contains(tag, "src=") {
			t.Errorf("inline script %q would be blocked by the CSP", tag)
		}
	}
	if regexp.MustCompile(`(?i)\son\w+\s*=`).MatchString(string(page)) {
		t.Error("an inline event handler would be blocked by the CSP")
	}
	if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "private") {
		t.Errorf("Cache-Control = %q", cc)
	}
}

func TestDocs_NoLoginMeansNoLoginPage(t *testing.T) {
	e := newEnv(t, Options{AuthMethod: config.AuthNone, Addr: "127.0.0.1:8181"})
	resp := e.get(t, "/api/docs", "Host", "127.0.0.1:8181")
	if resp.StatusCode != 200 {
		t.Errorf("= %d, want 200", resp.StatusCode)
	}
}

func TestDocs_AssetsAreAnAllowlist(t *testing.T) {
	e := newEnv(t, Options{AuthMethod: config.AuthToken}) // not logged in: assets are public files
	for name, ct := range map[string]string{
		"swagger-ui-bundle.js": "javascript",
		"swagger-ui.css":       "css",
		"init.js":              "javascript",
	} {
		resp := e.get(t, "/api/docs/assets/"+name)
		raw, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != 200 || !strings.Contains(resp.Header.Get("Content-Type"), ct) || len(raw) == 0 {
			t.Errorf("%s = %d %q (%d bytes)", name, resp.StatusCode, resp.Header.Get("Content-Type"), len(raw))
		}
	}
	// Nothing else in the embedded tree, and no directory listing.
	for _, p := range []string{
		"index.html", "README.md", "LICENSE", "NOTICE", "openapi.yaml", "../openapi.yaml",
		"..%2fopenapi.yaml", "%2e%2e/index.html", "swagger-ui-bundle.js.map", "", "nope.js",
	} {
		resp := e.get(t, "/api/docs/assets/"+p)
		// The mux answers a path with ".." by redirecting to the cleaned path. That is
		// fine as long as the destination serves nothing either.
		if loc := resp.Header.Get("Location"); resp.StatusCode/100 == 3 && loc != "" {
			next := e.get(t, strings.TrimPrefix(loc, e.ts.URL))
			resp = next
		}
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("/api/docs/assets/%s = %d, want 404 (never a file)", p, resp.StatusCode)
		}
	}
}

// The vendored Swagger UI must not change unnoticed. To update it, follow
// swaggerui/README.md and replace these hashes.
func TestDocs_VendoredFilesAreUnmodified(t *testing.T) {
	for file, want := range map[string]string{
		"swaggerui/swagger-ui-bundle.js": "36eeb4d5640b9c036b1d0f854499af1cceebbdb260b790a03a18be5e8488fbf5",
		"swaggerui/swagger-ui.css":       "c3cfe8248c4b0b89673600fb829924eafbac6de4bc4b2b06f5f5243483074d0a",
	} {
		raw, err := docsFS.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(raw)
		if got := hex.EncodeToString(sum[:]); got != want {
			t.Errorf("%s sha256 = %s, want %s: was the vendored file edited or replaced?", file, got, want)
		}
	}
	if _, err := os.Stat("swaggerui/LICENSE"); err != nil {
		t.Error("the Swagger UI licence file must stay next to the vendored code")
	}
}
