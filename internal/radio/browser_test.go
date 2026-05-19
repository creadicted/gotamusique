package radio

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// --- Search ---

func TestSearch_success(t *testing.T) {
	want := []Station{
		{UUID: "abc123", Name: "Jazz FM", URL: "http://example.com/jazz", Codec: "MP3", Bitrate: 128},
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "byname") {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(want) //nolint:errcheck
	}))
	defer ts.Close()

	rb := newRadioBrowserWithClient(&http.Client{}, ts.URL)
	got, err := rb.Search("jazz", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 1 || got[0].UUID != "abc123" || got[0].Name != "Jazz FM" {
		t.Errorf("got %+v", got)
	}
}

func TestSearch_sendsUserAgent(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != browserUserAgent {
			t.Errorf("User-Agent = %q, want %q", r.Header.Get("User-Agent"), browserUserAgent)
		}
		json.NewEncoder(w).Encode([]Station{}) //nolint:errcheck
	}))
	defer ts.Close()

	rb := newRadioBrowserWithClient(&http.Client{}, ts.URL)
	rb.Search("jazz", 10) //nolint:errcheck
}

func TestSearch_encodesSpacesInName(t *testing.T) {
	var capturedURI string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURI = r.URL.RequestURI()
		json.NewEncoder(w).Encode([]Station{}) //nolint:errcheck
	}))
	defer ts.Close()

	rb := newRadioBrowserWithClient(&http.Client{}, ts.URL)
	rb.Search("jazz blues", 10) //nolint:errcheck
	if !strings.Contains(capturedURI, "jazz%20blues") {
		t.Errorf("expected URL-encoded name in request URI, got %q", capturedURI)
	}
}

func TestSearch_limitsResults(t *testing.T) {
	var capturedURI string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURI = r.URL.RequestURI()
		json.NewEncoder(w).Encode([]Station{}) //nolint:errcheck
	}))
	defer ts.Close()

	rb := newRadioBrowserWithClient(&http.Client{}, ts.URL)
	rb.Search("jazz", 10) //nolint:errcheck
	if !strings.Contains(capturedURI, "limit=10") {
		t.Errorf("expected limit=10 in request URI, got %q", capturedURI)
	}
}

func TestSearch_customLimit(t *testing.T) {
	var capturedURI string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURI = r.URL.RequestURI()
		json.NewEncoder(w).Encode([]Station{}) //nolint:errcheck
	}))
	defer ts.Close()

	rb := newRadioBrowserWithClient(&http.Client{}, ts.URL)
	rb.Search("jazz", 20) //nolint:errcheck
	if !strings.Contains(capturedURI, "limit=20") {
		t.Errorf("expected limit=20 in request URI, got %q", capturedURI)
	}
}

func TestSearch_emptyResults(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]Station{}) //nolint:errcheck
	}))
	defer ts.Close()

	rb := newRadioBrowserWithClient(&http.Client{}, ts.URL)
	got, err := rb.Search("xyznonexistent", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %d stations", len(got))
	}
}

func TestSearch_apiNon2xx(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ts.Close()

	rb := newRadioBrowserWithClient(&http.Client{}, ts.URL)
	_, err := rb.Search("jazz", 10)
	if err == nil {
		t.Fatal("expected error on non-2xx response")
	}
}

func TestSearch_malformedJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not-json")) //nolint:errcheck
	}))
	defer ts.Close()

	rb := newRadioBrowserWithClient(&http.Client{}, ts.URL)
	_, err := rb.Search("jazz", 10)
	if err == nil {
		t.Fatal("expected error on malformed JSON")
	}
}

// --- ByUUID ---

func TestByUUID_found(t *testing.T) {
	want := Station{UUID: "abc123", Name: "Groove Salad", URL: "http://example.com/stream"}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "byuuid") {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode([]Station{want}) //nolint:errcheck
	}))
	defer ts.Close()

	rb := newRadioBrowserWithClient(&http.Client{}, ts.URL)
	got, err := rb.ByUUID("abc123")
	if err != nil {
		t.Fatalf("ByUUID: %v", err)
	}
	if got.UUID != "abc123" || got.Name != "Groove Salad" {
		t.Errorf("got %+v", got)
	}
}

func TestByUUID_notFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]Station{}) //nolint:errcheck
	}))
	defer ts.Close()

	rb := newRadioBrowserWithClient(&http.Client{}, ts.URL)
	_, err := rb.ByUUID("nonexistent-uuid")
	if err == nil {
		t.Fatal("expected error for missing station")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should say 'not found'; got %q", err)
	}
}

