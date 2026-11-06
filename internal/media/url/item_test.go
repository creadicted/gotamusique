package url

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/konradk/gotamusique/internal/config"
)

// fakeYTDLP writes a shell script that mimics yt-dlp and returns a config
// pointing at it. The script appends its argv to <dir>/args.log and dispatches
// on the flags it receives.
func fakeYTDLP(t *testing.T, body string) (*config.Config, string) {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "yt-dlp")
	src := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + filepath.Join(dir, "args.log") + "\n" + body
	if err := os.WriteFile(script, []byte(src), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	cfg.YoutubeDL.Binary = script
	cfg.Bot.MaxTrackDuration = 60
	cfg.Bot.MaxTrackPlaylist = 20
	return cfg, dir
}

const standardBody = `
case "$*" in
  *"-f bestaudio"*)
    case "$*" in
      *hdr-url*) echo '{"id":"h","url":"http://cdn.example/audio.m4a","duration":100,"http_headers":{"Referer":"https://site.example/","User-Agent":"UA-X"},"cookies":"sid=abc; Domain=.site.example; Path=/; Secure; tok=9"}' ;;
      *merged-url*) echo '{"id":"m","duration":100,"http_headers":{"Referer":"https://top/"},"requested_formats":[{"url":"http://cdn.example/a.m4a","http_headers":{"Referer":"https://fmt/"}},{"url":"http://cdn.example/v.mp4"}]}' ;;
      *nourl*) echo '{"id":"n","duration":100}' ;;
      *live-url*) echo '{"id":"lv","url":"http://cdn.example/live.m3u8","http_headers":{}}' ;;
      *) echo '{"id":"a","url":"http://cdn.example/audio.m4a","duration":205,"http_headers":{"Accept":"*/*"}}' ;;
    esac ;;
  *--dump-single-json*)
    case "$*" in
      *playlist-url*) echo '{"_type":"playlist","id":"p","title":"PL"}' ;;
      *live-url*)     echo '{"id":"lv","title":"Live","uploader":"U","is_live":true}' ;;
      *long-url*)     echo '{"id":"l","title":"Long","duration":7200,"uploader":"U"}' ;;
      *)              echo '{"id":"a","title":"Song","uploader":"Artist","duration":205,"thumbnail":"http://t/x.jpg"}' ;;
    esac ;;
  *--flat-playlist*)
    echo '{"_type":"url","id":"1","title":"One","url":"https://www.youtube.com/watch?v=1"}'
    echo '{"_type":"url","id":"2","title":"Two","url":"https://www.youtube.com/watch?v=2"}'
    echo '{"_type":"url","id":"3","title":"Three","url":"https://www.youtube.com/watch?v=3"}' ;;
esac
`

func argLog(t *testing.T, dir string) string {
	t.Helper()
	b, _ := os.ReadFile(filepath.Join(dir, "args.log"))
	return string(b)
}

