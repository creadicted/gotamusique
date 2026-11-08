// Package library scans the music folder into the music database.
//
// The scan is incremental (only new or changed files are probed), never wipes
// the library because the folder is unavailable, and only ever deletes rows of
// type "file".
package library

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/konradk/gotamusique/internal/database"
	"github.com/konradk/gotamusique/internal/media/file"
)

// RecordType is the music-table type of library rows written by the scan.
const RecordType = "file"

// audioExts are the extensions that are probed. Everything else is skipped
// without running ffprobe.
var audioExts = map[string]bool{
	".mp3": true, ".flac": true, ".ogg": true, ".oga": true, ".opus": true,
	".m4a": true, ".aac": true, ".wav": true, ".wma": true, ".ape": true, ".aiff": true,
}

// IsAudioFile reports whether name has a scanned audio extension (case-insensitive).
func IsAudioFile(name string) bool {
	return audioExts[strings.ToLower(filepath.Ext(name))]
}

var (
	// ErrUnavailable means no music library is open (music_folder unset, or the
	// database could not be opened at startup).
	ErrUnavailable = errors.New("library unavailable")
	// ErrEmpty means the library is open but holds no tracks.
	ErrEmpty = errors.New("library is empty, run !rescan")
	// ErrNotInLibrary means the file is not in the library database yet.
	ErrNotInLibrary = errors.New("file is not in the library, run !rescan")
	// ErrScanRunning is returned when a scan is already in progress.
	ErrScanRunning = errors.New("scan already running")
	// ErrFolderUnavailable is returned when the music folder is not a readable directory.
	ErrFolderUnavailable = errors.New("music folder is not a readable directory")
	// ErrNoAudioFound is returned when the walk finds no audio files although the
	// library has file rows; nothing is removed in that case.
	ErrNoAudioFound = errors.New("no audio files found, is the music folder mounted? nothing removed (use rescan force to clear the library)")
)

// Prober extracts tags from one audio file. Production uses file.ExtractTags;
// tests inject a fake so the scan logic can be tested without ffprobe.
type Prober func(absPath string) (file.Metadata, error)

// Config configures a Scanner.
type Config struct {
	MusicFolder    string
	IgnoredFolders []string
	IgnoredFiles   []string
	// Probe overrides the ffprobe-based prober (tests).
	Probe Prober
	Log   *slog.Logger
}

// Result summarises one scan.
type Result struct {
	Added     int // new files inserted
	Updated   int // existing files whose size/mtime changed and were re-probed
	Removed   int // rows deleted because the file is gone
	Unchanged int // files whose size and mtime matched; not probed
	Skipped   int // files that failed probing (logged)
}

func (r Result) String() string {
	return fmt.Sprintf("%d added, %d updated, %d removed", r.Added, r.Updated, r.Removed)
}

// Scanner scans one music folder into one music database. It is safe for
// concurrent use; only one scan runs at a time.
type Scanner struct {
	db      database.MusicDB
	folder  string
	ignDirs map[string]bool
	ignFile map[string]bool
	probe   Prober
	log     *slog.Logger

	mu      sync.Mutex
	running bool
}

// NewScanner returns a Scanner writing to db.
func NewScanner(db database.MusicDB, cfg Config) *Scanner {
	probe := cfg.Probe
	if probe == nil {
		probe = file.ExtractTags
	}
	log := cfg.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Scanner{
		db:      db,
		folder:  cfg.MusicFolder,
		ignDirs: toSet(cfg.IgnoredFolders),
		ignFile: toSet(cfg.IgnoredFiles),
		probe:   probe,
		log:     log,
	}
}

func toSet(names []string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		if n = strings.TrimSpace(n); n != "" {
			m[n] = true
		}
	}
	return m
}

// Running reports whether a scan is in progress.
func (s *Scanner) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// found is one audio file discovered by the walk.
type found struct {
	rel   string
	abs   string // symlink-resolved absolute path
	size  int64
	mtime int64 // unix MILLISECONDS (nanoseconds do not survive the JSON float64 round-trip)
}

// Scan walks the music folder and brings the database in line with it. It
// returns ErrScanRunning without doing anything if another scan is in progress.
func (s *Scanner) Scan(ctx context.Context) (Result, error) { return s.scan(ctx, false) }

