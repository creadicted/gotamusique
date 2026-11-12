// Package status holds the read-only view of the bot's playback state that front
// ends (the web API, later others) render. It is a plain data package so a front
// end can describe the bot with its own narrow interface without importing the
// bot itself.
package status

import "time"

// Entry is one queue slot.
type Entry struct {
	ID       uint64 // stable for the lifetime of the entry; use it for edits
	Title    string
	Duration time.Duration // 0 when unknown (live streams)
	Seekable bool
}

// Scan is the library scan state.
type Scan struct {
	Available bool // a music library is open
	Running   bool
}

// Snapshot is one reading of the bot's state.
type Snapshot struct {
	// Version changes whenever anything in the snapshot other than Position
	// changed since the previous reading, and never otherwise. Clients can use it
	// as an ETag. It is opaque: compare for equality only.
	Version uint64

	Queue []Entry
	Index int // position of the current entry; len(Queue) when idle

	Playing  bool          // audio is being sent right now
	Pause    string        // "", "manual" or "auto"
	Position time.Duration // valid only when PositionKnown
	// PositionKnown is false for live streams and when nothing is playing.
	PositionKnown bool

	Volume float64 // target volume in [0, 1]
	Muted  bool
	Mode   string // one-shot, repeat, random or autoplay

	Scan Scan
}
