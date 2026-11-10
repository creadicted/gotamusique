//go:build e2e

// Package e2e runs the real bot against a real Mumble server (the official
// mumblevoip/mumble-server image, started with Docker) and drives it with a
// second Mumble client that plays the part of a human listener.
//
// Run with: make e2e   (needs Docker; tests that decode audio also need ffmpeg)
package e2e

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/konradk/gotamusique/internal/bot"
	"github.com/konradk/gotamusique/internal/config"
	"layeh.com/gumble/gumble"
	"layeh.com/gumble/gumbleutil"
	_ "layeh.com/gumble/opus"
)

const mumbleImage = "mumblevoip/mumble-server:latest"

// startMumble runs a throwaway Mumble server and returns its host:port. The
// container is removed when the test ends.
func startMumble(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}
	out, err := exec.Command("docker", "run", "-d", "--rm",
		"-p", "127.0.0.1:0:64738/tcp", mumbleImage).CombinedOutput()
	if err != nil {
		t.Skipf("cannot start %s: %v\n%s", mumbleImage, err, out)
	}
	id := strings.TrimSpace(string(out))
	t.Cleanup(func() { exec.Command("docker", "rm", "-f", id).Run() }) //nolint:errcheck

	// The mapping is not published the instant `docker run -d` returns (slow on a
	// cold CI runner), so poll instead of asking once.
	var addr string
	eventually(t, 30*time.Second, "docker to publish the server port", func() bool {
		out, err := exec.Command("docker", "port", id, "64738/tcp").Output()
		if err != nil {
			return false
		}
		// "127.0.0.1:49153" (possibly several lines)
		addr = strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
		return addr != ""
	})

	eventually(t, 30*time.Second, "mumble server to accept connections", func() bool {
		c, err := dialHuman(addr, "probe", nil)
		if err != nil {
			return false
		}
		c.Disconnect() //nolint:errcheck
		return true
	})
	return addr
}

// startMumbleRestartable is startMumble on a fixed local port, plus a function
// that restarts the server (same port), to exercise the bot's reconnect.
func startMumbleRestartable(t *testing.T) (addr string, restart func()) {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	addr = fmt.Sprintf("127.0.0.1:%d", port)

	out, err := exec.Command("docker", "run", "-d", "--rm", "--stop-timeout", "2",
		"-p", fmt.Sprintf("127.0.0.1:%d:64738/tcp", port), mumbleImage).CombinedOutput()
	if err != nil {
		t.Skipf("cannot start %s: %v\n%s", mumbleImage, err, out)
	}
	id := strings.TrimSpace(string(out))
	t.Cleanup(func() { exec.Command("docker", "rm", "-f", id).Run() }) //nolint:errcheck

	waitUp := func() {
		eventually(t, 30*time.Second, "mumble server to accept connections", func() bool {
			c, err := dialHuman(addr, "probe", nil)
			if err != nil {
				return false
			}
			c.Disconnect() //nolint:errcheck
			return true
		})
	}
	waitUp()
	return addr, func() {
		// --rm containers vanish on stop, so start a new one on the same port.
		exec.Command("docker", "rm", "-f", id).Run() //nolint:errcheck
		out, err := exec.Command("docker", "run", "-d", "--rm", "--stop-timeout", "2",
			"-p", fmt.Sprintf("127.0.0.1:%d:64738/tcp", port), mumbleImage).CombinedOutput()
		if err != nil {
			t.Fatalf("restart mumble: %v\n%s", err, out)
		}
		id = strings.TrimSpace(string(out))
		waitUp()
	}
}

// icyServer is a SHOUTcast-style radio station: it streams silence as WAV with
// ICY metadata every 100 ms of audio, announcing titles[0] and, after switchAfter,
// titles[1]. It speaks "ICY 200 OK" on a raw socket like a real SHOUTcast server.
func icyServer(t *testing.T, titles [2]string, switchAfter time.Duration) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	start := time.Now()
	const metaint = 9600 // 100 ms of 48 kHz mono s16

	block := func(title string) []byte {
		m := fmt.Sprintf("StreamTitle='%s';", title)
		n := (len(m) + 15) / 16
		b := make([]byte, 1+n*16)
		b[0] = byte(n)
		copy(b[1:], m)
		return b
	}
	serve := func(c net.Conn) {
		defer c.Close()
		buf := make([]byte, 2048)
		n, _ := c.Read(buf)
		wantMeta := strings.Contains(strings.ToLower(string(buf[:n])), "icy-metadata: 1")
		hdr := "ICY 200 OK\r\nicy-name:e2e\r\nContent-Type: audio/wav\r\n"
		if wantMeta {
			hdr += fmt.Sprintf("icy-metaint:%d\r\n", metaint)
		}
		if _, err := c.Write([]byte(hdr + "\r\n")); err != nil {
			return
		}
		wav := []byte("RIFF\xff\xff\xff\xffWAVEfmt \x10\x00\x00\x00\x01\x00\x01\x00" +
			"\x80\xbb\x00\x00\x00\x77\x01\x00\x02\x00\x10\x00data\xff\xff\xff\xff")
		if _, err := c.Write(wav); err != nil {
			return
		}
		last := ""
		// ICY intervals count from the first body byte, WAV header included.
		pending := metaint - len(wav)
		for {
			if _, err := c.Write(make([]byte, pending)); err != nil {
				return
			}
			pending = metaint
			if wantMeta {
				title := titles[0]
				if time.Since(start) >= switchAfter {
					title = titles[1]
				}
				out := []byte{0}
				if title != last {
					out, last = block(title), title
				}
				if _, err := c.Write(out); err != nil {
					return
				}
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go serve(c)
		}
	}()
	return "http://" + ln.Addr().String() + "/live"
}