func TestCheckURL(t *testing.T) {
	for _, ok := range []string{"http://a.com/x", "https://www.youtube.com/watch?v=1"} {
		if err := CheckURL(ok); err != nil {
			t.Errorf("CheckURL(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"", "file:///etc/passwd", "ftp://a.com/x", "-f best", "--exec=id", "javascript:alert(1)", "not a url", "http://"} {
		if err := CheckURL(bad); err == nil {
			t.Errorf("CheckURL(%q) = nil, want error", bad)
		}
	}
}

func TestValidate_SetsMetadata(t *testing.T) {
	cfg, dir := fakeYTDLP(t, standardBody)
	it := New("https://example.com/watch?v=a", cfg)
	if err := it.Validate(); err != nil {
		t.Fatal(err)
	}
	if it.Title != "Song" || it.Uploader != "Artist" || it.Duration != 205*time.Second || it.Thumb != "http://t/x.jpg" {
		t.Errorf("unexpected metadata: %+v", it)
	}
	log := argLog(t, dir)
	if !strings.Contains(log, "--no-playlist") {
		t.Errorf("Validate must pass --no-playlist, args: %s", log)
	}
	if !strings.Contains(log, "-- https://example.com/watch?v=a") {
		t.Errorf("URL must follow --, args: %s", log)
	}
}

func TestValidate_RejectsPlaylist(t *testing.T) {
	cfg, _ := fakeYTDLP(t, standardBody)
	err := New("https://example.com/playlist-url", cfg).Validate()
	if !errors.Is(err, ErrIsPlaylist) {
		t.Errorf("err = %v, want ErrIsPlaylist", err)
	}
}

func TestValidate_PlaylistProbeIsFlatAndNeverFullyResolved(t *testing.T) {
	cfg, dir := fakeYTDLP(t, standardBody)
	_ = New("https://example.com/playlist-url", cfg).Validate()
	log := argLog(t, dir)
	if !strings.Contains(log, "--flat-playlist --playlist-end 1") {
		t.Errorf("probe must be flat and capped at one entry, args: %s", log)
	}
	// Exactly one yt-dlp call: the playlist must be rejected without a full lookup.
	if n := strings.Count(strings.TrimSpace(log), "\n") + 1; n != 1 {
		t.Errorf("yt-dlp called %d times for a playlist, want 1: %s", n, log)
	}
}

func TestValidate_SingleVideoProbesThenFetches(t *testing.T) {
	cfg, dir := fakeYTDLP(t, standardBody)
	if err := New("https://example.com/watch?v=a", cfg).Validate(); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(strings.TrimSpace(argLog(t, dir)), "\n") + 1; n != 2 {
		t.Errorf("yt-dlp called %d times for a video, want 2 (probe + info)", n)
	}
}

func TestValidate_DurationLimit(t *testing.T) {
	cfg, _ := fakeYTDLP(t, standardBody)
	err := New("https://example.com/long-url", cfg).Validate()
	if err == nil || !strings.Contains(err.Error(), "longer than 60 min") {
		t.Errorf("err = %v, want duration error", err)
	}
	cfg.Bot.MaxTrackDuration = 0 // no limit
	if err := New("https://example.com/long-url", cfg).Validate(); err != nil {
		t.Errorf("limit 0 should allow any length, got %v", err)
	}
}

func TestValidate_BadSchemeDoesNotRunBinary(t *testing.T) {
	cfg, dir := fakeYTDLP(t, standardBody)
	if err := New("file:///etc/passwd", cfg).Validate(); err == nil {
		t.Fatal("want error")
	}
	if log := argLog(t, dir); log != "" {
		t.Errorf("binary must not run for rejected URL, args: %s", log)
	}
}

func TestValidate_YtdlpFailure(t *testing.T) {
	cfg, _ := fakeYTDLP(t, "echo 'ERROR: Video unavailable' >&2\nexit 1\n")
	err := New("https://example.com/x", cfg).Validate()
	if err == nil || !strings.Contains(err.Error(), "Video unavailable") {
		t.Errorf("err = %v, want stderr surfaced", err)
	}
}

func TestMissingBinary(t *testing.T) {
	cfg := &config.Config{}
	cfg.YoutubeDL.Binary = filepath.Join(t.TempDir(), "nope")
	err := New("https://example.com/x", cfg).Validate()
	if !errors.Is(err, ErrBinaryMissing) {
		t.Errorf("err = %v, want ErrBinaryMissing", err)
	}
}

func TestResolve_SetsStreamURL(t *testing.T) {
	cfg, _ := fakeYTDLP(t, standardBody)
	it := New("https://example.com/watch?v=a", cfg)
	if it.StreamURL() != "" {
		t.Error("StreamURL must be empty before Resolve")
	}
	if err := it.Resolve(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := it.StreamURL(); got != "http://cdn.example/audio.m4a" {
		t.Errorf("StreamURL = %q", got)
	}
	if it.Title != "Song" {
		t.Errorf("Resolve should fill unknown metadata, Title = %q", it.Title)
	}
}

func TestResolve_EnforcesDurationForFlatEntry(t *testing.T) {
	cfg, _ := fakeYTDLP(t, standardBody)
	it := New("https://example.com/long-url", cfg) // duration unknown, as for a flat entry
	err := it.Resolve(context.Background())
	if err == nil || !strings.Contains(err.Error(), "longer than 60 min") {
		t.Errorf("err = %v, want duration error", err)
	}
	if it.StreamURL() != "" {
		t.Error("StreamURL must stay empty when Resolve fails")
	}
}

func TestResolve_Timeout(t *testing.T) {
	cfg, _ := fakeYTDLP(t, "sleep 5\n")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := New("https://example.com/x", cfg).Resolve(ctx)
	if err == nil {
		t.Fatal("want timeout error")
	}
	if time.Since(start) > 3*time.Second {
		t.Error("timeout did not cancel the process promptly")
	}
}

func TestConfigFlagsPassed(t *testing.T) {
	cfg, dir := fakeYTDLP(t, standardBody)
	cfg.YoutubeDL.CookieFile = "/c.txt"
	cfg.YoutubeDL.SourceAddress = "1.2.3.4"
	cfg.YoutubeDL.UserAgent = "UA"
	if _, err := GetInfo(context.Background(), "https://example.com/a", cfg); err != nil {
		t.Fatal(err)
	}
	log := argLog(t, dir)
	for _, want := range []string{"--cookies /c.txt", "--source-address 1.2.3.4", "--user-agent UA"} {
		if !strings.Contains(log, want) {
			t.Errorf("args missing %q: %s", want, log)
		}
	}
}

func TestPlaylist_CapsAndIsLazy(t *testing.T) {
	cfg, dir := fakeYTDLP(t, standardBody)
	cfg.Bot.MaxTrackPlaylist = 2
	items, err := Playlist(context.Background(), "https://example.com/list", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("len = %d, want 2 (capped)", len(items))
	}
	if items[0].Title != "One" || items[0].URL != "https://www.youtube.com/watch?v=1" {
		t.Errorf("unexpected first item: %+v", items[0])
	}
	if items[0].StreamURL() != "" {
		t.Error("playlist items must be unresolved")
	}
	if !strings.Contains(argLog(t, dir), "--playlist-end 2") {
		t.Error("expected --playlist-end 2")
	}
}

func TestPlaylist_Empty(t *testing.T) {
	cfg, _ := fakeYTDLP(t, "true\n")
	if _, err := Playlist(context.Background(), "https://example.com/list", cfg); err == nil {
		t.Error("want error for empty playlist")
	}
}

func TestFormatTitle(t *testing.T) {
	it := &URLItem{URL: "https://x/y", Title: "Song", Uploader: "Artist", Duration: 205 * time.Second}
	if got := it.FormatTitle(); got != "Song - Artist [3:25]" {
		t.Errorf("got %q", got)
	}
	it = &URLItem{URL: "https://x/y"}
	if got := it.FormatTitle(); got != "https://x/y" {
		t.Errorf("got %q", got)
	}
	it = &URLItem{Title: "Long", Duration: 3725 * time.Second}
	if got := it.FormatTitle(); got != "Long [1:02:05]" {
		t.Errorf("got %q", got)
	}
}

func TestResolve_PassesHeadersAndCookies(t *testing.T) {
	cfg, _ := fakeYTDLP(t, standardBody)
	it := New("https://example.com/hdr-url", cfg)
	if err := it.Resolve(context.Background()); err != nil {
		t.Fatal(err)
	}
	opts := it.StreamOptions()
	if opts.Headers["Referer"] != "https://site.example/" || opts.Headers["User-Agent"] != "UA-X" {
		t.Errorf("headers = %v", opts.Headers)
	}
	want := []string{"sid=abc; Domain=.site.example; Path=/; Secure", "tok=9"}
	if len(opts.Cookies) != 2 || opts.Cookies[0] != want[0] || opts.Cookies[1] != want[1] {
		t.Errorf("cookies = %q, want %q", opts.Cookies, want)
	}
	if !opts.Finite {
		t.Error("track with a duration must be finite")
	}
}

func TestResolve_MergedFormatsUseFirstFormat(t *testing.T) {
	cfg, _ := fakeYTDLP(t, standardBody)
	it := New("https://example.com/merged-url", cfg)
	if err := it.Resolve(context.Background()); err != nil {
		t.Fatal(err)
	}
	if it.StreamURL() != "http://cdn.example/a.m4a" {
		t.Errorf("StreamURL = %q", it.StreamURL())
	}
	if got := it.StreamOptions().Headers["Referer"]; got != "https://fmt/" {
		t.Errorf("Referer = %q, want the format's own header", got)
	}
}

func TestResolve_NoURLInOutput_Errors(t *testing.T) {
	cfg, _ := fakeYTDLP(t, standardBody)
	it := New("https://example.com/nourl", cfg)
	if err := it.Resolve(context.Background()); err == nil || !strings.Contains(err.Error(), "no stream URL") {
		t.Errorf("err = %v, want no stream URL", err)
	}
}

func TestStreamOptions_LiveIsNotFinite(t *testing.T) {
	cfg, _ := fakeYTDLP(t, standardBody)
	it := New("https://example.com/live-url", cfg)
	it.Duration = 0
	// Resolve fetches metadata first (unknown duration); the live fixture has none.
	if err := it.Resolve(context.Background()); err != nil {
		t.Fatal(err)
	}
	if it.StreamOptions().Finite {
		t.Error("stream without a duration must keep reconnect-at-EOF (not finite)")
	}
}

func TestStreamOptions_BeforeResolve_NoHeaders(t *testing.T) {
	cfg, _ := fakeYTDLP(t, standardBody)
	opts := New("https://example.com/x", cfg).StreamOptions()
	if len(opts.Headers) != 0 || len(opts.Cookies) != 0 {
		t.Errorf("unexpected options before Resolve: %+v", opts)
	}
}

func TestSplitCookies(t *testing.T) {
	got := splitCookies("a=1; Domain=.x.com; Path=/; Secure; b=2; Expires=123; c=3")
	want := []string{"a=1; Domain=.x.com; Path=/; Secure", "b=2; Expires=123", "c=3"}
	if len(got) != len(want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if len(splitCookies("")) != 0 {
		t.Error("empty input must yield no cookies")
	}
}
