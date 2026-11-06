package url

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/konradk/gotamusique/internal/config"
)

// ErrIsPlaylist is returned by Validate when the URL points at a playlist
// rather than a single track.
var ErrIsPlaylist = errors.New("that's a playlist, use the playlist command")

// URLItem is a track resolved through yt-dlp. The direct stream URL is resolved
// lazily by Resolve, just before playback, so it cannot expire in the queue.
type URLItem struct {
	URL      string
	Title    string
	Uploader string
	Duration time.Duration // 0 = unknown until resolved
	Thumb    string        // thumbnail URL only; not downloaded

	cfg *config.Config

	mu        sync.Mutex
	streamURL string
}

// New creates an item for rawURL. Call Validate (single tracks) before enqueuing.
func New(rawURL string, cfg *config.Config) *URLItem {
	return &URLItem{URL: rawURL, cfg: cfg}
}

// newFromEntry builds an unresolved item from a flat playlist entry.
func newFromEntry(e YTInfo, cfg *config.Config) *URLItem {
	return &URLItem{
		URL:      e.entryURL(),
		Title:    e.Title,
		Uploader: e.uploader(),
		Duration: e.DurationD(),
		Thumb:    e.Thumbnail,
		cfg:      cfg,
	}
}

func (u *URLItem) apply(info *YTInfo) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if info.Title != "" {
		u.Title = info.Title
	}
	if up := info.uploader(); up != "" {
		u.Uploader = up
	}
	if d := info.DurationD(); d > 0 {
		u.Duration = d
	}
	if info.Thumbnail != "" {
		u.Thumb = info.Thumbnail
	}
}

// checkDuration enforces [bot] max_track_duration (minutes; 0 = no limit).
func (u *URLItem) checkDuration() error {
	u.mu.Lock()
	defer u.mu.Unlock()
	max := u.cfg.Bot.MaxTrackDuration
	if max > 0 && u.Duration > time.Duration(max)*time.Minute {
		return fmt.Errorf("longer than %d min", max)
	}
	return nil
}

// Validate checks the URL, fetches metadata via yt-dlp and enforces the
// duration limit. It rejects playlist URLs with ErrIsPlaylist.
func (u *URLItem) Validate() error {
	if err := CheckURL(u.URL); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return u.ValidateCtx(ctx)
}

// ValidateCtx is Validate with a caller-supplied context.
func (u *URLItem) ValidateCtx(ctx context.Context) error {
	if err := CheckURL(u.URL); err != nil {
		return err
	}
	info, err := GetInfo(ctx, u.URL, u.cfg)
	if err != nil {
		return err
	}
	if info.Type == "playlist" {
		return ErrIsPlaylist
	}
	u.apply(info)
	return u.checkDuration()
}

// Resolve obtains the direct stream URL. For items whose duration is unknown
// (flat playlist entries) it first fetches metadata so the duration limit can
// be enforced.
func (u *URLItem) Resolve(ctx context.Context) error {
	u.mu.Lock()
	unknown := u.Duration == 0
	u.mu.Unlock()
	if unknown {
		info, err := GetInfo(ctx, u.URL, u.cfg)
		if err != nil {
			return err
		}
		u.apply(info)
	}
	if err := u.checkDuration(); err != nil {
		return err
	}
	s, err := ResolveStream(ctx, u.URL, u.cfg)
	if err != nil {
		return err
	}
	u.mu.Lock()
	u.streamURL = s
	u.mu.Unlock()
	return nil
}

// StreamURL returns the resolved direct URL ("" before Resolve).
func (u *URLItem) StreamURL() string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.streamURL
}

// FormatTitle returns a display string such as "Title - Uploader [3:25]".
func (u *URLItem) FormatTitle() string {
	u.mu.Lock()
	defer u.mu.Unlock()
	title := u.Title
	if title == "" {
		title = u.URL
	}
	if u.Uploader != "" {
		title += " - " + u.Uploader
	}
	if u.Duration > 0 {
		s := int(u.Duration.Seconds())
		if s >= 3600 {
			title += fmt.Sprintf(" [%d:%02d:%02d]", s/3600, s%3600/60, s%60)
		} else {
			title += fmt.Sprintf(" [%d:%02d]", s/60, s%60)
		}
	}
	return title
}
