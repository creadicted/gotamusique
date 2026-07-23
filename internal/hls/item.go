package hls

import (
	"net/url"
	"path"
	"strings"
)

// HLSItem represents an HLS/M3U8 playlist stream.
type HLSItem struct {
	URL  string
	Name string
}

// StreamURL implements audio.MediaItem.
func (h *HLSItem) StreamURL() string { return h.URL }

// FormatTitle implements audio.MediaItem.
func (h *HLSItem) FormatTitle() string { return "[HLS] " + h.Name }

// NewHLSItemFromURL constructs an HLSItem from a raw URL.
// Name is derived from the last path segment with the .m3u8 extension stripped,
// falling back to the hostname, then the raw URL string for malformed input.
func NewHLSItemFromURL(rawURL string) *HLSItem {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return &HLSItem{URL: rawURL, Name: rawURL}
	}

	base := path.Base(u.Path)
	name := base
	if strings.HasSuffix(strings.ToLower(base), ".m3u8") {
		name = base[:len(base)-len(".m3u8")]
	}
	if name == "" || name == "/" || name == "." {
		name = u.Host
	}

	return &HLSItem{URL: rawURL, Name: name}
}
