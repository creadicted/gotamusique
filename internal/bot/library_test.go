package bot

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/konradk/gotamusique/internal/audio"
	"github.com/konradk/gotamusique/internal/command"
	"github.com/konradk/gotamusique/internal/config"
	"github.com/konradk/gotamusique/internal/database"
	"github.com/konradk/gotamusique/internal/library"
	"github.com/konradk/gotamusique/internal/media/file"
	"github.com/konradk/gotamusique/internal/queue"
)

// libBot builds a Bot with a real music DB on a temp music folder. The folder
// gets n files t00.mp3...; rows are inserted straight into the DB so ffprobe is
// never needed.
type libBot struct {
	*Bot
	dir  string
	cfg  *config.Config
	root string
}

func newLibBot(t *testing.T, n int, mutate func(*config.Config)) *libBot {
	t.Helper()
	root := t.TempDir()
	cfg := defaultCfg()
	cfg.Files.MusicFolder = root
	cfg.Bot.MusicDatabasePath = filepath.Join(t.TempDir(), "data", "music.db") // dir must be created
	cfg.Bot.AutoplayLength = 3
	if mutate != nil {
		mutate(cfg)
	}
	b, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(b.closeLibrary)
	lb := &libBot{Bot: b, cfg: cfg, root: root}
	for i := 0; i < n; i++ {
		lb.addTrack(t, fmt.Sprintf("t%02d.mp3", i), true)
	}
	return lb
}

