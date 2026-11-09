package audio

import (
	"context"
	"time"
)

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

// StreamOptions tunes how ffmpeg opens an item's stream.
type StreamOptions struct {
	// Finite marks a bounded track that must end at EOF, so ffmpeg's
	// reconnect-at-EOF is disabled for it. Live streams (radio, HLS, unknown
	// duration) leave it false and keep reconnect-at-EOF.
	Finite bool
	// Headers are extra HTTP request headers (e.g. Referer, User-Agent) some
	// resolved streams require. Only applied to http(s) URLs.
	Headers map[string]string
	// Cookies are Set-Cookie-formatted lines scoped to the stream URL.
	Cookies []string
	// StartAt starts the stream this far into the item (ffmpeg -ss before -i).
	// Only meaningful for Seekable items; zero starts at the beginning.
	StartAt time.Duration
}

// Seekable is implemented by items that can be restarted part-way in, such as a
// local audio file. Live streams cannot: the source keeps broadcasting, so there
// is no earlier position to return to.
type Seekable interface {
	CanSeek() bool
}

// StreamConfigurer is optionally implemented by items that need non-default
// ffmpeg input options. It is called after Resolve, just before playback.
type StreamConfigurer interface {
	StreamOptions() StreamOptions
}

// OptionsFor returns item's stream options, or the zero value.
func OptionsFor(item MediaItem) StreamOptions {
	if c, ok := item.(StreamConfigurer); ok {
		return c.StreamOptions()
	}
	return StreamOptions{}
}
