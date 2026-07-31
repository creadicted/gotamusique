package hls

import "testing"

func TestNewHLSItemFromURL_filename(t *testing.T) {
	item := NewHLSItemFromURL("https://cdn.example.com/live/jazz-128k.m3u8")
	if item.Name != "jazz-128k" {
		t.Errorf("Name = %q, want %q", item.Name, "jazz-128k")
	}
	if item.URL != "https://cdn.example.com/live/jazz-128k.m3u8" {
		t.Errorf("URL = %q", item.URL)
	}
}

func TestNewHLSItemFromURL_uppercaseExtension(t *testing.T) {
	item := NewHLSItemFromURL("https://cdn.example.com/live/stream.M3U8")
	if item.Name != "stream" {
		t.Errorf("Name = %q, want %q", item.Name, "stream")
	}
}

func TestNewHLSItemFromURL_trailingSlash(t *testing.T) {
	// path.Base strips the trailing slash and returns the last segment ("live"),
	// which is a valid name — hostname fallback is not triggered here.
	item := NewHLSItemFromURL("https://cdn.example.com/live/")
	if item.Name != "live" {
		t.Errorf("Name = %q, want %q", item.Name, "live")
	}
}

func TestNewHLSItemFromURL_rootPath(t *testing.T) {
	// path.Base("/") returns "/", which triggers the hostname fallback.
	item := NewHLSItemFromURL("https://cdn.example.com/")
	if item.Name != "cdn.example.com" {
		t.Errorf("Name = %q, want hostname fallback", item.Name)
	}
}

func TestNewHLSItemFromURL_noPath(t *testing.T) {
	item := NewHLSItemFromURL("https://cdn.example.com")
	if item.Name != "cdn.example.com" {
		t.Errorf("Name = %q, want hostname", item.Name)
	}
}

func TestNewHLSItemFromURL_malformed(t *testing.T) {
	item := NewHLSItemFromURL("not-a-url")
	if item.Name != "not-a-url" {
		t.Errorf("Name = %q, want raw URL as fallback", item.Name)
	}
}

func TestNewHLSItemFromURL_pathWithoutExtension(t *testing.T) {
	item := NewHLSItemFromURL("https://cdn.example.com/live/stream")
	if item.Name != "stream" {
		t.Errorf("Name = %q, want %q", item.Name, "stream")
	}
}

func TestHLSItem_FormatTitle(t *testing.T) {
	item := &HLSItem{Name: "jazz-128k"}
	if item.FormatTitle() != "[HLS] jazz-128k" {
		t.Errorf("FormatTitle = %q", item.FormatTitle())
	}
}

func TestHLSItem_FormatTitle_empty(t *testing.T) {
	item := &HLSItem{Name: ""}
	if item.FormatTitle() != "[HLS] " {
		t.Errorf("FormatTitle = %q", item.FormatTitle())
	}
}

func TestHLSItem_StreamURL(t *testing.T) {
	item := &HLSItem{URL: "https://cdn.example.com/live/jazz-128k.m3u8", Name: "jazz-128k"}
	if item.StreamURL() != "https://cdn.example.com/live/jazz-128k.m3u8" {
		t.Errorf("StreamURL = %q", item.StreamURL())
	}
}
