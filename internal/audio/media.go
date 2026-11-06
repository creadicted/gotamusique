package audio

import "context"

// MediaItem is implemented by any type the audio pipeline can stream.
// RadioItem satisfies this interface; Phase 2 adds FileItem and URLItem.
type MediaItem interface {
	StreamURL() string
	FormatTitle() string
}

// Resolver is optionally implemented by items that must resolve their stream
// URL just before playback (e.g. yt-dlp URLs, which expire). The play loop calls
// Resolve before StreamURL; an error skips the item.
type Resolver interface {
	Resolve(ctx context.Context) error
}

// Finite is optionally implemented by items with a known, bounded length (e.g.
// a yt-dlp track with a duration). Finite items must end at EOF, so the
// pipeline disables ffmpeg's reconnect-at-EOF for them; live streams (radio,
// HLS, unknown duration) keep it.
type Finite interface {
	IsFinite() bool
}

// IsFinite reports whether item is a finite track.
func IsFinite(item MediaItem) bool {
	f, ok := item.(Finite)
	return ok && f.IsFinite()
}
