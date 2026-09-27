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

func (f *FileItem) absPath() string {
	return filepath.Clean(filepath.Join(f.musicFolder, f.Path))
}

// Validate checks that the path does not escape musicFolder and that the file
// exists and is readable.
func (f *FileItem) Validate() error {
	abs := f.absPath()
	root := filepath.Clean(f.musicFolder)
	if !strings.HasPrefix(abs+string(filepath.Separator), root+string(filepath.Separator)) {
		return fmt.Errorf("path %q escapes music folder", f.Path)
	}
	fh, err := os.Open(abs)
	if err != nil {
		return fmt.Errorf("cannot open %q: %w", f.Path, err)
	}
	fh.Close()
	return nil
}

// Prepare extracts title, artist, duration, and album art via ffprobe/ffmpeg.
func (f *FileItem) Prepare() error {
	meta, err := ExtractMetadata(f.absPath())
	if err != nil {
		return err
	}
	f.Title = meta.Title
	f.Artist = meta.Artist
	f.Duration = meta.Duration
	f.Thumb = meta.Thumb
	return nil
}

// StreamURL implements audio.MediaItem — returns the absolute path for ffmpeg.
func (f *FileItem) StreamURL() string { return f.absPath() }

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