// human is a plain Mumble client standing in for a person in the channel.
type human struct {
	client *gumble.Client
	mu     sync.Mutex
	texts  []string
}

func dialHuman(addr, name string, onText func(string)) (*gumble.Client, error) {
	cfg := gumble.NewConfig()
	cfg.Username = name
	if onText != nil {
		cfg.Attach(gumbleutil.Listener{TextMessage: func(e *gumble.TextMessageEvent) { onText(e.Message) }})
	}
	return gumble.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, addr, cfg,
		&tls.Config{InsecureSkipVerify: true}) //nolint:gosec // throwaway test server
}

// joinHuman connects a human to the server's root channel.
func joinHuman(t *testing.T, addr, name string) *human {
	t.Helper()
	h := &human{}
	c, err := dialHuman(addr, name, func(m string) {
		h.mu.Lock()
		h.texts = append(h.texts, m)
		h.mu.Unlock()
	})
	if err != nil {
		t.Fatalf("human %s: connect: %v", name, err)
	}
	h.client = c
	t.Cleanup(func() { h.leave() })
	return h
}

func (h *human) leave() {
	if h.client != nil && h.client.State() != gumble.StateDisconnected {
		h.client.Disconnect() //nolint:errcheck
	}
}

func (h *human) say(msg string) {
	h.client.Do(func() { h.client.Self.Channel.Send(msg, false) })
}

func (h *human) heard(substr string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, m := range h.texts {
		if strings.Contains(m, substr) {
			return true
		}
	}
	return false
}

// startBot runs the real bot against addr. extra is raw INI appended to [bot].
func startBot(t *testing.T, addr, extraBot string) *bot.Bot {
	t.Helper()
	host, portStr, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portStr)
	ini := fmt.Sprintf("[server]\nhost = %s\nport = %d\ntls_skip_verify = True\n\n"+
		"[bot]\nusername = e2ebot\nlast_played_path =\n%s\n", host, port, extraBot)
	path := filepath.Join(t.TempDir(), "configuration.ini")
	if err := os.WriteFile(path, []byte(ini), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	b, err := bot.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- b.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("bot did not shut down")
		}
	})
	return b
}

// eventually polls cond until it holds or the timeout expires.
func eventually(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out after %v waiting for %s", timeout, what)
}

// never asserts cond stays false for d.
func never(t *testing.T, d time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			t.Fatalf("unexpected: %s", what)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// silenceServer serves an endless 48 kHz mono s16 WAV of silence, throttled to
// about real time, so ffmpeg keeps "playing" until the bot stops it. It needs
// ffmpeg on the machine, otherwise the test is skipped.
func silenceServer(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hdr := []byte("RIFF\xff\xff\xff\xffWAVEfmt \x10\x00\x00\x00\x01\x00\x01\x00" +
			"\x80\xbb\x00\x00\x00\x77\x01\x00\x02\x00\x10\x00data\xff\xff\xff\xff")
		w.Header().Set("Content-Type", "audio/wav")
		w.Write(hdr)                //nolint:errcheck
		chunk := make([]byte, 9600) // 100 ms
		for {
			if _, err := w.Write(chunk); err != nil {
				return
			}
			w.(http.Flusher).Flush()
			select {
			case <-r.Context().Done():
				return
			case <-time.After(100 * time.Millisecond):
			}
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/silence.wav"
}

// fakeItem is a queue entry. It is never decoded unless ffmpeg is installed
// and the URL is reachable, which the pause logic under test does not need.
type fakeItem struct{ url string }

func (f *fakeItem) StreamURL() string   { return f.url }
func (f *fakeItem) FormatTitle() string { return "[e2e] " + f.url }
