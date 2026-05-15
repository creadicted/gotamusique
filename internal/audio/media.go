package audio

// MediaItem is implemented by any type the audio pipeline can stream.
// RadioItem satisfies this interface; Phase 2 adds FileItem and URLItem.
type MediaItem interface {
	StreamURL() string
	FormatTitle() string
}
