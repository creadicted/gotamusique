package bot

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/konradk/gotamusique/internal/config"
	"github.com/konradk/gotamusique/internal/library"
	"github.com/konradk/gotamusique/internal/media/file"
	urlitem "github.com/konradk/gotamusique/internal/media/url"
	"github.com/konradk/gotamusique/internal/queue"
	"github.com/konradk/gotamusique/internal/status"
	"github.com/konradk/gotamusique/internal/web"
)

var bg = context.Background()

func TestAdd_LibraryFile(t *testing.T) {
	lb := newLibBot(t, 0, nil)
	lb.addTrack(t, "rock/a.mp3", true)
	lb.addTrack(t, "rock/b.mp3", true)

	got, err := lb.Add(bg, web.Source{Type: web.SourceLibrary, Path: "rock/b.mp3"}, queue.AtEnd)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID == 0 || !strings.Contains(got.Title, "Title rock/b.mp3") {
		t.Errorf("added = %+v", got)
	}
	if e := lb.Snapshot().Queue; len(e) != 1 || e[0].ID != got.ID || e[0].Duration != 120*time.Second || !e[0].Seekable {
		t.Errorf("queue = %+v", e)
	}
}

func TestAdd_LibraryFileRejections(t *testing.T) {
	lb := newLibBot(t, 0, nil)
	lb.addTrack(t, "gone.mp3", false) // in the library, deleted from disk
	lb.addTrack(t, "ok.mp3", true)

	cases := []struct {
		path string
		want error
	}{
		{"nope.mp3", library.ErrNotInLibrary},
		{"", web.ErrBadSource},
		{"../../etc/passwd", library.ErrNotInLibrary},
		{"ok.mp3/../ok.mp3", library.ErrNotInLibrary}, // not the stored spelling
		{"gone.mp3", web.ErrBadSource},
	}
	for _, c := range cases {
		_, err := lb.Add(bg, web.Source{Type: web.SourceLibrary, Path: c.path}, queue.AtEnd)
		if !errors.Is(err, c.want) {
			t.Errorf("path %q: err = %v, want %v", c.path, err, c.want)
		}
	}
	if len(lb.Snapshot().Queue) != 0 {
		t.Error("a rejected source must not be queued")
	}

	b, _ := New(defaultCfg()) // no library at all
	if _, err := b.Add(bg, web.Source{Type: web.SourceLibrary, Path: "x.mp3"}, queue.AtEnd); !errors.Is(err, library.ErrUnavailable) {
		t.Errorf("no library: err = %v, want ErrUnavailable", err)
	}
}

func TestAdd_RadioPresetAndValidation(t *testing.T) {
	cfg := defaultCfg()
	cfg.Radio = map[string]config.RadioPreset{"jazz": {URL: "http://radio.example/jazz", Comment: "Jazz"}}
	b, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	got, err := b.Add(bg, web.Source{Type: web.SourceRadio, Preset: "jazz"}, queue.AtEnd)
	if err != nil || got.Title != "[Radio] Jazz" {
		t.Fatalf("preset = %+v, %v", got, err)
	}
	bad := []web.Source{
		{Type: web.SourceRadio, Preset: "nope"},
		{Type: web.SourceRadio},                              // nothing given
		{Type: web.SourceRadio, Preset: "jazz", UUID: "abc"}, // two given
		{Type: web.SourceRadio, URL: "ftp://x/y"},            // not http(s)
		{Type: web.SourceRadio, URL: "http://"},              // no host
		{Type: "playlist"},                                   // unknown type
		{Type: web.SourceURL, URL: "file:///etc/passwd"},     // yt-dlp only takes http(s)
		{Type: web.SourceURL, URL: "--exec=id"},
	}
	for _, src := range bad {
		if _, err := b.Add(bg, src, queue.AtEnd); !errors.Is(err, web.ErrBadSource) {
			t.Errorf("%+v: err = %v, want ErrBadSource", src, err)
		}
	}
	if n := len(b.Snapshot().Queue); n != 1 {
		t.Errorf("queue has %d entries, want only the valid preset", n)
	}
}

func TestAdd_RadioURLChecksReachability(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer up.Close()
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) }))
	defer down.Close()

	b, _ := New(defaultCfg())
	if _, err := b.Add(bg, web.Source{Type: web.SourceRadio, URL: up.URL}, queue.AtEnd); err != nil {
		t.Errorf("reachable stream: %v", err)
	}
	if _, err := b.Add(bg, web.Source{Type: web.SourceRadio, URL: down.URL}, queue.AtEnd); !errors.Is(err, web.ErrBadSource) {
		t.Errorf("404 stream: err = %v, want ErrBadSource", err)
	}
}

func TestAdd_RadioURLHonoursTheDeadline(t *testing.T) {
	release := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-release }))
	defer slow.Close()
	defer close(release)

	b, _ := New(defaultCfg())
	ctx, cancel := context.WithTimeout(bg, 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := b.Add(ctx, web.Source{Type: web.SourceRadio, URL: slow.URL}, queue.AtEnd)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want DeadlineExceeded", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Error("Add kept waiting after its context ended")
	}
}