// ScanForce is Scan without the empty-walk safety: if the walk finds no audio
// files, every file row is removed. Use it to deliberately clear the library.
func (s *Scanner) ScanForce(ctx context.Context) (Result, error) { return s.scan(ctx, true) }

func (s *Scanner) scan(ctx context.Context, force bool) (Result, error) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return Result{}, ErrScanRunning
	}
	s.running = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	root, err := s.resolveRoot()
	if err != nil {
		return Result{}, err
	}

	files, err := s.walk(ctx, root)
	if err != nil {
		return Result{}, err
	}

	existing, err := s.db.Query((&database.Condition{}).AndEqual("type", RecordType, true))
	if err != nil {
		return Result{}, fmt.Errorf("reading library: %w", err)
	}

	// Safety: an empty walk over a non-empty library almost always means the folder
	// is unmounted or emptied by accident. Remove nothing (unless forced).
	if !force && len(files) == 0 && len(existing) > 0 {
		return Result{}, ErrNoAudioFound
	}

	return s.reconcile(ctx, files, existing)
}

// resolveRoot checks the folder is a readable directory and returns its
// symlink-resolved absolute path.
func (s *Scanner) resolveRoot() (string, error) {
	if s.folder == "" {
		return "", ErrFolderUnavailable
	}
	abs, err := filepath.Abs(s.folder)
	if err != nil {
		return "", ErrFolderUnavailable
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", ErrFolderUnavailable
	}
	info, err := os.Stat(real)
	if err != nil || !info.IsDir() {
		return "", ErrFolderUnavailable
	}
	// Readable? Listing proves it; a directory we cannot list is "unavailable".
	f, err := os.Open(real)
	if err != nil {
		return "", ErrFolderUnavailable
	}
	defer f.Close()
	if _, err := f.ReadDir(1); err != nil && !errors.Is(err, io.EOF) {
		return "", ErrFolderUnavailable // cannot list it (permissions, I/O error)
	}
	return real, nil
}

// walk collects the audio files under root, applying the ignore lists and the
// symlink rules. It does not follow directory symlinks; a file symlink is kept
// only if it resolves inside root.
func (s *Scanner) walk(ctx context.Context, root string) ([]found, error) {
	var out []found
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}
		if err != nil {
			// An unreadable entry must not abort the whole scan; skip it.
			s.log.Warn("scan: cannot read entry", slog.String("path", path), slog.String("err", err.Error()))
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if path == root {
			return nil
		}
		name := d.Name()

		if d.IsDir() {
			if s.ignDirs[name] {
				return fs.SkipDir
			}
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return s.walkSymlink(root, path, name, &out)
		}
		if !d.Type().IsRegular() || s.ignFile[name] || !IsAudioFile(name) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			s.log.Warn("scan: cannot stat", slog.String("path", path), slog.String("err", err.Error()))
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		out = append(out, found{rel: filepath.ToSlash(rel), abs: path, size: info.Size(), mtime: info.ModTime().UnixMilli()})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// walkSymlink handles a symlink entry: directory symlinks are never followed;
// file symlinks count only if their target is a regular audio file inside root.
func (s *Scanner) walkSymlink(root, path, name string, out *[]found) error {
	if s.ignFile[name] || !IsAudioFile(name) {
		return nil
	}
	target, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil // dangling link
	}
	if !within(root, target) {
		s.log.Warn("scan: symlink escapes music folder, skipping", slog.String("path", path))
		return nil
	}
	info, err := os.Stat(target)
	if err != nil || !info.Mode().IsRegular() {
		return nil
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return nil
	}
	*out = append(*out, found{rel: filepath.ToSlash(rel), abs: target, size: info.Size(), mtime: info.ModTime().UnixMilli()})
	return nil
}

// within reports whether target is root or lies beneath it.
func within(root, target string) bool {
	return target == root || strings.HasPrefix(target, root+string(filepath.Separator))
}

