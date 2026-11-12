package web

import (
	"context"
	"errors"
	"time"

	"github.com/konradk/gotamusique/internal/queue"
	"github.com/konradk/gotamusique/internal/radio"
	"github.com/konradk/gotamusique/internal/status"
)

// Bot is what the web layer needs from the bot. Errors the handlers translate to
// HTTP statuses are the sentinels below plus queue.ErrNotFound and the
// library.Err* values; anything else is reported as an internal error.
type Bot interface {
	Snapshot() status.Snapshot

	Pause() bool
	Resume() bool
	Stop()
	Skip()
	// Seek restarts the current item pos into it.
	Seek(pos time.Duration) error
	// SetVolume sets the volume in percent and ends a mute.
	SetVolume(pct int)
	Mute()
	Unmute()
	SetPlaybackMode(name string) error

	Clear()
	RandomizeQueue()
	// PlayEntry, RemoveEntry and MoveEntry address queue entries by stable ID
	// and return queue.ErrNotFound when the entry is already gone.
	PlayEntry(id uint64) error
	RemoveEntry(id uint64) error
	MoveEntry(id, afterID uint64) error
	// Add validates src, queues it at pos and returns the new entry. It blocks
	// for the network checks (stream reachable, yt-dlp metadata) until ctx ends.
	Add(ctx context.Context, src Source, pos queue.Position) (Added, error)

	SearchRadio(ctx context.Context, query string, limit int) ([]radio.Station, error)
	// StartRescan starts an incremental library scan in the background. It
	// returns library.ErrScanRunning or library.ErrUnavailable without starting.
	StartRescan() error
}

// Source describes something to queue. Type selects which other field is read:
//
//	"library": Path, a path from the library listing
//	"radio":   exactly one of Preset (a configured name), URL (a direct stream)
//	           or UUID (a radio-browser.info station)
//	"url":     URL, a single track resolved through yt-dlp
type Source struct {
	Type   string
	Path   string
	Preset string
	URL    string
	UUID   string
}

// Source types.
const (
	SourceLibrary = "library"
	SourceRadio   = "radio"
	SourceURL     = "url"
)

// Added is the queue entry a successful Add created.
type Added struct {
	ID    uint64
	Title string
}

// Errors the bot returns for the handlers to map.
var (
	ErrNothingPlaying = errors.New("nothing is playing")
	ErrNotSeekable    = errors.New("the current item cannot be seeked")
	ErrSeekRange      = errors.New("seek position out of range")
	// ErrBadSource wraps the reason a Source could not be queued (unknown
	// preset, unreachable stream, a playlist, over the length limit...). The
	// message is safe to show to the user.
	ErrBadSource = errors.New("cannot queue that")
	// ErrBusy means every yt-dlp slot stayed occupied until the deadline.
	ErrBusy = errors.New("too many requests in progress, try again in a moment")
)
