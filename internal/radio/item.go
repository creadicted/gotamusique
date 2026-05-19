package radio

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
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
// timeout. If the Go HTTP client cannot parse the response (e.g. ICY/SHOUTcast
// servers that respond with "ICY 200 OK"), a raw TCP probe is used as a final
// fallback. Call this explicitly — constructors do not call it.
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
	if err == nil {
		resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
		return fmt.Errorf("validating stream %q: server returned %d", r.URL, resp.StatusCode)
	}

	// ICY/SHOUTcast servers respond with "ICY 200 OK" instead of "HTTP/1.1 200 OK",
	// causing the Go HTTP client to return a parse error or io.EOF. Fall back to a
	// raw TCP probe to distinguish a live ICY stream from a server that connects and
	// immediately closes without sending data.
	if rawTCPReachable(r.URL, validateTimeout) {
		return nil
	}
	return fmt.Errorf("validating stream %q: %w", r.URL, err)
}

// rawTCPReachable sends a minimal GET request over a raw TCP connection and
// returns true if the server sends at least one byte back.
func rawTCPReachable(rawURL string, timeout time.Duration) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := u.Host
	if u.Port() == "" {
		if u.Scheme == "https" {
			host += ":443"
		} else {
			host += ":80"
		}
	}

	conn, err := net.DialTimeout("tcp", host, timeout)
	if err != nil {
		return false
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(timeout)) //nolint:errcheck

	path := u.RequestURI()
	fmt.Fprintf(conn, "GET %s HTTP/1.0\r\nHost: %s\r\nUser-Agent: gotamusique/0.1\r\n\r\n", path, u.Host) //nolint:errcheck

	buf := make([]byte, 1)
	n, _ := conn.Read(buf)
	return n > 0
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
