package file

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FileItem represents a local audio file queued for playback.
type FileItem struct {
	ID          string
	Path        string // relative to musicFolder
	Title       string
	Artist      string
	Duration    time.Duration
	Thumb       string // base64 PNG or ""
	Tags        []string
	musicFolder string
	resolvedAbs string // real absolute path set by Validate (symlinks resolved)
}

// New creates a FileItem for relPath under musicFolder.
// Call Validate then Prepare before enqueuing.
func New(relPath, musicFolder string) *FileItem {
	return &FileItem{
		ID:          IDFromPath(relPath),
		Path:        relPath,
		musicFolder: musicFolder,
	}
}

// effectivePath returns the resolved absolute path after Validate, or the best
// lexical guess before it is called.
func (f *FileItem) effectivePath() string {
	if f.resolvedAbs != "" {
		return f.resolvedAbs
	}
	return filepath.Clean(filepath.Join(f.musicFolder, f.Path))
}

// Validate checks that the path does not escape musicFolder (including via
// symlinks) and that the file exists and is readable.
func (f *FileItem) Validate() error {
	// Make the root absolute so the prefix check is reliable even when the
	// process working directory changes.
	absRoot, err := filepath.Abs(f.musicFolder)
	if err != nil {
		return fmt.Errorf("invalid music_folder: %w", err)
	}

	// Lexical containment check — catches ../ traversal before any stat.
	candidate := filepath.Clean(filepath.Join(absRoot, f.Path))
	if !strings.HasPrefix(candidate+string(filepath.Separator), absRoot+string(filepath.Separator)) {
		return fmt.Errorf("path %q escapes music folder", f.Path)
	}

	// Open the file first to verify existence and readability.
	fh, err := os.Open(candidate)
	if err != nil {
		return fmt.Errorf("cannot open %q: %w", f.Path, err)
	}
	fh.Close()

	// Resolve symlinks in both root and candidate, then re-check containment.
	// This prevents a symlink inside musicFolder from pointing outside it.
	realRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return fmt.Errorf("music_folder %q: %w", f.musicFolder, err)
	}
	realPath, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return fmt.Errorf("resolving %q: %w", f.Path, err)
	}
	if !strings.HasPrefix(realPath+string(filepath.Separator), realRoot+string(filepath.Separator)) {
		return fmt.Errorf("path %q escapes music folder via symlink", f.Path)
	}

	f.resolvedAbs = realPath
	return nil
}

// Prepare extracts title, artist, duration, and album art via ffprobe/ffmpeg.
func (f *FileItem) Prepare() error {
	meta, err := ExtractMetadata(f.effectivePath())
	if err != nil {
		return err
	}
	f.Title = meta.Title
	f.Artist = meta.Artist
	f.Duration = meta.Duration
	f.Thumb = meta.Thumb
	return nil
}

// StreamURL implements audio.MediaItem — returns the symlink-resolved absolute
// path for ffmpeg.
func (f *FileItem) StreamURL() string { return f.effectivePath() }

// FormatTitle implements audio.MediaItem.
// Preference: "Artist - Title" > "Title" > bare filename without extension.
func (f *FileItem) FormatTitle() string {
	if f.Artist != "" && f.Title != "" {
		return f.Artist + " - " + f.Title
	}
	if f.Title != "" {
		return f.Title
	}
	base := filepath.Base(f.Path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}
