package bot

import (
	"errors"
	"fmt"
	"time"

	"github.com/konradk/gotamusique/internal/audio"
	fileitem "github.com/konradk/gotamusique/internal/media/file"
	"github.com/konradk/gotamusique/internal/queue"
	"github.com/konradk/gotamusique/internal/status"
)

var (
	// ErrNothingPlaying is returned by Seek when no item is current.
	ErrNothingPlaying = errors.New("nothing is playing")
	// ErrNotSeekable is returned by Seek for items that cannot be restarted
	// part-way in (live streams).
	ErrNotSeekable = errors.New("the current item cannot be seeked")
	// ErrSeekRange is returned by Seek for a negative position or one at or past
	// the end of an item whose length is known.
	ErrSeekRange = errors.New("seek position out of range")
)

// restartCurrent interrupts whatever is running and wakes the loop so it launches
// the queue's current item (again). Callers have already settled the queue
// position and the pause state.
func (b *Bot) restartCurrent() {
	b.invalidatePlayback()
	// Same trick as startCurrent: the interrupted track's onTrackEnd must not
	// also advance the queue.
	b.launchVersion.Add(1)
	b.mu.Lock()
	if b.audio != nil {
		b.audio.Interrupt()
	}
	b.mu.Unlock()
	b.wakeLoop()
}

// PlayEntry jumps to the queue entry with the given ID and starts it, like Play
// but addressed by ID, so a client acting on a stale view never starts the wrong
// track. It returns queue.ErrNotFound when the entry is already gone.
func (b *Bot) PlayEntry(id uint64) error {
	if _, err := b.queue.JumpToID(id); err != nil {
		return fmt.Errorf("play: %w", err)
	}
	b.startCurrent()
	return nil
}

// RemoveEntry deletes the queue entry with the given ID. Removing a track that is
// not playing leaves playback alone. Removing the playing one behaves like a skip:
// the next entry starts (in repeat and random mode the queue wraps if that was the
// last one). It returns queue.ErrNotFound when the entry is already gone.
func (b *Bot) RemoveEntry(id uint64) error {
	wasCurrent, err := b.queue.RemoveByID(id)
	if err != nil {
		return fmt.Errorf("remove: %w", err)
	}
	if !wasCurrent {
		return nil
	}
	if b.queue.Current() == nil && b.queue.Len() > 0 {
		if m := b.queue.Mode(); m == queue.ModeRepeat || m == queue.ModeRandom {
			_ = b.queue.JumpTo(0)
		}
	}
	b.endPause() // the paused track is gone; what comes next plays
	b.restartCurrent()
	return nil
}

// MoveEntry moves the entry id to sit directly after the entry afterID (0 = the
// front). The playing track keeps playing. It returns queue.ErrNotFound when
// either entry is gone.
func (b *Bot) MoveEntry(id, afterID uint64) error {
	if err := b.queue.MoveAfter(id, afterID); err != nil {
		return fmt.Errorf("move: %w", err)
	}
	return nil
}

// Seek restarts the current item pos into it. While paused it only moves the
// resume point, so the next Resume continues from pos. Only local files can seek;
// live streams return ErrNotSeekable.
func (b *Bot) Seek(pos time.Duration) error {
	item := b.queue.Current()
	if item == nil {
		return ErrNothingPlaying
	}
	if s, ok := item.(audio.Seekable); !ok || !s.CanSeek() {
		return ErrNotSeekable
	}
	if pos < 0 {
		return ErrSeekRange
	}
	if fi, ok := item.(*fileitem.FileItem); ok && fi.Duration > 0 && pos >= fi.Duration {
		return ErrSeekRange
	}
	b.resume.set(item, pos)
	if b.IsPaused() {
		return nil
	}
	b.restartCurrent()
	return nil
}

// snapSig is everything in a Snapshot that counts as "changed" for Version.
// Position is left out on purpose: it moves every moment and clients interpolate.
type snapSig struct {
	queue   uint64
	pause   int32
	muted   bool
	volume  float64
	playing bool
	scan    status.Scan
}

// Snapshot reads the bot's state for a front end. The queue part is one
// consistent copy; the rest (pause, volume, position) is read right after, so a
// control landing in between can make the two disagree for one reading. The next
// reading shows the settled state, and Version changes with it.
func (b *Bot) Snapshot() status.Snapshot {
	q := b.queue.Snapshot()
	cur := q.Current()

	b.mu.Lock()
	p, nowPlaying, muted := b.audio, b.nowPlaying, b.isMuted
	volume := b.cfg.Bot.Volume
	if p != nil {
		volume = p.Volume().TargetVolume
	}
	b.mu.Unlock()

	s := status.Snapshot{
		Queue:  make([]status.Entry, len(q.Items)),
		Index:  q.Index,
		Volume: volume,
		Muted:  muted,
		Mode:   b.PlaybackMode(),
	}
	for i, it := range q.Items {
		e := status.Entry{ID: q.IDs[i], Title: it.FormatTitle()}
		if sk, ok := it.(audio.Seekable); ok {
			e.Seekable = sk.CanSeek()
		}
		if fi, ok := it.(*fileitem.FileItem); ok {
			e.Duration = fi.Duration
		}
		s.Queue[i] = e
	}

	pauseNow := b.pauseState.Load()
	switch pauseNow {
	case pauseManual:
		s.Pause = "manual"
	case pauseAuto:
		s.Pause = "auto"
	}

	if p != nil && cur != nil && cur == nowPlaying {
		if pos, running := p.Position(); running {
			s.Playing = true
			// Live streams have no meaningful position.
			if sk, ok := cur.(audio.Seekable); ok && sk.CanSeek() {
				s.Position, s.PositionKnown = pos, true
			}
		}
	}
	if !s.Playing && s.Pause != "" && cur != nil {
		if at, ok := b.resume.peek(cur); ok {
			s.Position, s.PositionKnown = at, true
		}
	}

	if lib := b.libraryRef(); lib != nil {
		s.Scan = status.Scan{Available: true, Running: lib.scanner.Running()}
	}

	s.Version = b.versionFor(snapSig{
		queue: q.Version, pause: pauseNow, muted: muted,
		volume: volume, playing: s.Playing, scan: s.Scan,
	})
	return s
}

// versionFor returns the version number for sig: unchanged while sig repeats,
// higher whenever it differs from the previous reading.
func (b *Bot) versionFor(sig snapSig) uint64 {
	b.snapMu.Lock()
	defer b.snapMu.Unlock()
	if sig != b.snapSig {
		b.snapSig = sig
		b.snapRev++
	}
	return b.snapRev
}
