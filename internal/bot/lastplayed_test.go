package bot

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/konradk/gotamusique/internal/hls"
	fileitem "github.com/konradk/gotamusique/internal/media/file"
	urlitem "github.com/konradk/gotamusique/internal/media/url"
	"github.com/konradk/gotamusique/internal/radio"
)

var quiet = slog.New(slog.DiscardHandler)

func lastBot(t *testing.T, folder string) (*Bot, string) {
	t.Helper()
	cfg := defaultCfg()
	cfg.Bot.LastPlayedPath = filepath.Join(t.TempDir(), "state", "last.json")
	cfg.Files.MusicFolder = folder
	cfg.Bot.MusicDatabasePath = filepath.Join(t.TempDir(), "music.db")
	b, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return b, cfg.Bot.LastPlayedPath
}

// restart builds a second bot reading the first one's state file.
func restart(t *testing.T, folder, path string) *Bot {
	t.Helper()
	cfg := defaultCfg()
	cfg.Bot.LastPlayedPath = path
	cfg.Files.MusicFolder = folder
	cfg.Bot.MusicDatabasePath = filepath.Join(t.TempDir(), "music.db")
	b, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestLastPlayed_radioRoundTrip(t *testing.T) {
	b, path := lastBot(t, "")
	b.saveLastPlayed(&radio.RadioItem{URL: "http://x/stream", Name: "Jazz"}, 0)

	b2 := restart(t, "", path)
	title, at, ok := b2.RestoreLast()
	if !ok || title != "[Radio] Jazz" || at != 0 {
		t.Fatalf("RestoreLast = %q, %v, %v", title, at, ok)
	}
	if got := b2.queue.Current(); got == nil || got.StreamURL() != "http://x/stream" {
		t.Errorf("queued %v, want the radio stream", got)
	}
}

func TestLastPlayed_hlsRoundTrip(t *testing.T) {
	b, path := lastBot(t, "")
	b.saveLastPlayed(&hls.HLSItem{URL: "http://x/live.m3u8", Name: "live"}, 0)
	if title, _, ok := restart(t, "", path).RestoreLast(); !ok || title != "[HLS] live" {
		t.Errorf("RestoreLast = %q, %v", title, ok)
	}
}

func TestLastPlayed_urlRoundTripIsLazy(t *testing.T) {
	b, path := lastBot(t, "")
	it := urlitem.New("https://youtu.be/abc", b.cfg)
	it.Title, it.Uploader = "Song", "Band"
	b.saveLastPlayed(it, time.Minute) // not seekable: position must not be kept

	b2 := restart(t, "", path)
	if _, at, ok := b2.RestoreLast(); !ok || at != 0 {
		t.Errorf("url item: ok=%v at=%v, want true / 0", ok, at)
	}
}

func musicFolderWith(t *testing.T, rel string) string {
	t.Helper()
	dir := t.TempDir()
	full := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestLastPlayed_fileResumesAtPosition(t *testing.T) {
	folder := musicFolderWith(t, "rock/a.flac")
	b, path := lastBot(t, folder)
	it := fileitem.New("rock/a.flac", folder)
	it.Title, it.Artist, it.Duration = "A", "Band", 5*time.Minute
	b.saveLastPlayed(it, 84*time.Second)

	b2 := restart(t, folder, path)
	title, at, ok := b2.RestoreLast()
	if !ok || title != "Band - A" || at != 84*time.Second {
		t.Fatalf("RestoreLast = %q, %v, %v", title, at, ok)
	}
	cur := b2.queue.Current()
	if got, taken := b2.resume.take(cur); !taken || got != 84*time.Second {
		t.Errorf("resume point = %v, %v; want 84s for the queued file", got, taken)
	}
}

func TestLastPlayed_fileNearEndStartsFromTop(t *testing.T) {
	folder := musicFolderWith(t, "a.flac")
	b, path := lastBot(t, folder)
	it := fileitem.New("a.flac", folder)
	it.Duration = 3 * time.Minute
	b.saveLastPlayed(it, 3*time.Minute-time.Second)

	if _, at, ok := restart(t, folder, path).RestoreLast(); !ok || at != 0 {
		t.Errorf("at=%v ok=%v, want 0 / true: a finished track restarts", at, ok)
	}
}

func TestLastPlayed_deletedFileRestoresNothing(t *testing.T) {
	folder := musicFolderWith(t, "a.flac")
	b, path := lastBot(t, folder)
	b.saveLastPlayed(fileitem.New("a.flac", folder), 10*time.Second)
	os.Remove(filepath.Join(folder, "a.flac"))

	b2 := restart(t, folder, path)
	if _, _, ok := b2.RestoreLast(); ok {
		t.Error("restored a file that no longer exists")
	}
	if b2.queue.Len() != 0 {
		t.Error("queued something anyway")
	}
}

func TestRestoreLast_onlyOnceAndOnlyOnEmptyQueue(t *testing.T) {
	b, path := lastBot(t, "")
	b.saveLastPlayed(&radio.RadioItem{URL: "http://x/s", Name: "R"}, 0)

	b2 := restart(t, "", path)
	b2.Enqueue(&mockItem{"already"})
	if _, _, ok := b2.RestoreLast(); ok {
		t.Error("restored over a non-empty queue")
	}

	b3 := restart(t, "", path)
	if _, _, ok := b3.RestoreLast(); !ok {
		t.Fatal("first RestoreLast failed")
	}
	b3.Clear()
	if _, _, ok := b3.RestoreLast(); ok {
		t.Error("RestoreLast ran twice")
	}
}

func TestLastPlayed_disabledAndUnreadable(t *testing.T) {
	b, _ := lastBot(t, "")
	b.cfg.Bot.LastPlayedPath = ""
	b.saveLastPlayed(&radio.RadioItem{URL: "http://x", Name: "R"}, 0) // must not panic or write
	if loadLastPlayed("", quiet) != nil {
		t.Error("empty path should mean nothing saved")
	}
	bad := filepath.Join(t.TempDir(), "bad.json")
	os.WriteFile(bad, []byte("{not json"), 0o644)
	if loadLastPlayed(bad, quiet) != nil {
		t.Error("corrupt file should be ignored")
	}
	if loadLastPlayed(filepath.Join(t.TempDir(), "missing.json"), quiet) != nil {
		t.Error("missing file should be ignored")
	}
}

func TestLastPlayed_noTempFileLeftBehind(t *testing.T) {
	b, path := lastBot(t, "")
	b.saveLastPlayed(&radio.RadioItem{URL: "http://x", Name: "R"}, 0)
	if _, err := os.Stat(path + ".tmp"); err == nil {
		t.Error("temp file left behind after a successful save")
	}
}

func TestResume_notPausedRestoresLastItem(t *testing.T) {
	b, path := lastBot(t, "")
	b.saveLastPlayed(&radio.RadioItem{URL: "http://x/s", Name: "R"}, 0)
	b2 := restart(t, "", path)
	if b2.Resume() {
		t.Fatal("Resume() reported a pause on a fresh bot")
	}
	if _, _, ok := b2.RestoreLast(); !ok {
		t.Error("RestoreLast should work on a fresh bot after Resume() found no pause")
	}
}
