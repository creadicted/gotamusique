package bot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/konradk/gotamusique/internal/audio"
	"github.com/konradk/gotamusique/internal/database"
	"github.com/konradk/gotamusique/internal/library"
	"github.com/konradk/gotamusique/internal/media/file"
	"github.com/konradk/gotamusique/internal/queue"
)

// musicLibrary is the bot's handle on the music database and scanner. It is nil
// on Bot when the library is unavailable.
type musicLibrary struct {
	db      database.MusicDB
	scanner *library.Scanner
	folder  string
	scans   sync.WaitGroup // in-flight Rescan calls; close waits for them
}

// openLibrary opens the music database when [files] music_folder is set. Any
// failure is logged and returned as nil so the bot keeps running without a
// library: radio, HLS, URL and !file need none of it.
func openLibrary(b *Bot) *musicLibrary {
	cfg := b.cfg
	if cfg.Files.MusicFolder == "" {
		return nil
	}
	path := cfg.Bot.MusicDatabasePath
	if path == "" {
		path = "music.db"
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			b.log.Error("music library disabled: cannot create database directory",
				slog.String("path", path), slog.String("err", err.Error()))
			return nil
		}
	}
	if err := database.MigrateMusic(path); err != nil {
		b.log.Error("music library disabled: migration failed",
			slog.String("path", path), slog.String("err", err.Error()))
		return nil
	}
	db, err := database.OpenMusic(path)
	if err != nil {
		b.log.Error("music library disabled: cannot open database",
			slog.String("path", path), slog.String("err", err.Error()))
		return nil
	}
	return &musicLibrary{
		db:     db,
		folder: cfg.Files.MusicFolder,
		scanner: library.NewScanner(db, library.Config{
			MusicFolder:    cfg.Files.MusicFolder,
			IgnoredFolders: cfg.Files.IgnoredFolders,
			IgnoredFiles:   cfg.Files.IgnoredFiles,
			Log:            b.log,
		}),
	}
}

func (l *musicLibrary) close() {
	if l != nil && l.db != nil {
		l.db.Close() //nolint:errcheck
	}
}

// libraryRef returns the current library or nil. Safe for concurrent use.
func (b *Bot) libraryRef() *musicLibrary {
	b.libMu.Lock()
	defer b.libMu.Unlock()
	return b.lib
}

// HasLibrary reports whether a music library is open.
func (b *Bot) HasLibrary() bool { return b.libraryRef() != nil }

// Rescan scans the music folder into the library and returns the summary line.
// It blocks until the scan finishes; callers run it in a goroutine.
// force clears the library when the folder holds no audio files. The scan is
// cancelled when the bot shuts down.
func (b *Bot) Rescan(ctx context.Context, force bool) (string, error) {
	b.libMu.Lock()
	lib := b.lib
	if lib != nil {
		lib.scans.Add(1)
	}
	b.libMu.Unlock()
	if lib == nil {
		return "", library.ErrUnavailable
	}
	defer lib.scans.Done()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer context.AfterFunc(b.life, cancel)()

	scan := lib.scanner.Scan
	if force {
		scan = lib.scanner.ScanForce
	}
	res, err := scan(ctx)
	if err != nil {
		return "", err
	}
	b.log.Info("library scan finished", slog.String("result", res.String()),
		slog.Int("unchanged", res.Unchanged), slog.Int("skipped", res.Skipped))
	return res.String(), nil
}

// ListLibrary returns the scanned tracks ordered by path. A non-nil re keeps only
// tracks whose path or title match.
func (b *Bot) ListLibrary(re *regexp.Regexp) ([]library.Track, error) {
	lib := b.libraryRef()
	if lib == nil {
		return nil, library.ErrUnavailable
	}
	recs, err := lib.db.Query((&database.Condition{}).
		AndEqual("type", library.RecordType, true).OrderBy("path", false))
	if err != nil {
		return nil, fmt.Errorf("reading library: %w", err)
	}
	var out []library.Track
	for _, r := range recs {
		if re == nil || re.MatchString(r.Path) || re.MatchString(r.Title) {
			out = append(out, toLibraryTrack(r))
		}
	}
	return out, nil
}

