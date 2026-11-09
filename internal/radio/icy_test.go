package radio

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

func icyBlock(title string) []byte {
	s := fmt.Sprintf("StreamTitle='%s';", title)
	n := (len(s) + 15) / 16
	b := make([]byte, n*16)
	copy(b, s)
	return append([]byte{byte(n)}, b...)
}

// serveICY answers one connection with a SHOUTcast-style "ICY 200 OK" response.
func serveICY(t *testing.T, metaint int, titles ...string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		buf := make([]byte, 1024)
		c.Read(buf) //nolint:errcheck
		fmt.Fprintf(c, "ICY 200 OK\r\nicy-metaint:%d\r\nicy-name:Test\r\n\r\n", metaint)
		audio := make([]byte, metaint)
		for _, title := range titles {
			c.Write(audio)           //nolint:errcheck
			c.Write(icyBlock(title)) //nolint:errcheck
			c.Write(audio)           //nolint:errcheck
			c.Write([]byte{0})       //nolint:errcheck // same title again: no block
		}
		time.Sleep(200 * time.Millisecond)
	}()
	return "http://" + ln.Addr().String() + "/stream"
}

func TestWatchICY_reportsTitleChanges(t *testing.T) {
	url := serveICY(t, 64, "Artist A - Song 1", "Artist A - Song 1", "Artist B - Song 2")
	var got []string
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	WatchICY(ctx, url, func(s string) { got = append(got, s) }) //nolint:errcheck
	want := "Artist A - Song 1|Artist B - Song 2"
	if strings.Join(got, "|") != want {
		t.Errorf("got %v, want repeated title collapsed: %s", got, want)
	}
}

func TestWatchICY_noMetadata(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		c.Read(make([]byte, 1024)) //nolint:errcheck
		fmt.Fprint(c, "HTTP/1.0 200 OK\r\nContent-Type: audio/mpeg\r\n\r\n")
	}()
	err := WatchICY(context.Background(), "http://"+ln.Addr().String(), func(string) { t.Error("unexpected title") })
	if err == nil || !strings.Contains(err.Error(), "no metadata") {
		t.Errorf("err = %v, want no-metadata error", err)
	}
}

func TestWatchICY_cancelStops(t *testing.T) {
	url := serveICY(t, 64)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { WatchICY(ctx, url, func(string) {}); close(done) }() //nolint:errcheck
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("WatchICY did not stop after cancel")
	}
}

func TestParseStreamTitle(t *testing.T) {
	cases := map[string]string{
		"StreamTitle='Artist - Song';StreamUrl='';": "Artist - Song",
		"StreamTitle='It's Me - Don't';":            "It's Me - Don't",
		"StreamTitle='';":                           "",
		"StreamUrl='x';":                            "",
		"StreamTitle='A\x00\x00":                    "A",
		"StreamTitle='  Spaced  ';":                 "Spaced",
	}
	for in, want := range cases {
		if got := ParseStreamTitle(in); got != want {
			t.Errorf("ParseStreamTitle(%q) = %q, want %q", in, got, want)
		}
	}
	long := "StreamTitle='" + strings.Repeat("x", 500) + "';"
	if got := ParseStreamTitle(long); len([]rune(got)) != icyMaxTitleLen {
		t.Errorf("length = %d, want %d", len([]rune(got)), icyMaxTitleLen)
	}
}
