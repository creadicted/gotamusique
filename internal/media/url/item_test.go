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
  *--dump-single-json*)
    case "$*" in
      *playlist-url*) echo '{"_type":"playlist","id":"p","title":"PL"}' ;;
      *long-url*)     echo '{"id":"l","title":"Long","duration":7200,"uploader":"U"}' ;;
      *)              echo '{"id":"a","title":"Song","uploader":"Artist","duration":205,"thumbnail":"http://t/x.jpg"}' ;;
    esac ;;
  *"-g"*)  echo 'http://cdn.example/audio.m4a' ;;
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