// addTrack inserts a library row and (optionally) creates the file on disk.
func (l *libBot) addTrack(t *testing.T, rel string, onDisk bool) {
	t.Helper()
	lib := l.libraryRef()
	if lib == nil {
		t.Fatal("no library open")
	}
	if onDisk {
		p := filepath.Join(l.root, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	err := lib.db.Insert(database.MusicRecord{
		ID: file.IDFromPath(rel), Type: library.RecordType, Title: "Title " + rel, Path: rel,
		Keywords: "title " + rel, Metadata: map[string]any{"artist": "Art", "duration": float64(120)},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func queuePaths(b *Bot) []string {
	var out []string
	for _, it := range b.QueueItems() {
		if fi, ok := it.(*file.FileItem); ok {
			out = append(out, fi.Path)
		}
	}
	return out
}

// ---------------- library wiring ----------------

func TestNew_NoMusicFolder_NoLibrary(t *testing.T) {
	b, err := New(defaultCfg())
	if err != nil {
		t.Fatal(err)
	}
	if b.HasLibrary() {
		t.Error("no music_folder: library must not open")
	}
	if _, err := os.Stat("music.db"); err == nil {
		t.Error("a database file was created although music_folder is unset")
		os.Remove("music.db")
	}
}

func TestNew_WithMusicFolder_OpensLibraryAndCreatesDBDir(t *testing.T) {
	lb := newLibBot(t, 0, nil)
	if !lb.HasLibrary() {
		t.Fatal("library should be open")
	}
	if _, err := os.Stat(lb.cfg.Bot.MusicDatabasePath); err != nil {
		t.Errorf("database file not created (including its parent dir): %v", err)
	}
}

func TestNew_UnopenableDatabase_BotStillStartsWithoutLibrary(t *testing.T) {
	cfg := defaultCfg()
	cfg.Files.MusicFolder = t.TempDir()
	blocker := filepath.Join(t.TempDir(), "afile")
	os.WriteFile(blocker, []byte("x"), 0o644)
	cfg.Bot.MusicDatabasePath = filepath.Join(blocker, "music.db") // parent is a file: cannot mkdir
	b, err := New(cfg)
	if err != nil {
		t.Fatalf("a broken library must not stop the bot: %v", err)
	}
	if b.HasLibrary() {
		t.Error("library should be disabled when the database cannot be opened")
	}
	// Everything library-related degrades cleanly.
	if _, err := b.Rescan(context.Background(), false); !errors.Is(err, library.ErrUnavailable) {
		t.Errorf("Rescan err = %v, want ErrUnavailable", err)
	}
	if err := b.SetPlaybackMode("autoplay"); !errors.Is(err, library.ErrUnavailable) {
		t.Errorf("autoplay err = %v, want ErrUnavailable", err)
	}
}

func TestNew_CorruptDatabase_BotStillStartsWithoutLibrary(t *testing.T) {
	cfg := defaultCfg()
	cfg.Files.MusicFolder = t.TempDir()
	cfg.Bot.MusicDatabasePath = filepath.Join(t.TempDir(), "music.db")
	os.WriteFile(cfg.Bot.MusicDatabasePath, []byte(strings.Repeat("not sqlite ", 50)), 0o644)
	b, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if b.HasLibrary() {
		t.Error("a corrupt database must disable the library, not crash")
	}
}

func TestStartupMode_AutoplayWithLibrary(t *testing.T) {
	lb := newLibBot(t, 3, func(c *config.Config) { c.Bot.PlaybackMode = "autoplay" })
	if lb.PlaybackMode() != "autoplay" {
		t.Errorf("mode = %q, want autoplay when a library is open", lb.PlaybackMode())
	}
}

func TestStartupMode_AutoplayWithoutLibrary_FallsBack(t *testing.T) {
	cfg := defaultCfg()
	cfg.Bot.PlaybackMode = "autoplay"
	b, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if b.PlaybackMode() != "one-shot" {
		t.Errorf("mode = %q, want one-shot fallback", b.PlaybackMode())
	}
}

func TestShutdown_ClosesLibrary(t *testing.T) {
	lb := newLibBot(t, 1, nil)
	lb.Shutdown()
	if lb.HasLibrary() {
		t.Error("Shutdown must close the library")
	}
	lb.Shutdown() // idempotent
}

// ---------------- Rescan ----------------

func TestRescan_NoLibrary(t *testing.T) {
	b, _ := New(defaultCfg())
	if _, err := b.Rescan(context.Background(), false); !errors.Is(err, library.ErrUnavailable) {
		t.Errorf("err = %v, want ErrUnavailable", err)
	}
}

func TestRescan_UnavailableFolderKeepsRows(t *testing.T) {
	lb := newLibBot(t, 3, nil)
	os.RemoveAll(lb.root)
	_, err := lb.Rescan(context.Background(), false)
	if !errors.Is(err, library.ErrFolderUnavailable) {
		t.Fatalf("err = %v, want ErrFolderUnavailable", err)
	}
	recs, _ := lb.libraryRef().db.Query((&database.Condition{}).AndEqual("type", "file", true))
	if len(recs) != 3 {
		t.Errorf("rows = %d, want 3 (nothing removed)", len(recs))
	}
}

// ---------------- autoplay: entering the mode ----------------

func TestEnterAutoplay_NoLibrary(t *testing.T) {
	b, _ := New(defaultCfg())
	err := b.SetPlaybackMode("autoplay")
	if !errors.Is(err, library.ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if b.PlaybackMode() == "autoplay" {
		t.Error("mode flipped despite the error")
	}
}

func TestEnterAutoplay_EmptyLibrary_ReportsAndKeepsMode(t *testing.T) {
	lb := newLibBot(t, 0, nil)
	lb.SetPlaybackMode("repeat")
	err := lb.SetPlaybackMode("autoplay")
	if !errors.Is(err, library.ErrEmpty) {
		t.Fatalf("err = %v, want ErrEmpty", err)
	}
	if lb.PlaybackMode() != "repeat" {
		t.Errorf("mode = %q, want unchanged repeat", lb.PlaybackMode())
	}
}

func TestEnterAutoplay_OnlyStaleRows_IsEmpty(t *testing.T) {
	lb := newLibBot(t, 0, nil)
	lb.addTrack(t, "ghost.mp3", false) // row exists, file does not
	if err := lb.SetPlaybackMode("autoplay"); !errors.Is(err, library.ErrEmpty) {
		t.Errorf("err = %v, want ErrEmpty when no row has a file", err)
	}
}

func TestEnterAutoplay_SetsModeAndActive(t *testing.T) {
	lb := newLibBot(t, 5, nil)
	if err := lb.SetPlaybackMode("autoplay"); err != nil {
		t.Fatal(err)
	}
	if lb.PlaybackMode() != "autoplay" || !lb.autoplayActive.Load() {
		t.Errorf("mode=%q active=%v", lb.PlaybackMode(), lb.autoplayActive.Load())
	}
}

func TestLeavingAutoplay_DeactivatesIt(t *testing.T) {
	lb := newLibBot(t, 5, nil)
	lb.SetPlaybackMode("autoplay")
	lb.SetPlaybackMode("repeat")
	if lb.autoplayActive.Load() {
		t.Error("switching to another mode must deactivate autoplay")
	}
	lb.queue.Clear()
	lb.autoplayTick(lb.queue.Snapshot())
	if lb.queue.Len() != 0 {
		t.Error("autoplayTick refilled although the mode is repeat")
	}
}

// ---------------- autoplay: refill ----------------

func TestAutoplayRefill_FillsLengthTracks_AllRealFiles(t *testing.T) {
	lb := newLibBot(t, 10, nil) // AutoplayLength = 3
	ok, err := lb.autoplayRefill(lb.queue.Snapshot())
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if lb.queue.Len() != 3 {
		t.Errorf("queue len = %d, want autoplay_length (3)", lb.queue.Len())
	}
	seen := map[string]bool{}
	for _, p := range queuePaths(lb.Bot) {
		if seen[p] {
			t.Errorf("duplicate track %s in one batch", p)
		}
		seen[p] = true
	}
	for _, it := range lb.QueueItems() {
		fi := it.(*file.FileItem)
		if fi.Title == "" || fi.Artist != "Art" || fi.Duration != 120*time.Second {
			t.Errorf("metadata not carried from the DB row: %+v", fi)
		}
	}
}

func TestAutoplayRefill_SmallLibraryGivesFewer(t *testing.T) {
	lb := newLibBot(t, 2, nil)
	lb.autoplayRefill(lb.queue.Snapshot())
	if lb.queue.Len() != 2 {
		t.Errorf("len = %d, want 2 (all that exist)", lb.queue.Len())
	}
}

func TestAutoplayRefill_ReplacesQueueSoItStaysShort(t *testing.T) {
	lb := newLibBot(t, 20, nil)
	lb.queue.SetMode(queue.ModeAutoplay)
	for i := 0; i < 8; i++ {
		lb.queue.Append(&mockItem{fmt.Sprint(i)})
	}
	for lb.queue.Next() {
	}
	if lb.queue.Current() != nil {
		t.Fatal("setup: queue should be idle")
	}
	for round := 0; round < 5; round++ {
		ok, err := lb.autoplayRefill(lb.queue.Snapshot())
		if err != nil || !ok {
			t.Fatalf("round %d: ok=%v err=%v", round, ok, err)
		}
		if lb.queue.Len() != 3 || lb.queue.Index() != 0 {
			t.Fatalf("round %d: len=%d index=%d, want a fresh batch of 3 at index 0", round, lb.queue.Len(), lb.queue.Index())
		}
		for lb.queue.Next() { // play the batch out
		}
	}
	if lb.queue.Len() > 3 {
		t.Errorf("queue grew to %d across refills", lb.queue.Len())
	}
}

func TestAutoplayRefill_KeepsMode(t *testing.T) {
	lb := newLibBot(t, 5, nil)
	lb.SetPlaybackMode("autoplay")
	lb.autoplayRefill(lb.queue.Snapshot())
	if lb.PlaybackMode() != "autoplay" {
		t.Errorf("refill changed the mode to %q", lb.PlaybackMode())
	}
}

func TestAutoplayRefill_StaleSnapshot_DoesNotOverwriteQueuedTrack(t *testing.T) {
	lb := newLibBot(t, 5, nil)
	snap := lb.queue.Snapshot()         // loop sees an idle queue
	user := &mockItem{"queued-by-user"} // ...then the user queues something during the DB query
	lb.Enqueue(user)
	ok, err := lb.autoplayRefill(snap)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("a stale refill must not replace the queue")
	}
	if lb.queue.Len() != 1 || lb.queue.Current() != user {
		t.Errorf("the user's track was lost: len=%d", lb.queue.Len())
	}
}

func TestAutoplayRefill_StaleAfterModeChangeOrControl(t *testing.T) {
	for name, change := range map[string]func(b *Bot){
		"mode change": func(b *Bot) { b.queue.SetMode(queue.ModeRepeat) },
		"clear":       func(b *Bot) { b.Clear() },
		"skip":        func(b *Bot) { b.Skip() },
		"stop":        func(b *Bot) { b.Stop() },
	} {
		t.Run(name, func(t *testing.T) {
			lb := newLibBot(t, 5, nil)
			snap := lb.queue.Snapshot()
			change(lb.Bot)
			if ok, _ := lb.autoplayRefill(snap); ok {
				t.Error("refill went through a changed queue")
			}
		})
	}
}

func TestAutoplayRefill_SkipsRowsWhoseFileIsGone(t *testing.T) {
	lb := newLibBot(t, 0, nil)
	for i := 0; i < 20; i++ {
		lb.addTrack(t, fmt.Sprintf("ghost%02d.mp3", i), false)
	}
	lb.addTrack(t, "real.mp3", true)
	ok, err := lb.autoplayRefill(lb.queue.Snapshot())
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	for _, p := range queuePaths(lb.Bot) {
		if strings.HasPrefix(p, "ghost") {
			t.Errorf("a row whose file is gone was queued: %s", p)
		}
	}
}

func TestAutoplayRefill_IgnoresNonFileRows(t *testing.T) {
	lb := newLibBot(t, 0, nil)
	lib := lb.libraryRef()
	// These rows point at a file that REALLY EXISTS, so Validate() would accept
	// them: only the type = "file" filter keeps them out of autoplay.
	os.WriteFile(filepath.Join(lb.root, "exists.mp3"), []byte("x"), 0o644)
	lib.db.Insert(database.MusicRecord{ID: "u1", Type: "url", Title: "u", Path: "exists.mp3", Metadata: map[string]any{}})
	lib.db.Insert(database.MusicRecord{ID: "r1", Type: "radio", Title: "r", Path: "exists.mp3", Metadata: map[string]any{}})
	_, err := lb.autoplayRefill(lb.queue.Snapshot())
	if !errors.Is(err, library.ErrEmpty) {
		t.Errorf("err = %v, want ErrEmpty: only type=file rows are autoplayed", err)
	}
}

// Regression: a random sample of a few times n rows can contain only stale rows
// even though real tracks exist; that must not be reported as an empty library.
func TestAutoplayRefill_ManyStaleRowsFewRealOnes_AlwaysFindsReal(t *testing.T) {
	for trial := 0; trial < 40; trial++ {
		lb := newLibBot(t, 0, nil)
		for i := 0; i < 60; i++ {
			lb.addTrack(t, fmt.Sprintf("ghost%02d.mp3", i), false)
		}
		lb.addTrack(t, "real.mp3", true)
		items, err := lb.libraryRef().randomTracks(3)
		if err != nil || len(items) != 1 {
			t.Fatalf("trial %d: items=%d err=%v, want the 1 real track", trial, len(items), err)
		}
		if items[0].(*file.FileItem).Path != "real.mp3" {
			t.Fatalf("trial %d: got %s", trial, items[0].(*file.FileItem).Path)
		}
	}
}

func TestAutoplayRefill_StaleRowsDoNotShrinkABatchThatCouldBeFull(t *testing.T) {
	lb := newLibBot(t, 0, nil) // AutoplayLength = 3
	for i := 0; i < 30; i++ {
		lb.addTrack(t, fmt.Sprintf("ghost%02d.mp3", i), false)
	}
	for i := 0; i < 4; i++ {
		lb.addTrack(t, fmt.Sprintf("real%d.mp3", i), true)
	}
	for trial := 0; trial < 20; trial++ {
		items, err := lb.libraryRef().randomTracks(3)
		if err != nil || len(items) != 3 {
			t.Fatalf("trial %d: got %d items (err %v), want a full batch of 3", trial, len(items), err)
		}
	}
}

func TestAutoplayRefill_PathEscapingFolderIsSkipped(t *testing.T) {
	lb := newLibBot(t, 0, nil)
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "secret.mp3"), []byte("x"), 0o644)
	rel := "../" + filepath.Base(outside) + "/secret.mp3"
	lib := lb.libraryRef()
	lib.db.Insert(database.MusicRecord{ID: file.IDFromPath(rel), Type: "file", Title: "evil", Path: rel, Metadata: map[string]any{}})
	_, err := lb.autoplayRefill(lb.queue.Snapshot())
	if !errors.Is(err, library.ErrEmpty) {
		t.Errorf("a path escaping music_folder must never be queued, err = %v", err)
	}
}

func TestAutoplayRefill_NoLibraryAndEmptyErrors(t *testing.T) {
	b, _ := New(defaultCfg())
	if _, err := b.autoplayRefill(b.queue.Snapshot()); !errors.Is(err, library.ErrUnavailable) {
		t.Errorf("err = %v", err)
	}
	lb := newLibBot(t, 0, nil)
	if _, err := lb.autoplayRefill(lb.queue.Snapshot()); !errors.Is(err, library.ErrEmpty) {
		t.Errorf("err = %v", err)
	}
}

func TestAutoplayRefill_NonPositiveLengthUsesDefault(t *testing.T) {
	lb := newLibBot(t, 20, func(c *config.Config) { c.Bot.AutoplayLength = 0 })
	lb.autoplayRefill(lb.queue.Snapshot())
	if lb.queue.Len() != 5 {
		t.Errorf("len = %d, want the default 5", lb.queue.Len())
	}
}

func TestAutoplayRefill_RandomnessAcrossCalls(t *testing.T) {
	lb := newLibBot(t, 30, nil)
	first := map[string]bool{}
	for i := 0; i < 15; i++ {
		lb.queue.Clear()
		lb.autoplayRefill(lb.queue.Snapshot())
		first[queuePaths(lb.Bot)[0]] = true
	}
	if len(first) < 3 {
		t.Errorf("only %d distinct first tracks in 15 refills: selection is not random", len(first))
	}
}

// ---------------- autoplay: tick / failure cap ----------------

func TestAutoplayTick_OnlyActsInAutoplayMode(t *testing.T) {
	lb := newLibBot(t, 5, nil)
	lb.autoplayActive.Store(true)
	lb.autoplayTick(lb.queue.Snapshot()) // mode is one-shot
	if lb.queue.Len() != 0 {
		t.Error("tick refilled outside autoplay mode")
	}
}

func TestAutoplayTick_RefillsIdleQueue(t *testing.T) {
	lb := newLibBot(t, 5, nil)
	lb.SetPlaybackMode("autoplay")
	lb.autoplayTick(lb.queue.Snapshot())
	if lb.queue.Len() != 3 || lb.queue.Current() == nil {
		t.Errorf("len=%d current=%v", lb.queue.Len(), lb.queue.Current())
	}
}

func TestAutoplayTick_FailureCapStopsAutoplayAndStaysInMode(t *testing.T) {
	lb := newLibBot(t, 5, nil) // AutoplayLength = 3 => cap 3
	lb.SetPlaybackMode("autoplay")
	lb.autoFails.Store(3)
	lb.autoplayTick(lb.queue.Snapshot())
	if lb.autoplayActive.Load() {
		t.Error("autoplay must stop after a streak of failures")
	}
	if lb.queue.Len() != 0 {
		t.Error("it must not refill once the cap is hit")
	}
	if lb.PlaybackMode() != "autoplay" {
		t.Errorf("mode = %q: the bot must stay in autoplay mode after the cap", lb.PlaybackMode())
	}
	// Further ticks stay quiet (no refill loop).
	lb.autoplayTick(lb.queue.Snapshot())
	if lb.queue.Len() != 0 {
		t.Error("a stopped autoplay refilled on a later tick")
	}
}

func TestAutoplayTick_BelowCapStillRefills(t *testing.T) {
	lb := newLibBot(t, 5, nil)
	lb.SetPlaybackMode("autoplay")
	lb.autoFails.Store(2)
	lb.autoplayTick(lb.queue.Snapshot())
	if lb.queue.Len() == 0 {
		t.Error("2 failures < cap 3: autoplay must keep going")
	}
}

func TestAutoplay_RestartAfterCapByRunningModeAgain(t *testing.T) {
	lb := newLibBot(t, 5, nil)
	lb.SetPlaybackMode("autoplay")
	lb.autoFails.Store(3)
	lb.autoplayTick(lb.queue.Snapshot()) // cap hit
	if lb.autoplayActive.Load() {
		t.Fatal("setup: autoplay should be stopped")
	}
	if err := lb.SetPlaybackMode("autoplay"); err != nil { // user runs !mode autoplay again
		t.Fatal(err)
	}
	if !lb.autoplayActive.Load() || lb.autoFails.Load() != 0 {
		t.Errorf("active=%v fails=%d: running the mode again must restart autoplay cleanly",
			lb.autoplayActive.Load(), lb.autoFails.Load())
	}
	lb.autoplayTick(lb.queue.Snapshot())
	if lb.queue.Len() == 0 {
		t.Error("restarted autoplay did not refill")
	}
}

func TestAutoplayTick_LibraryGoneStopsWithoutLooping(t *testing.T) {
	lb := newLibBot(t, 5, nil)
	lb.SetPlaybackMode("autoplay")
	lb.closeLibrary()
	lb.autoplayTick(lb.queue.Snapshot())
	if lb.autoplayActive.Load() {
		t.Error("autoplay must stop when the library is unavailable")
	}
}

func TestAutoplayTick_EmptiedLibraryStops(t *testing.T) {
	lb := newLibBot(t, 3, nil)
	lb.SetPlaybackMode("autoplay")
	lb.libraryRef().db.Delete((&database.Condition{}).AndEqual("type", "file", true))
	lb.autoplayTick(lb.queue.Snapshot())
	if lb.autoplayActive.Load() {
		t.Error("an emptied library must stop autoplay instead of spinning")
	}
}

// ---------------- autoplay: failure counting via the track-end path ----------------

func TestTrackFailed_CountsAutoplayFailures_OnlyInAutoplay(t *testing.T) {
	lb := newLibBot(t, 5, nil)
	it := &mockItem{"x"}
	lb.Enqueue(it)
	lb.trackFailed(it, lb.queue.Snapshot(), ": failed — ", errBoom)
	if lb.autoFails.Load() != 0 {
		t.Error("failures outside autoplay must not count toward the autoplay cap")
	}
	lb.SetPlaybackMode("autoplay")
	lb.queue.Clear()
	it2 := &mockItem{"y"}
	lb.Enqueue(it2)
	lb.autoFails.Store(0)
	lb.trackFailed(it2, lb.queue.Snapshot(), ": failed — ", errBoom)
	if lb.autoFails.Load() != 1 {
		t.Errorf("autoFails = %d, want 1", lb.autoFails.Load())
	}
}

func TestOnTrackEnd_PlayedTrackResetsAutoplayFailures(t *testing.T) {
	lb := newLibBot(t, 5, nil)
	lb.SetPlaybackMode("autoplay")
	a := &mockItem{"a"}
	lb.queue.Clear()
	lb.Enqueue(a)
	lb.autoFails.Store(2)
	lb.onTrackEnd(a, lb.queue.Snapshot(), lb.launchVersion.Load(), nil)
	if lb.autoFails.Load() != 0 {
		t.Errorf("autoFails = %d, want 0 after a track played", lb.autoFails.Load())
	}
}

// ---------------- autoplay end to end through the loop ----------------

// With every file unplayable the loop must refill, fail, and stop itself: no
// endless refill loop. The pipeline here has no gumble client, so Launch fails.
func TestLoop_Autoplay_UnplayableLibraryStopsItself(t *testing.T) {
	lb := newLibBot(t, 6, nil)
	cfg := lb.cfg
	lb.mu.Lock()
	lb.audio = audio.New(nil, cfg, lb.log)
	lb.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { lb.loop(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })

	if err := lb.SetPlaybackMode("autoplay"); err != nil {
		t.Fatal(err)
	}
	// The pipeline cannot launch (no client), so every refilled track fails.
	waitFor(t, "autoplay to stop itself", func() bool { return !lb.autoplayActive.Load() })
	if lb.PlaybackMode() != "autoplay" {
		t.Errorf("mode = %q, want autoplay kept", lb.PlaybackMode())
	}
	settled := lb.queue.Version()
	time.Sleep(400 * time.Millisecond)
	if lb.queue.Version() != settled {
		t.Error("the queue kept changing after autoplay stopped: refill loop")
	}
}

func TestLoop_Autoplay_IdleBotStartsOnModeSwitch(t *testing.T) {
	lb := newLibBot(t, 6, nil)
	lb.mu.Lock()
	lb.audio = audio.New(nil, lb.cfg, lb.log)
	lb.mu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { lb.loop(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })

	if lb.queue.Len() != 0 {
		t.Fatal("setup: queue must start empty")
	}
	if err := lb.SetPlaybackMode("autoplay"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the idle queue to be filled", func() bool { return lb.queue.Version() > 1 })
}

func TestLoop_Autoplay_PlayingTrackIsNotInterruptedByModeSwitch(t *testing.T) {
	lb := newLibBot(t, 6, nil)
	cur := &mockItem{"playing"}
	lb.Enqueue(cur)
	lb.SetPlaybackMode("autoplay")
	if lb.queue.Current() != cur || lb.queue.Len() != 1 {
		t.Errorf("switching to autoplay must not touch a queue that is playing: len=%d", lb.queue.Len())
	}
}

// ---------------- startup scan ----------------

// runFailingConnect calls Run against a closed port: Run does its startup work
// (including the opt-in scan) and then returns the initial-connection error.
func runFailingConnect(t *testing.T, b *Bot) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	b.cfg.Server.Host = "127.0.0.1"
	b.cfg.Server.Port = 1
	if err := b.Run(ctx); err == nil {
		t.Fatal("Run against a closed port should fail to connect")
	}
}

func logContains(path, want string) bool {
	data, _ := os.ReadFile(path)
	return strings.Contains(string(data), want)
}

func TestRun_StartupScan_RunsWhenEnabled(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "bot.log")
	lb := newLibBot(t, 0, func(c *config.Config) {
		c.Bot.RefreshCacheOnStart = true
		c.Bot.Logfile = logPath
	})
	runFailingConnect(t, lb.Bot)
	waitFor(t, "the startup scan to finish", func() bool { return logContains(logPath, "library scan finished") })
}

func TestRun_StartupScan_OffByDefault(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "bot.log")
	lb := newLibBot(t, 0, func(c *config.Config) { c.Bot.Logfile = logPath })
	runFailingConnect(t, lb.Bot)
	time.Sleep(300 * time.Millisecond)
	if logContains(logPath, "library scan") {
		t.Error("refresh_cache_on_startup defaults to false: no scan may run at startup")
	}
}

func TestRun_StartupScan_NoLibraryIsNoop(t *testing.T) {
	cfg := defaultCfg()
	cfg.Bot.RefreshCacheOnStart = true
	b, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	runFailingConnect(t, b) // must neither panic nor hang
}

func TestRun_StartupScan_FailureIsLoggedNotFatal(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "bot.log")
	lb := newLibBot(t, 2, func(c *config.Config) {
		c.Bot.RefreshCacheOnStart = true
		c.Bot.Logfile = logPath
	})
	os.RemoveAll(lb.root) // the music folder vanished before startup
	runFailingConnect(t, lb.Bot)
	waitFor(t, "the failed startup scan to be logged", func() bool { return logContains(logPath, "startup library scan failed") })
	recs, _ := lb.libraryRef().db.Query((&database.Condition{}).AndEqual("type", "file", true))
	if len(recs) != 2 {
		t.Errorf("rows = %d, want 2: a failed startup scan must not remove anything", len(recs))
	}
}

