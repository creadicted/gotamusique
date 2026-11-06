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
