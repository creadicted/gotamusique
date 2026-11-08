package bot

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/konradk/gotamusique/internal/audio"
	"github.com/konradk/gotamusique/internal/hls"
	fileitem "github.com/konradk/gotamusique/internal/media/file"
	urlitem "github.com/konradk/gotamusique/internal/media/url"
	"github.com/konradk/gotamusique/internal/radio"
)

// lastPlayed is the one item the bot remembers across restarts, so !resume can pick
// it up again. It is deliberately not the whole queue (see milestone 3-10): just
// enough to answer "play what you were playing".
type lastPlayed struct {
	Kind       string `json:"kind"` // radio | hls | file | url
	URL        string `json:"url,omitempty"`
	Path       string `json:"path,omitempty"` // file: relative to music_folder
	Name       string `json:"name,omitempty"`
	Title      string `json:"title,omitempty"`
	Artist     string `json:"artist,omitempty"`
	DurationMS int64  `json:"duration_ms,omitempty"`
	PositionMS int64  `json:"position_ms,omitempty"` // seekable items only
	SavedAt    string `json:"saved_at"`
}

// resumeTailGuard: a saved position this close to the end of a file means the track
// had effectively finished, so resuming starts it from the top instead.
const resumeTailGuard = 3 * time.Second

func describe(item audio.MediaItem, pos time.Duration) (lastPlayed, bool) {
	lp := lastPlayed{PositionMS: pos.Milliseconds(), SavedAt: time.Now().UTC().Format(time.RFC3339)}
	switch it := item.(type) {
	case *radio.RadioItem:
		lp.Kind, lp.URL, lp.Name = "radio", it.URL, it.Name
	case *hls.HLSItem:
		lp.Kind, lp.URL, lp.Name = "hls", it.URL, it.Name
	case *fileitem.FileItem:
		lp.Kind, lp.Path, lp.Title, lp.Artist = "file", it.Path, it.Title, it.Artist
		lp.DurationMS = it.Duration.Milliseconds()
	case *urlitem.URLItem:
		lp.Kind, lp.URL, lp.Title, lp.Artist = "url", it.URL, it.Title, it.Uploader
		lp.DurationMS = it.Duration.Milliseconds()
	default:
		return lp, false
	}
	return lp, true
}

// saveLastPlayed writes item (and, for seekable items, how far it got) to
// last_played_path. Empty path disables it. Failures are logged, never fatal.
func (b *Bot) saveLastPlayed(item audio.MediaItem, pos time.Duration) {
	path := b.cfg.Bot.LastPlayedPath
	if path == "" || item == nil {
		return
	}
	if s, ok := item.(audio.Seekable); !ok || !s.CanSeek() {
		pos = 0
	}
	lp, ok := describe(item, pos)
	if !ok {
		return
	}
	if err := writeLastPlayed(path, lp); err != nil {
		b.log.Warn("could not save last played item", slog.String("path", path), slog.String("err", err.Error()))
	}
}

func writeLastPlayed(path string, lp lastPlayed) error {
	data, err := json.MarshalIndent(lp, "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path) // atomic: a crash never leaves half a file
}

// loadLastPlayed reads the saved item; a missing or unreadable file is "nothing saved".
func loadLastPlayed(path string, log *slog.Logger) *lastPlayed {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Warn("could not read last played item", slog.String("path", path), slog.String("err", err.Error()))
		}
		return nil
	}
	var lp lastPlayed
	if err := json.Unmarshal(data, &lp); err != nil || lp.Kind == "" {
		log.Warn("ignoring unreadable last played file", slog.String("path", path))
		return nil
	}
	return &lp
}

// buildItem recreates the playable item. Radio and HLS items are not re-validated
// here; like any queued item they are checked when they start, and a dead stream is
// reported by the normal failure handling.
func (b *Bot) buildItem(lp lastPlayed) (audio.MediaItem, error) {
	switch lp.Kind {
	case "radio":
		return &radio.RadioItem{URL: lp.URL, Name: lp.Name}, nil
	case "hls":
		return &hls.HLSItem{URL: lp.URL, Name: lp.Name}, nil
	case "file":
		if b.cfg.Files.MusicFolder == "" {
			return nil, errors.New("music_folder is not set")
		}
		it := fileitem.New(lp.Path, b.cfg.Files.MusicFolder)
		if err := it.Validate(); err != nil {
			return nil, err
		}
		it.Title, it.Artist = lp.Title, lp.Artist
		it.Duration = time.Duration(lp.DurationMS) * time.Millisecond
		return it, nil
	case "url":
		it := urlitem.New(lp.URL, b.cfg)
		it.Title, it.Uploader = lp.Title, lp.Artist
		it.Duration = time.Duration(lp.DurationMS) * time.Millisecond
		return it, nil
	}
	return nil, fmt.Errorf("unknown item kind %q", lp.Kind)
}

// RestoreLast re-queues the item remembered from the previous run and starts it,
// from the saved position for local files. It only acts on an idle bot with an empty
// queue, and only once: a second call finds nothing to restore.
func (b *Bot) RestoreLast() (title string, at time.Duration, ok bool) {
	b.mu.Lock()
	lp := b.last
	b.mu.Unlock()
	if lp == nil || b.IsPaused() || b.queue.Len() > 0 {
		return "", 0, false
	}
	item, err := b.buildItem(*lp)
	b.mu.Lock()
	b.last = nil // one shot, also when the item is gone
	b.mu.Unlock()
	if err != nil {
		b.log.Warn("cannot restore last played item", slog.String("err", err.Error()))
		return "", 0, false
	}

	if s, seekable := item.(audio.Seekable); seekable && s.CanSeek() {
		at = time.Duration(lp.PositionMS) * time.Millisecond
		if d := time.Duration(lp.DurationMS) * time.Millisecond; d > 0 && at >= d-resumeTailGuard {
			at = 0
		}
		if at > 0 {
			b.rememberPosition(item, at)
		}
	}
	b.Enqueue(item)
	return item.FormatTitle(), at, true
}

// saveOnShutdown records how far the playing item got, so the next start can resume
// it. A paused bot already saved its position when it paused.
func (b *Bot) saveOnShutdown() {
	b.mu.Lock()
	p, playing := b.audio, b.nowPlaying
	b.mu.Unlock()
	if p == nil || playing == nil || playing != b.queue.Current() {
		return
	}
	if pos, running := p.Position(); running {
		b.saveLastPlayed(playing, pos)
	}
}
