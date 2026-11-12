// Package status holds the read-only view of the bot's playback state that front
// ends (the web API, later others) render. It is a plain data package so a front
// end can describe the bot with its own narrow interface without importing the
// bot itself.
package status

import (
	"encoding/binary"
	"hash/fnv"
	"math"
	"time"
)

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

// Fingerprint hashes everything a client renders from the snapshot, so it changes
// exactly when the rendered state changes and never otherwise. Version and, while
// audio is playing, Position are left out: the position advances every moment and
// clients interpolate it. When nothing plays the position is static, so then a
// change (a seek while paused) counts.
func (s Snapshot) Fingerprint() uint64 {
	h := fnv.New64a()
	var buf [8]byte
	u64 := func(v uint64) {
		binary.LittleEndian.PutUint64(buf[:], v)
		h.Write(buf[:]) //nolint:errcheck // hash.Hash never fails
	}
	str := func(v string) {
		u64(uint64(len(v))) // length prefix keeps "ab","c" apart from "a","bc"
		h.Write([]byte(v))  //nolint:errcheck
	}
	flag := func(v bool) {
		if v {
			u64(1)
		} else {
			u64(0)
		}
	}

	u64(uint64(len(s.Queue)))
	for _, e := range s.Queue {
		u64(e.ID)
		str(e.Title)
		u64(uint64(e.Duration))
		flag(e.Seekable)
	}
	u64(uint64(s.Index))
	flag(s.Playing)
	str(s.Pause)
	if !s.Playing {
		flag(s.PositionKnown)
		u64(uint64(s.Position))
	}
	u64(math.Float64bits(s.Volume))
	flag(s.Muted)
	str(s.Mode)
	flag(s.Scan.Available)
	flag(s.Scan.Running)
	return h.Sum64()
}
