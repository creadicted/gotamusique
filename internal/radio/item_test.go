package radio

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/konradk/gotamusique/internal/config"
)

// --- constructors ---

func TestNewRadioItemFromURL_hostname(t *testing.T) {
	item := NewRadioItemFromURL("http://stream.example.com/jazz")
	if item.Name != "stream.example.com" {
		t.Errorf("Name = %q, want %q", item.Name, "stream.example.com")
	}
	if item.URL != "http://stream.example.com/jazz" {
		t.Errorf("URL = %q", item.URL)
	}
}

func TestNewRadioItemFromURL_hostnameWithPort(t *testing.T) {
	item := NewRadioItemFromURL("http://stream.example.com:8080/live")
	if item.Name != "stream.example.com:8080" {
		t.Errorf("Name = %q, want host:port", item.Name)
	}
}

func TestNewRadioItemFromURL_malformed(t *testing.T) {
	item := NewRadioItemFromURL("not-a-url")
	if item.Name != "not-a-url" {
		t.Errorf("Name = %q, want raw URL as fallback", item.Name)
	}
}

func TestNewRadioItemFromPreset_withComment(t *testing.T) {
	p := config.RadioPreset{URL: "http://example.com/stream", Comment: "Jazz FM"}
	item := NewRadioItemFromPreset("jazz", p)
	if item.Name != "Jazz FM" {
		t.Errorf("Name = %q, want Comment", item.Name)
	}
	if item.URL != p.URL {
		t.Errorf("URL = %q", item.URL)
	}
}

func TestNewRadioItemFromPreset_noComment(t *testing.T) {
	p := config.RadioPreset{URL: "http://stream.somafm.com/groovesalad-128-mp3"}
	item := NewRadioItemFromPreset("groovesalad", p)
	if item.Name != "stream.somafm.com" {
		t.Errorf("Name = %q, want hostname", item.Name)
	}
}

func TestNewRadioItemFromPreset_aliasIgnored(t *testing.T) {
	p := config.RadioPreset{URL: "http://example.com/stream", Comment: "My Station"}
	item := NewRadioItemFromPreset("alias-does-not-matter", p)
	if item.Name != "My Station" {
		t.Errorf("Name = %q", item.Name)
	}
}

func TestNewRadioItemFromStation(t *testing.T) {
	s := Station{UUID: "abc", Name: "Groove Salad", URL: "http://example.com/stream"}
	item := NewRadioItemFromStation(s)
	if item.Name != "Groove Salad" {
		t.Errorf("Name = %q", item.Name)
	}
	if item.URL != s.URL {
		t.Errorf("URL = %q", item.URL)
	}
}

func TestNewRadioItemFromStation_emptyName(t *testing.T) {
	s := Station{UUID: "abc", Name: "", URL: "http://example.com/stream"}
	item := NewRadioItemFromStation(s)
	if item.Name != s.URL {
		t.Errorf("Name = %q, want URL as fallback", item.Name)
	}
}

// --- methods ---

func TestRadioItem_StreamURL(t *testing.T) {
	item := &RadioItem{URL: "http://example.com/stream", Name: "Test"}
	if item.StreamURL() != "http://example.com/stream" {
		t.Errorf("StreamURL = %q", item.StreamURL())
	}
}

func TestRadioItem_FormatTitle(t *testing.T) {
	item := &RadioItem{Name: "Groove Salad"}
	if item.FormatTitle() != "[Radio] Groove Salad" {
		t.Errorf("FormatTitle = %q", item.FormatTitle())
	}
}

func TestRadioItem_FormatTitle_empty(t *testing.T) {
	item := &RadioItem{Name: ""}
	if item.FormatTitle() != "[Radio] " {
		t.Errorf("FormatTitle = %q", item.FormatTitle())
	}
}

// --- Validate ---

func TestValidate_headSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	item := NewRadioItemFromURL(ts.URL)
	if err := item.Validate(); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestValidate_headFails_getFallbackSucceeds(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	item := NewRadioItemFromURL(ts.URL)
	if err := item.Validate(); err != nil {
		t.Errorf("expected nil after GET fallback, got %v", err)
	}
}

func TestValidate_headNon2xx_getFallbackSucceeds(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	item := NewRadioItemFromURL(ts.URL)
	if err := item.Validate(); err != nil {
		t.Errorf("expected nil after GET fallback, got %v", err)
	}
}

func TestValidate_bothFail(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ts.Close()

	item := NewRadioItemFromURL(ts.URL)
	err := item.Validate()
	if err == nil {
		t.Fatal("expected error when both HEAD and GET return non-2xx")
	}
	if !strings.Contains(err.Error(), ts.URL) {
		t.Errorf("error should mention the URL; got %q", err)
	}
}

func TestValidate_unreachable(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	ts.Close() // shut down before the test runs

	item := NewRadioItemFromURL(ts.URL)
	if err := item.Validate(); err == nil {
		t.Fatal("expected error for unreachable server")
	}
}

func TestValidate_headSuccess_noGetCalled(t *testing.T) {
	getCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			getCount++
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	item := NewRadioItemFromURL(ts.URL)
	if err := item.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if getCount > 0 {
		t.Errorf("GET should not be called when HEAD succeeds; called %d time(s)", getCount)
	}
}

func TestValidate_emptyResponse(t *testing.T) {
	// Server accepts the TCP connection but closes it without sending any data.
	// This produces io.EOF in the HTTP client and must NOT be treated as reachable.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	item := NewRadioItemFromURL("http://" + ln.Addr().String() + "/stream")
	if err := item.Validate(); err == nil {
		t.Error("expected error for server that closes connection without data, got nil")
	}
}

func TestValidate_icyProtocol(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 512)
				c.Read(buf) //nolint:errcheck
				c.Write([]byte("ICY 200 OK\r\nContent-Type: audio/mpeg\r\n\r\n")) //nolint:errcheck
			}(conn)
		}
	}()

	item := NewRadioItemFromURL("http://" + ln.Addr().String() + "/stream")
	if err := item.Validate(); err != nil {
		t.Errorf("ICY stream should be treated as reachable, got: %v", err)
	}
}

func TestValidate_head201(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	defer ts.Close()

	item := NewRadioItemFromURL(ts.URL)
	if err := item.Validate(); err != nil {
		t.Errorf("201 should be accepted as 2xx; got %v", err)
	}
}
