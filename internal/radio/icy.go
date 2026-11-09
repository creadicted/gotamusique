package radio

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	icyDialTimeout = 10 * time.Second
	icyReadTimeout = 60 * time.Second
	icyMaxRedirect = 5
	icyMaxTitleLen = 200
)

// WatchTracks implements audio.TrackWatcher: it calls fn with the stream title
// each time the station announces a different one, until ctx is cancelled or
// the stream ends. Stations without ICY metadata simply never call fn.
func (r *RadioItem) WatchTracks(ctx context.Context, fn func(title string)) {
	WatchICY(ctx, r.URL, fn) //nolint:errcheck // best effort; a station without metadata is normal
}

// WatchICY opens its own connection to rawURL asking for in-band ICY metadata
// and calls fn whenever StreamTitle changes. It returns when ctx is cancelled,
// the stream ends, or the station offers no metadata (an error is returned for
// the last case so callers can log it).
func WatchICY(ctx context.Context, rawURL string, fn func(title string)) error {
	conn, br, metaint, err := openICY(ctx, rawURL)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Closing the connection unblocks any read when ctx is cancelled.
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()

	last := ""
	for {
		conn.SetReadDeadline(time.Now().Add(icyReadTimeout)) //nolint:errcheck
		if _, err := io.CopyN(io.Discard, br, int64(metaint)); err != nil {
			return ctxErr(ctx, err)
		}
		n, err := br.ReadByte()
		if err != nil {
			return ctxErr(ctx, err)
		}
		if n == 0 {
			continue // no metadata this interval
		}
		block := make([]byte, int(n)*16)
		if _, err := io.ReadFull(br, block); err != nil {
			return ctxErr(ctx, err)
		}
		title := ParseStreamTitle(string(block))
		if title != "" && title != last {
			last = title
			fn(title)
		}
	}
}

func ctxErr(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

// ParseStreamTitle extracts StreamTitle from an ICY metadata block such as
// "StreamTitle='Artist - Song';StreamUrl=”;". Control characters are removed
// and the result is length-limited. It returns "" when there is no title.
func ParseStreamTitle(block string) string {
	const key = "StreamTitle='"
	i := strings.Index(block, key)
	if i < 0 {
		return ""
	}
	rest := block[i+len(key):]
	// The value ends at the first "';" (titles may themselves contain quotes).
	if j := strings.Index(rest, "';"); j >= 0 {
		rest = rest[:j]
	} else {
		rest = strings.TrimRight(rest, "\x00")
		rest = strings.TrimSuffix(rest, "'")
	}
	rest = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, rest)
	rest = strings.TrimSpace(rest)
	if r := []rune(rest); len(r) > icyMaxTitleLen {
		rest = string(r[:icyMaxTitleLen])
	}
	return rest
}

// openICY connects (following redirects) and returns the connection positioned
// at the start of the audio body, plus the metadata interval in bytes.
// It speaks HTTP/1.0 over a raw socket because SHOUTcast servers answer with
// "ICY 200 OK", which net/http rejects.
func openICY(ctx context.Context, rawURL string) (net.Conn, *bufio.Reader, int, error) {
	for hop := 0; hop <= icyMaxRedirect; hop++ {
		u, err := url.Parse(rawURL)
		if err != nil {
			return nil, nil, 0, err
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return nil, nil, 0, fmt.Errorf("icy: unsupported scheme %q", u.Scheme)
		}
		port := u.Port()
		if port == "" {
			port = map[string]string{"http": "80", "https": "443"}[u.Scheme]
		}
		addr := net.JoinHostPort(u.Hostname(), port)

		d := net.Dialer{Timeout: icyDialTimeout}
		var conn net.Conn
		if u.Scheme == "https" {
			conn, err = (&tls.Dialer{NetDialer: &d, Config: &tls.Config{ServerName: u.Hostname()}}).DialContext(ctx, "tcp", addr)
		} else {
			conn, err = d.DialContext(ctx, "tcp", addr)
		}
		if err != nil {
			return nil, nil, 0, err
		}

		path := u.RequestURI()
		conn.SetDeadline(time.Now().Add(icyDialTimeout)) //nolint:errcheck
		fmt.Fprintf(conn, "GET %s HTTP/1.0\r\nHost: %s\r\nUser-Agent: %s\r\nIcy-MetaData: 1\r\nAccept: */*\r\nConnection: close\r\n\r\n",
			path, u.Host, browserUserAgent)

		br := bufio.NewReader(conn)
		status, err := br.ReadString('\n')
		if err != nil {
			conn.Close()
			return nil, nil, 0, err
		}
		fields := strings.Fields(status)
		code := 0
		if len(fields) >= 2 {
			code, _ = strconv.Atoi(fields[1])
		}

		headers := map[string]string{}
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				conn.Close()
				return nil, nil, 0, err
			}
			line = strings.TrimRight(line, "\r\n")
			if line == "" {
				break
			}
			if k, v, ok := strings.Cut(line, ":"); ok {
				headers[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
			}
		}

		if code >= 300 && code < 400 && headers["location"] != "" {
			conn.Close()
			next, err := u.Parse(headers["location"])
			if err != nil {
				return nil, nil, 0, err
			}
			rawURL = next.String()
			continue
		}
		if code < 200 || code >= 300 {
			conn.Close()
			return nil, nil, 0, fmt.Errorf("icy: server returned %q", strings.TrimSpace(status))
		}
		metaint, err := strconv.Atoi(headers["icy-metaint"])
		if err != nil || metaint <= 0 {
			conn.Close()
			return nil, nil, 0, errors.New("icy: station sends no metadata")
		}
		return conn, br, metaint, nil
	}
	return nil, nil, 0, errors.New("icy: too many redirects")
}