// SearchLibrary returns the tracks whose keywords contain every given keyword
// (case-insensitive), ordered by path.
func (b *Bot) SearchLibrary(keywords []string) ([]library.Track, error) {
	lib := b.libraryRef()
	if lib == nil {
		return nil, library.ErrUnavailable
	}
	cond := (&database.Condition{}).AndEqual("type", library.RecordType, true)
	for _, kw := range keywords {
		cond.AndContains("keywords", kw, false)
	}
	recs, err := lib.db.Query(cond.OrderBy("path", false))
	if err != nil {
		return nil, fmt.Errorf("reading library: %w", err)
	}
	out := make([]library.Track, 0, len(recs))
	for _, r := range recs {
		out = append(out, toLibraryTrack(r))
	}
	return out, nil
}

// FindTagged returns the library files carrying every given tag, ordered by path.
func (b *Bot) FindTagged(tags []string) ([]library.Track, error) {
	lib := b.libraryRef()
	if lib == nil {
		return nil, library.ErrUnavailable
	}
	recs, err := lib.db.QueryByTags(tags)
	if err != nil {
		return nil, fmt.Errorf("reading library: %w", err)
	}
	out := make([]library.Track, 0, len(recs))
	for _, r := range recs {
		if r.Type == library.RecordType {
			out = append(out, toLibraryTrack(r))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// LookupFile resolves a !file argument against the library: an exact path, else
// a folder (every track below it), else a case-insensitive partial path match.
func (b *Bot) LookupFile(arg string) (library.FileLookup, error) {
	lib := b.libraryRef()
	if lib == nil {
		return library.FileLookup{}, library.ErrUnavailable
	}
	arg = strings.TrimSpace(arg)
	// An exact path is one primary-key lookup; only a miss loads the library.
	if r, err := lib.db.QueryByID(file.IDFromPath(arg)); err != nil {
		return library.FileLookup{}, fmt.Errorf("reading library: %w", err)
	} else if r != nil && r.Type == library.RecordType && r.Path == arg {
		return library.FileLookup{Kind: library.LookupExact, Tracks: []library.Track{toLibraryTrack(*r)}}, nil
	}
	recs, err := lib.db.Query((&database.Condition{}).
		AndEqual("type", library.RecordType, true).OrderBy("path", false))
	if err != nil {
		return library.FileLookup{}, fmt.Errorf("reading library: %w", err)
	}
	dir := strings.TrimSuffix(arg, "/") + "/"
	lower := strings.ToLower(arg)
	var folder, partial []library.Track
	for _, r := range recs {
		switch {
		case strings.HasPrefix(r.Path, dir):
			folder = append(folder, toLibraryTrack(r))
		case strings.Contains(strings.ToLower(r.Path), lower):
			partial = append(partial, toLibraryTrack(r))
		}
	}
	if len(folder) > 0 {
		return library.FileLookup{Kind: library.LookupFolder, Tracks: folder}, nil
	}
	return library.FileLookup{Kind: library.LookupPartial, Tracks: partial}, nil
}

// UpdateTags rewrites the tags of the library file row id.
func (b *Bot) UpdateTags(id string, fn func(tags []string) []string) error {
	lib := b.libraryRef()
	if lib == nil {
		return library.ErrUnavailable
	}
	if err := lib.db.UpdateTags(id, fn); err != nil {
		if errors.Is(err, database.ErrNotFound) {
			return library.ErrNotInLibrary
		}
		return err
	}
	return nil
}

func toLibraryTrack(r database.MusicRecord) library.Track {
	t := library.Track{Path: r.Path, Title: r.Title, Tags: r.Tags}
	if a, ok := r.Metadata["artist"].(string); ok {
		t.Artist = a
	}
	if d, ok := r.Metadata["duration"].(float64); ok {
		t.Duration = durationFromSeconds(d)
	}
	return t
}

// startupScan runs the opt-in scan in the background.
func (b *Bot) startupScan(ctx context.Context) {
	if !b.cfg.Bot.RefreshCacheOnStart || b.libraryRef() == nil {
		return
	}
	go func() {
		if _, err := b.Rescan(ctx, false); err != nil {
			b.log.Warn("startup library scan failed", slog.String("err", err.Error()))
		}
	}()
}

// ---- autoplay ----

// autoplayRefill builds the next batch from the library and swaps it in. It
// returns whether the queue was replaced.
//
// It replaces the whole queue (played history is dropped so the queue and !queue
// stay short), but only if the queue is unchanged since snap: a track queued, a
// mode change or a control that landed while the database was being queried is
// never overwritten.
func (b *Bot) autoplayRefill(snap queue.Snapshot) (bool, error) {
	lib := b.libraryRef()
	if lib == nil {
		return false, library.ErrUnavailable
	}
	n := b.cfg.Bot.AutoplayLength
	if n <= 0 {
		n = 5
	}
	items, err := lib.randomTracks(n)
	if err != nil {
		return false, err
	}
	if len(items) == 0 {
		return false, library.ErrEmpty
	}
	return b.queue.ReplaceIfUnchanged(snap.Version, items), nil
}

// randomTracks returns up to n random library tracks as playable items,
// skipping rows whose file no longer exists or escapes the music folder.
//
// Fast path: sample a few times n random rows and keep the valid ones. Stale
// rows are possible between scans, so the sample can come up short; when it does
// the slow path validates every file row in random order, which is guaranteed to
// find a playable track whenever one exists (a sample alone could report an empty
// library while real tracks remain).
func (l *musicLibrary) randomTracks(n int) ([]audio.MediaItem, error) {
	fileRows := (&database.Condition{}).AndEqual("type", library.RecordType, true)
	recs, err := l.db.QueryRandom(n*3, fileRows)
	if err != nil {
		return nil, fmt.Errorf("reading library: %w", err)
	}
	items := l.playable(recs, n)
	// A sample smaller than requested already covered the whole table, so the
	// slow path could not find anything more.
	if len(items) >= n || len(recs) < n*3 {
		return items, nil
	}

	all, err := l.db.Query((&database.Condition{}).AndEqual("type", library.RecordType, true))
	if err != nil {
		return nil, fmt.Errorf("reading library: %w", err)
	}
	rand.Shuffle(len(all), func(i, j int) { all[i], all[j] = all[j], all[i] })
	return l.playable(all, n), nil
}

// playable turns records into file items, in order, skipping rows whose file is
// gone, unreadable or outside the music folder, until n items are collected.
func (l *musicLibrary) playable(recs []database.MusicRecord, n int) []audio.MediaItem {
	var items []audio.MediaItem
	for _, r := range recs {
		if len(items) >= n {
			break
		}
		it := file.New(r.Path, l.folder)
		if it.Validate() != nil {
			continue
		}
		it.Title = r.Title
		if a, ok := r.Metadata["artist"].(string); ok {
			it.Artist = a
		}
		if d, ok := r.Metadata["duration"].(float64); ok {
			it.Duration = durationFromSeconds(d)
		}
		items = append(items, it)
	}
	return items
}

// autoplayTick is called by the loop when the queue is idle (nothing current).
// In autoplay mode it refills the queue. A streak of failures at least as long
// as autoplay_length with no track played in between stops autoplay: with fresh
// untried entries on every refill the "every entry failed" guard can never
// trigger, so an unmounted music folder would otherwise loop forever.
func (b *Bot) autoplayTick(snap queue.Snapshot) {
	if b.queue.Mode() != queue.ModeAutoplay || !b.autoplayActive.Load() {
		return
	}
	if b.autoplayFailures() >= b.autoplayCap() {
		b.autoplayActive.Store(false)
		b.fails.reset()
		b.sendChannelMessage("autoplay stopped: tracks keep failing")
		return
	}
	ok, err := b.autoplayRefill(snap)
	if err != nil {
		b.autoplayActive.Store(false)
		b.sendChannelMessage("autoplay stopped: " + err.Error())
		return
	}
	if ok {
		b.wakeLoop()
	}
}

func (b *Bot) autoplayCap() int {
	if n := b.cfg.Bot.AutoplayLength; n > 0 {
		return n
	}
	return 5
}

func (b *Bot) autoplayFailures() int { return int(b.autoFails.Load()) }

func durationFromSeconds(s float64) time.Duration { return time.Duration(s * float64(time.Second)) }

// closeLibrary closes the database; called on shutdown.
func (b *Bot) closeLibrary() {
	b.libMu.Lock()
	lib := b.lib
	b.lib = nil
	b.libMu.Unlock()
	if lib != nil {
		lib.scans.Wait() // scans see b.life cancelled and return promptly
	}
	lib.close()
}