// reconcile inserts new files, re-probes changed ones, and removes rows whose
// file is gone. Only rows of type "file" are ever touched.
func (s *Scanner) reconcile(ctx context.Context, files []found, existing []database.MusicRecord) (Result, error) {
	var res Result
	byID := make(map[string]database.MusicRecord, len(existing))
	for _, r := range existing {
		byID[r.ID] = r
	}

	seen := make(map[string]bool, len(files))
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		id := file.IDFromPath(f.rel)
		seen[id] = true

		old, had := byID[id]
		if had && unchanged(old, f) {
			res.Unchanged++
			continue
		}

		meta, err := s.probe(f.abs)
		if err != nil {
			s.log.Warn("scan: ffprobe failed, skipping", slog.String("path", f.rel), slog.String("err", err.Error()))
			res.Skipped++
			// A previously-good row for a now-unreadable file keeps its tags, but
			// its size/mtime are refreshed so it is not re-probed on every scan
			// until the file changes again.
			if had {
				if old.Metadata == nil {
					old.Metadata = map[string]any{}
				}
				old.Metadata["size"] = f.size
				old.Metadata["mtime"] = f.mtime
				if err := s.db.Insert(old); err != nil {
					return res, fmt.Errorf("writing %s: %w", f.rel, err)
				}
			}
			continue
		}
		rec := buildRecord(id, f, meta)
		rec.Tags = old.Tags // user tags survive a re-probe (zero value when the row is new)
		if err := s.db.Insert(rec); err != nil {
			return res, fmt.Errorf("writing %s: %w", f.rel, err)
		}
		if had {
			res.Updated++
		} else {
			res.Added++
		}
	}

	for _, r := range existing {
		if seen[r.ID] {
			continue
		}
		// Keep rows for files that were found but failed probing (they are in
		// `seen`); only rows whose file is truly gone are removed.
		if err := s.db.Delete((&database.Condition{}).
			AndEqual("id", r.ID, true).AndEqual("type", RecordType, true)); err != nil {
			return res, fmt.Errorf("removing %s: %w", r.Path, err)
		}
		res.Removed++
	}
	return res, nil
}

// unchanged reports whether the stored size/mtime match the file on disk.
func unchanged(r database.MusicRecord, f found) bool {
	size, ok1 := asInt64(r.Metadata["size"])
	mtime, ok2 := asInt64(r.Metadata["mtime"])
	return ok1 && ok2 && size == f.size && mtime == f.mtime
}

// asInt64 reads a number back from the metadata JSON. json.Unmarshal into
// map[string]any yields float64, which represents integers exactly only up to
// 2^53 — which is why mtime is stored in milliseconds, not nanoseconds.
func asInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case int:
		return int64(n), true
	case float64: // json.Unmarshal into map[string]any yields float64
		return int64(n), true
	}
	return 0, false
}

// buildRecord assembles the DB row. keywords is "<title> <artist>", matching the
// Python migration; with no title tag the title falls back to the filename and
// the folder path words are added so untagged files stay searchable.
func buildRecord(id string, f found, m file.Metadata) database.MusicRecord {
	title := strings.TrimSpace(m.Title)
	artist := strings.TrimSpace(m.Artist)

	keywords := title
	if artist != "" {
		keywords = strings.TrimSpace(keywords + " " + artist)
	}
	if title == "" {
		base := filepath.Base(f.rel)
		title = strings.TrimSuffix(base, filepath.Ext(base))
		keywords = strings.Join(pathWords(f.rel), " ")
		if artist != "" {
			keywords += " " + artist
		}
	}

	meta := map[string]any{
		"duration": m.Duration.Seconds(),
		"size":     f.size,
		"mtime":    f.mtime,
	}
	if artist != "" {
		meta["artist"] = artist
	}
	return database.MusicRecord{
		ID:       id,
		Type:     RecordType,
		Title:    title,
		Keywords: keywords,
		Path:     f.rel,
		Metadata: meta,
	}
}

// pathWords returns the folder names and the filename stem of a relative path,
// de-duplicated, for use as search keywords.
func pathWords(rel string) []string {
	parts := strings.Split(rel, "/")
	parts[len(parts)-1] = strings.TrimSuffix(parts[len(parts)-1], filepath.Ext(parts[len(parts)-1]))
	var out []string
	for _, p := range parts {
		if p != "" && !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return out
}