func TestStartupScan_CancelledContextStopsIt(t *testing.T) {
	lb := newLibBot(t, 0, func(c *config.Config) { c.Bot.RefreshCacheOnStart = true })
	for i := 0; i < 5; i++ {
		os.WriteFile(filepath.Join(lb.root, fmt.Sprintf("t%d.mp3", i)), []byte("x"), 0o644)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	lb.startupScan(ctx) // already cancelled: must wind down without hanging
	waitFor(t, "the scan goroutine to exit", func() bool { return !lb.libraryRef().scanner.Running() })
}

// ---------------- concurrency ----------------

func TestLibrary_ConcurrentRefillAndQueueing(t *testing.T) {
	lb := newLibBot(t, 20, nil)
	lb.SetPlaybackMode("autoplay")
	var wg sync.WaitGroup
	for g := 0; g < 6; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 40; i++ {
				switch (g + i) % 4 {
				case 0:
					lb.autoplayTick(lb.queue.Snapshot())
				case 1:
					lb.Enqueue(&mockItem{fmt.Sprintf("u%d-%d", g, i)})
				case 2:
					lb.queue.Next()
				case 3:
					_ = lb.QueueItems()
				}
			}
		}(g)
	}
	wg.Wait()
	if idx := lb.queue.Index(); idx < 0 || idx > lb.queue.Len() {
		t.Errorf("index %d out of range for len %d", idx, lb.queue.Len())
	}
}