func TestAdd_Position(t *testing.T) {
	b := pauseBot(t, false)
	cfg := b.cfg
	cfg.Radio = map[string]config.RadioPreset{"r": {URL: "http://radio.example/r", Comment: "R"}}
	enqueue3(b)
	_ = b.Play(0)
	next, err := b.Add(bg, web.Source{Type: web.SourceRadio, Preset: "r"}, queue.AfterCurrent)
	if err != nil {
		t.Fatal(err)
	}
	last, _ := b.Add(bg, web.Source{Type: web.SourceRadio, Preset: "r"}, queue.AtEnd)
	s := b.Snapshot()
	if got := indexOfID(s.Queue, next.ID); got != 1 {
		t.Errorf("after-current entry at %d, want 1", got)
	}
	if got := indexOfID(s.Queue, last.ID); got != len(s.Queue)-1 {
		t.Errorf("end entry at %d, want %d", got, len(s.Queue)-1)
	}
}

func indexOfID(es []status.Entry, id uint64) int {
	for i, e := range es {
		if e.ID == id {
			return i
		}
	}
	return -1
}

// ---- yt-dlp ----

const ytBody = `
case "$*" in
  *--dump-single-json*)
    case "$*" in
      *playlist-url*) echo '{"_type":"playlist","id":"p","title":"PL"}' ;;
      *long-url*)     echo '{"id":"l","title":"Long","duration":7200,"uploader":"U"}' ;;
      *)              echo '{"id":"a","title":"Song","uploader":"Artist","duration":205}' ;;
    esac ;;
esac
`

func ytBot(t *testing.T, body string) *Bot {
	t.Helper()
	script := filepath.Join(t.TempDir(), "yt-dlp")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := defaultCfg()
	cfg.YoutubeDL.Binary = script
	cfg.Bot.MaxTrackDuration = 60
	b, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestAdd_URL(t *testing.T) {
	b := ytBot(t, ytBody)
	got, err := b.Add(bg, web.Source{Type: web.SourceURL, URL: "https://example.com/watch?v=ok"}, queue.AtEnd)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Title, "Song") {
		t.Errorf("title = %q", got.Title)
	}
	if e := b.Snapshot().Queue; len(e) != 1 || e[0].Duration != 205*time.Second {
		t.Errorf("queue = %+v, want one entry of 3:25 (URL items report their length)", e)
	}

	for url, why := range map[string]string{
		"https://example.com/playlist-url": "a playlist",
		"https://example.com/long-url":     "over max_track_duration",
	} {
		if _, err := b.Add(bg, web.Source{Type: web.SourceURL, URL: url}, queue.AtEnd); !errors.Is(err, web.ErrBadSource) {
			t.Errorf("%s: err = %v, want ErrBadSource", why, err)
		}
	}
	if len(b.Snapshot().Queue) != 1 {
		t.Error("rejected URLs must not be queued")
	}
}

func TestAdd_URLMissingBinaryIsAServerProblem(t *testing.T) {
	cfg := defaultCfg()
	cfg.YoutubeDL.Binary = filepath.Join(t.TempDir(), "nope")
	b, _ := New(cfg)
	_, err := b.Add(bg, web.Source{Type: web.SourceURL, URL: "https://example.com/x"}, queue.AtEnd)
	if !errors.Is(err, urlitem.ErrBinaryMissing) || errors.Is(err, web.ErrBadSource) {
		t.Errorf("err = %v, want ErrBinaryMissing and not the user's fault", err)
	}
}

// blockingYT is a fake yt-dlp that signals it started (a file per process) and then
// waits until the test creates "release", so a test can hold yt-dlp slots.
func blockingYT(dir string) string {
	return "touch \"" + dir + "/started.$$\"\n" +
		"while [ ! -f \"" + dir + "/release\" ]; do sleep 0.02; done\n" +
		`echo '{"id":"a","title":"Song","duration":100}'` + "\n"
}

func startedCount(dir string) int {
	m, _ := filepath.Glob(filepath.Join(dir, "started.*"))
	return len(m)
}