func TestByUUID_notFoundMentionsUUID(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]Station{}) //nolint:errcheck
	}))
	defer ts.Close()

	rb := newRadioBrowserWithClient(&http.Client{}, ts.URL)
	_, err := rb.ByUUID("deadbeef-uuid")
	if err == nil || !strings.Contains(err.Error(), "deadbeef-uuid") {
		t.Errorf("error should include the UUID; got %v", err)
	}
}

func TestByUUID_apiNon2xx(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	rb := newRadioBrowserWithClient(&http.Client{}, ts.URL)
	_, err := rb.ByUUID("abc123")
	if err == nil {
		t.Fatal("expected error on API failure")
	}
}

func TestByUUID_returnsFirst(t *testing.T) {
	first := Station{UUID: "first", Name: "First Station", URL: "http://example.com/1"}
	second := Station{UUID: "second", Name: "Second Station", URL: "http://example.com/2"}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]Station{first, second}) //nolint:errcheck
	}))
	defer ts.Close()

	rb := newRadioBrowserWithClient(&http.Client{}, ts.URL)
	got, err := rb.ByUUID("first")
	if err != nil {
		t.Fatalf("ByUUID: %v", err)
	}
	if got.UUID != "first" {
		t.Errorf("expected first station; got UUID %q", got.UUID)
	}
}

// --- mirror fallback ---

func TestMirrorFallback_secondMirrorSucceeds(t *testing.T) {
	ts1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ts1.Close()

	ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]Station{{UUID: "abc", Name: "Test", URL: "http://example.com"}}) //nolint:errcheck
	}))
	defer ts2.Close()

	rb := &RadioBrowser{
		client:  &http.Client{},
		baseURL: ts1.URL,
		mirrors: []string{ts2.URL},
	}
	got, err := rb.Search("test", 10)
	if err != nil {
		t.Fatalf("expected fallback to succeed; got %v", err)
	}
	if len(got) == 0 {
		t.Error("expected results from mirror")
	}
}

func TestMirrorFallback_allFail(t *testing.T) {
	ts1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ts1.Close()

	ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer ts2.Close()

	rb := &RadioBrowser{
		client:  &http.Client{},
		baseURL: ts1.URL,
		mirrors: []string{ts2.URL},
	}
	_, err := rb.Search("test", 10)
	if err == nil {
		t.Fatal("expected error when all mirrors fail")
	}
}

func TestMirrorFallback_noMirrors(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ts.Close()

	rb := newRadioBrowserWithClient(&http.Client{}, ts.URL)
	_, err := rb.Search("test", 10)
	if err == nil {
		t.Fatal("expected error when single target fails and no mirrors")
	}
}

// --- NewRadioBrowser defaults ---

func TestNewRadioBrowser_defaults(t *testing.T) {
	rb := NewRadioBrowser()
	if rb.baseURL == "" {
		t.Error("baseURL must not be empty")
	}
	if rb.client == nil {
		t.Error("client must not be nil")
	}
	if rb.client.Timeout != browserTimeout {
		t.Errorf("Timeout = %v, want %v", rb.client.Timeout, browserTimeout)
	}
	if len(rb.mirrors) == 0 {
		t.Error("must have at least one fallback mirror")
	}
}

func TestNewRadioBrowser_baseURLIsFirstMirror(t *testing.T) {
	rb := NewRadioBrowser()
	if rb.baseURL != browserMirrors[0] {
		t.Errorf("baseURL = %q, want %q", rb.baseURL, browserMirrors[0])
	}
}

// --- integration tests (skipped with -short) ---

func TestSearch_integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	rb := NewRadioBrowser()
	stations, err := rb.Search("jazz", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(stations) == 0 {
		t.Error("expected non-empty results from radio-browser.info")
	}
}

func TestByUUID_integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	rb := NewRadioBrowser()
	stations, err := rb.Search("jazz", 10)
	if err != nil || len(stations) == 0 {
		t.Skip("could not fetch stations to derive a UUID")
	}
	uuid := stations[0].UUID
	got, err := rb.ByUUID(uuid)
	if err != nil {
		t.Fatalf("ByUUID(%q): %v", uuid, err)
	}
	if got.UUID != uuid {
		t.Errorf("UUID mismatch: got %q, want %q", got.UUID, uuid)
	}
}