func TestLibrary_RandomTracksAreFileItems(t *testing.T) {
	lb := newLibBot(t, 4, nil)
	items, err := lb.libraryRef().randomTracks(4)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, it := range items {
		fi, ok := it.(*file.FileItem)
		if !ok {
			t.Fatalf("item is %T, want *file.FileItem", it)
		}
		if !strings.HasPrefix(fi.StreamURL(), lb.root) && !strings.Contains(fi.StreamURL(), filepath.Base(lb.root)) {
			// StreamURL is the symlink-resolved path; on macOS /tmp resolves elsewhere.
			t.Logf("stream url %q vs root %q", fi.StreamURL(), lb.root)
		}
		paths = append(paths, fi.Path)
	}
	sort.Strings(paths)
	if len(paths) != 4 {
		t.Errorf("got %v", paths)
	}
}

func TestAutoplayCap_FollowsAutoplayLengthWithDefault(t *testing.T) {
	lb := newLibBot(t, 0, func(c *config.Config) { c.Bot.AutoplayLength = 7 })
	if lb.autoplayCap() != 7 {
		t.Errorf("cap = %d, want 7", lb.autoplayCap())
	}
	lb2 := newLibBot(t, 0, func(c *config.Config) { c.Bot.AutoplayLength = 0 })
	if lb2.autoplayCap() != 5 {
		t.Errorf("cap = %d, want the default 5 for a non-positive length", lb2.autoplayCap())
	}
	lb3 := newLibBot(t, 0, func(c *config.Config) { c.Bot.AutoplayLength = -4 })
	if lb3.autoplayCap() != 5 {
		t.Errorf("cap = %d, want the default 5 for a negative length", lb3.autoplayCap())
	}
}