// Every yt-dlp call shares one bound (see urlitem.MaxConcurrent). With all slots
// taken, a web add gives up at its own deadline as "busy", not as a slow site, and
// never launches a fourth process.
func TestAdd_URLBusyWhenEverySlotIsTaken(t *testing.T) {
	dir := t.TempDir()
	b := ytBot(t, blockingYT(dir))

	var wg sync.WaitGroup
	for i := 0; i < urlitem.MaxConcurrent; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(bg, 20*time.Second)
			defer cancel()
			if _, err := b.Add(ctx, web.Source{Type: web.SourceURL, URL: "https://example.com/hold"}, queue.AtEnd); err != nil {
				t.Errorf("a holder failed: %v", err)
			}
		}()
	}
	deadline := time.Now().Add(10 * time.Second)
	for startedCount(dir) < urlitem.MaxConcurrent && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if startedCount(dir) < urlitem.MaxConcurrent {
		t.Fatal("the holders did not start")
	}

	ctx, cancel := context.WithTimeout(bg, 100*time.Millisecond)
	defer cancel()
	if _, err := b.Add(ctx, web.Source{Type: web.SourceURL, URL: "https://example.com/late"}, queue.AtEnd); !errors.Is(err, web.ErrBusy) {
		t.Errorf("err = %v, want ErrBusy", err)
	}
	if n := startedCount(dir); n != urlitem.MaxConcurrent {
		t.Errorf("%d yt-dlp processes ran, want %d", n, urlitem.MaxConcurrent)
	}

	if err := os.WriteFile(filepath.Join(dir, "release"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	if n := len(b.Snapshot().Queue); n != urlitem.MaxConcurrent {
		t.Errorf("%d tracks queued, want the %d holders'", n, urlitem.MaxConcurrent)
	}
}

// ---- rescan ----

func TestStartRescan(t *testing.T) {
	b, _ := New(defaultCfg())
	if err := b.StartRescan(); !errors.Is(err, library.ErrUnavailable) {
		t.Errorf("no library: err = %v, want ErrUnavailable", err)
	}

	lb := newLibBot(t, 0, nil)
	lb.addTrack(t, "a.mp3", true)
	if s := lb.Snapshot().Scan; !s.Available || s.Running || s.LastResult != "" || s.LastError != "" {
		t.Errorf("initial scan state = %+v", s)
	}
	if err := lb.StartRescan(); err != nil {
		t.Fatal(err)
	}
	// The scan runs in the background; its outcome appears in the snapshot. ffprobe
	// is not available for the fake "x" file, so the scan skips it and finishes.
	deadline := time.Now().Add(10 * time.Second)
	var s struct{ done bool }
	for time.Now().Before(deadline) {
		sc := lb.Snapshot().Scan
		if !sc.Running && (sc.LastResult != "" || sc.LastError != "") {
			s.done = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !s.done {
		t.Fatalf("the scan outcome never showed up in the snapshot: %+v", lb.Snapshot().Scan)
	}
}

func TestRescan_RecordsTheOutcomeInTheSnapshot(t *testing.T) {
	lb := newLibBot(t, 0, nil)
	lb.addTrack(t, "a.mp3", true)
	v := lb.Snapshot().Version
	if _, err := lb.Rescan(bg, false); err != nil {
		// A fake file ffprobe cannot read is skipped, not an error; any error here
		// would be recorded instead, which the next assertion also accepts.
		t.Logf("rescan: %v", err)
	}
	sc := lb.Snapshot().Scan
	if sc.LastResult == "" && sc.LastError == "" {
		t.Fatalf("outcome not recorded: %+v", sc)
	}
	if lb.Snapshot().Version == v {
		t.Error("a finished scan must change the snapshot version")
	}
}

func TestStartRescan_RefusesWhileAScanRuns(t *testing.T) {
	lb := newLibBot(t, 0, nil)
	lb.addTrack(t, "a.mp3", true)
	lib := lb.libraryRef()

	entered, release := make(chan struct{}), make(chan struct{})
	lib.scanner = library.NewScanner(lib.db, library.Config{
		MusicFolder: lb.root,
		Probe: func(string) (file.Metadata, error) {
			close(entered)
			<-release
			return file.Metadata{Title: "t"}, nil
		},
	})
	// The row inserted by addTrack has no size/mtime, so the scan must probe it.
	if err := lb.StartRescan(); err != nil {
		t.Fatal(err)
	}
	// The answer to the request already shows the scan as running, before the
	// background goroutine has had a chance to start the scanner.
	if !lb.Snapshot().Scan.Running {
		t.Error("the snapshot right after StartRescan must report the scan as running")
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the scan never started")
	}
	if !lb.Snapshot().Scan.Running {
		t.Error("the snapshot should report the running scan")
	}
	if err := lb.StartRescan(); !errors.Is(err, library.ErrScanRunning) {
		t.Errorf("second StartRescan = %v, want ErrScanRunning", err)
	}
	close(release)

	deadline := time.Now().Add(5 * time.Second)
	for lb.Snapshot().Scan.Running && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if sc := lb.Snapshot().Scan; sc.Running || sc.LastResult == "" {
		t.Errorf("after the scan: %+v", sc)
	}
}

func TestRadioPresets_SortedAndWithoutURLs(t *testing.T) {
	cfg := defaultCfg()
	cfg.Radio = map[string]config.RadioPreset{
		"rock": {URL: "http://secret.example/rock"},
		"jazz": {URL: "http://secret.example/jazz", Comment: "Jazz Yeah !"},
		"blue": {URL: "http://secret.example/blue", Comment: "Blues"},
	}
	b, _ := New(cfg)
	got := b.RadioPresets()
	if len(got) != 3 || got[0].Name != "blue" || got[1].Name != "jazz" || got[2].Name != "rock" {
		t.Fatalf("presets = %+v, want blue, jazz, rock", got)
	}
	if got[1].Comment != "Jazz Yeah !" || got[2].Comment != "" {
		t.Errorf("comments = %+v", got)
	}
	if empty, _ := New(defaultCfg()); empty.RadioPresets() == nil {
		t.Error("no presets must be an empty slice, not nil")
	}
}
