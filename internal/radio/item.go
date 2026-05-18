package radio

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/konradk/gotamusique/internal/config"
)

const validateTimeout = 5 * time.Second

// RadioItem represents a live HTTP audio stream (radio station).
type RadioItem struct {
	URL  string
	Name string
}

// StreamURL implements audio.MediaItem.
func (r *RadioItem) StreamURL() string { return r.URL }

// FormatTitle implements audio.MediaItem.
func (r *RadioItem) FormatTitle() string { return "[Radio] " + r.Name }

// Validate checks that the stream URL is reachable. It tries HEAD first; if
// that returns non-2xx it falls back to a plain GET. Both requests use a 5s
// timeout. Call this explicitly — constructors do not call it.
func (r *RadioItem) Validate() error {
	client := &http.Client{Timeout: validateTimeout}

	resp, err := client.Head(r.URL)
	if err == nil {
		resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
	}

	resp, err = client.Get(r.URL)
	if err != nil {
		// ICY/SHOUTcast servers respond with "ICY 200 OK" instead of
		// "HTTP/1.1 200 OK". The Go HTTP client cannot parse this and returns
		// either io.EOF or a "malformed HTTP" error. The server did respond,
		// so treat these as reachable.
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
			strings.Contains(err.Error(), "malformed HTTP") {
			return nil
		}
		return fmt.Errorf("validating stream %q: %w", r.URL, err)
	}
	resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return fmt.Errorf("validating stream %q: server returned %d", r.URL, resp.StatusCode)
}

// NewRadioItemFromURL constructs a RadioItem from a raw URL.
// Name defaults to the URL hostname (host:port if non-standard).
func NewRadioItemFromURL(rawURL string) *RadioItem {
	name := rawURL
	if u, err := url.Parse(rawURL); err == nil && u.Host != "" {
		name = u.Host
	}
	return &RadioItem{URL: rawURL, Name: name}
}

// NewRadioItemFromPreset constructs a RadioItem from a config preset.
// preset.Comment becomes Name; the URL hostname is used when Comment is empty.
func NewRadioItemFromPreset(_ string, preset config.RadioPreset) *RadioItem {
	name := preset.Comment
	if name == "" {
		if u, err := url.Parse(preset.URL); err == nil && u.Host != "" {
			name = u.Host
		} else {
			name = preset.URL
		}
	}
	return &RadioItem{URL: preset.URL, Name: name}
}

// NewRadioItemFromStation constructs a RadioItem from a radio-browser Station.
// url_resolved is preferred over url when available; it is already dereferenced
// by radio-browser and avoids ICY/redirect issues that confuse the Go HTTP client.
func NewRadioItemFromStation(s Station) *RadioItem {
	name := s.Name
	if name == "" {
		name = s.URL
	}
	streamURL := s.URLResolved
	if streamURL == "" {
		streamURL = s.URL
	}
	return &RadioItem{URL: streamURL, Name: name}
}