func trackPaths(ts []command.LibraryTrack) []string {
	var out []string
	for _, t := range ts {
		out = append(out, t.Path)
	}
	return out
}

func TestListLibrary_OrderedByPath_AllRows(t *testing.T) {
	lb := newLibBot(t, 0, nil)
	for _, p := range []string{"b/z.mp3", "a/y.mp3", "a/x.mp3"} {
		lb.addTrack(t, p, false)
	}
	got, err := lb.ListLibrary(nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(trackPaths(got), ",") != "a/x.mp3,a/y.mp3,b/z.mp3" {
		t.Errorf("paths = %v", trackPaths(got))
	}
	if got[0].Artist != "Art" || got[0].Title != "Title a/x.mp3" {
		t.Errorf("track = %+v, want artist and title carried over", got[0])
	}
}

func TestListLibrary_RegexMatchesPathOrTitle(t *testing.T) {
	lb := newLibBot(t, 0, nil)
	lb.addTrack(t, "rock/one.mp3", false)
	lb.addTrack(t, "jazz/two.mp3", false)
	got, _ := lb.ListLibrary(regexp.MustCompile("(?i)ROCK"))
	if strings.Join(trackPaths(got), ",") != "rock/one.mp3" {
		t.Errorf("path match: %v", trackPaths(got))
	}
	got, _ = lb.ListLibrary(regexp.MustCompile("Title jazz"))
	if strings.Join(trackPaths(got), ",") != "jazz/two.mp3" {
		t.Errorf("title match: %v", trackPaths(got))
	}
}

func TestListLibrary_IgnoresNonFileRows(t *testing.T) {
	lb := newLibBot(t, 1, nil)
	lb.libraryRef().db.Insert(database.MusicRecord{ID: "u1", Type: "url", Title: "x", Path: "http://x"})
	got, _ := lb.ListLibrary(nil)
	if len(got) != 1 {
		t.Errorf("got %v, want only the file row", trackPaths(got))
	}
}

func TestSearchLibrary_AllKeywordsMustMatch_Literally(t *testing.T) {
	lb := newLibBot(t, 0, nil)
	lb.addTrack(t, "beethoven/symphony.mp3", false)
	lb.addTrack(t, "beethoven/sonata.mp3", false)
	lb.addTrack(t, "100%/mix.mp3", false)
	lb.addTrack(t, "100x/mix.mp3", false)

	got, _ := lb.SearchLibrary([]string{"BEETHOVEN", "symphony"})
	if strings.Join(trackPaths(got), ",") != "beethoven/symphony.mp3" {
		t.Errorf("AND search: %v", trackPaths(got))
	}
	got, _ = lb.SearchLibrary([]string{"100%"})
	if strings.Join(trackPaths(got), ",") != "100%/mix.mp3" {
		t.Errorf("literal %%: %v", trackPaths(got))
	}
	got, _ = lb.SearchLibrary([]string{"nothing"})
	if len(got) != 0 {
		t.Errorf("no match: %v", trackPaths(got))
	}
}

func TestLibraryBrowse_NoLibrary(t *testing.T) {
	b, _ := New(defaultCfg())
	if _, err := b.ListLibrary(nil); !errors.Is(err, library.ErrUnavailable) {
		t.Errorf("ListLibrary err = %v", err)
	}
	if _, err := b.SearchLibrary([]string{"x"}); !errors.Is(err, library.ErrUnavailable) {
		t.Errorf("SearchLibrary err = %v", err)
	}
}

func TestUpdateTagsAndFindTagged(t *testing.T) {
	lb := newLibBot(t, 0, nil)
	lb.addTrack(t, "a.mp3", false)
	lb.addTrack(t, "b.mp3", false)
	idA := file.IDFromPath("a.mp3")

	if err := lb.UpdateTags(idA, func(tags []string) []string { return append(tags, "Fav") }); err != nil {
		t.Fatal(err)
	}
	got, err := lb.FindTagged([]string{"fav"})
	if err != nil || strings.Join(trackPaths(got), ",") != "a.mp3" {
		t.Fatalf("FindTagged = %v, %v", trackPaths(got), err)
	}
	if len(got[0].Tags) != 1 || got[0].Duration != 120*time.Second {
		t.Errorf("track = %+v, want tags and duration carried over", got[0])
	}
	if err := lb.UpdateTags("nope", func(t []string) []string { return t }); !errors.Is(err, library.ErrNotInLibrary) {
		t.Errorf("err = %v, want ErrNotInLibrary", err)
	}
}

func TestTagging_NoLibrary(t *testing.T) {
	b, _ := New(defaultCfg())
	if _, err := b.FindTagged([]string{"x"}); !errors.Is(err, library.ErrUnavailable) {
		t.Errorf("FindTagged err = %v", err)
	}
	if err := b.UpdateTags("x", nil); !errors.Is(err, library.ErrUnavailable) {
		t.Errorf("UpdateTags err = %v", err)
	}
}

func TestLookupFile_ExactFolderPartial(t *testing.T) {
	lb := newLibBot(t, 0, nil)
	for _, p := range []string{"rock/a.mp3", "rock/b.mp3", "rockabilly/c.mp3", "jazz/Blue Train.mp3"} {
		lb.addTrack(t, p, false)
	}
	cases := []struct {
		arg  string
		kind command.LookupKind
		want string
	}{
		{"jazz/Blue Train.mp3", command.LookupExact, "jazz/Blue Train.mp3"},
		{"rock", command.LookupFolder, "rock/a.mp3,rock/b.mp3"}, // not rockabilly/
		{"rock/", command.LookupFolder, "rock/a.mp3,rock/b.mp3"},
		{"BLUE tr", command.LookupPartial, "jazz/Blue Train.mp3"},
		{"a.mp3", command.LookupPartial, "rock/a.mp3"},
		{"nothing", command.LookupPartial, ""},
	}
	for _, c := range cases {
		got, err := lb.LookupFile(c.arg)
		if err != nil || got.Kind != c.kind || strings.Join(trackPaths(got.Tracks), ",") != c.want {
			t.Errorf("LookupFile(%q) = kind %v %v err %v; want kind %v %q", c.arg, got.Kind, trackPaths(got.Tracks), err, c.kind, c.want)
		}
	}
}

func TestLookupFile_NoLibrary(t *testing.T) {
	b, _ := New(defaultCfg())
	if _, err := b.LookupFile("x"); !errors.Is(err, library.ErrUnavailable) {
		t.Errorf("err = %v", err)
	}
}

func TestLast_JumpsToFinalItem(t *testing.T) {
	b, _ := New(defaultCfg())
	if b.Last() {
		t.Error("empty queue: Last must report false")
	}
	for _, id := range []string{"a", "b", "c"} {
		b.queue.Append(file.New(id+".mp3", "/m"))
	}
	if !b.Last() || b.queue.Index() != 2 {
		t.Errorf("index = %d, want 2", b.queue.Index())
	}
}

func TestRepeatCurrentAndRandomize(t *testing.T) {
	b, _ := New(defaultCfg())
	if _, ok := b.RepeatCurrent(1); ok {
		t.Error("nothing is current")
	}
	b.queue.Append(file.New("a.mp3", "/m"))
	b.queue.Append(file.New("b.mp3", "/m"))
	if _, ok := b.RepeatCurrent(2); !ok || b.queue.Len() != 4 {
		t.Errorf("len = %d, want 4", b.queue.Len())
	}
	b.RandomizeQueue()
	if b.queue.Len() != 4 {
		t.Error("shuffle must keep every item")
	}
}
