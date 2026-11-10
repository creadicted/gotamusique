package bot

import (
	"fmt"
	"sync"
	"time"

	"github.com/konradk/gotamusique/internal/audio"
)

// Pause state. A pause keeps the queue position (unlike Stop, which rewinds to the
// first item) and stops ffmpeg, so nothing is decoded or downloaded while paused.
// The reason decides who may undo it: auto-resume only undoes pauseAuto, never a
// pause the user asked for.
const (
	pauseNone   int32 = iota // playing or idle
	pauseManual              // !pause
	pauseAuto                // the bot's channel emptied
)

// IsPaused reports whether playback is paused, for any reason.
func (b *Bot) IsPaused() bool { return b.pauseState.Load() != pauseNone }

// Pause halts playback and keeps the queue position. Resume continues from the
// current item. For a live stream "continue" means rejoining it: the source kept
// broadcasting while paused, so there is nothing to seek back to.
// It reports whether the state changed (false when already paused by the user).
func (b *Bot) Pause() bool { return b.pause(pauseManual) }

func (b *Bot) pause(reason int32) bool {
	for {
		cur := b.pauseState.Load()
		if cur == reason || cur == pauseManual {
			return false // already paused; a manual pause is never downgraded to auto
		}
		if b.pauseState.CompareAndSwap(cur, reason) {
			break
		}
	}
	b.captureResume()
	// A stale resolve must not launch, and the interrupted track's onTrackEnd must
	// not advance the queue: bumping launchVersion is the same trick Play uses.
	b.invalidatePlayback()
	b.launchVersion.Add(1)
	b.mu.Lock()
	if b.audio != nil {
		b.audio.Interrupt()
	}
	b.mu.Unlock()
	return true
}

// Resume continues playback after a pause (of any kind). It reports whether the
// bot was paused.
func (b *Bot) Resume() bool {
	if b.pauseState.Swap(pauseNone) == pauseNone {
		return false
	}
	b.wakeLoop()
	return true
}

// occupancyChanged applies the auto-pause rules for a channel that currently holds
// others users besides the bot (-1 = unknown, ignored).
func (b *Bot) occupancyChanged(others int) {
	if !b.cfg.Bot.AutoPauseWhenAlone || others < 0 {
		return
	}
	if others == 0 {
		// Nothing queued means nothing to pause, and no chatter on an idle bot.
		if b.queue.Current() != nil && b.pause(pauseAuto) {
			b.sendChannelMessage("Nobody here, pausing.")
		}
		return
	}
	if b.cfg.Bot.AutoResumeOnJoin && b.pauseState.CompareAndSwap(pauseAuto, pauseNone) {
		b.wakeLoop()
		b.sendChannelMessage("Welcome back, resuming.")
	}
}

// pauseIfAlone is the launch-time counterpart of occupancyChanged: user events
// only fire on a change, so a track that starts while the channel is already
// empty (play or skip from chat, autoplay refill, a reconnect, the bot moved
// into an empty channel) would otherwise play to nobody. It reports whether it
// paused instead; others is the channel count (-1 = unknown, never pauses).
func (b *Bot) pauseIfAlone(others int) bool {
	if !b.cfg.Bot.AutoPauseWhenAlone || others != 0 {
		return false
	}
	b.occupancyChanged(0)
	return b.IsPaused()
}

// onOccupancyChange re-counts the bot's channel on every user event instead of
// decoding the event type, which covers joins, leaves, moves and the bot itself
// being moved.
func (b *Bot) onOccupancyChange() {
	if !b.cfg.Bot.AutoPauseWhenAlone {
		return
	}
	b.occupancyChanged(b.othersInChannel())
}

// othersInChannel counts the users in the bot's channel other than the bot, or -1
// when not connected.
func (b *Bot) othersInChannel() int {
	b.mu.Lock()
	client := b.client
	b.mu.Unlock()
	if client == nil {
		return -1
	}
	n := -1
	client.Do(func() {
		if client.Self == nil || client.Self.Channel == nil {
			return
		}
		n = 0
		for session := range client.Self.Channel.Users {
			if session != client.Self.Session {
				n++
			}
		}
	})
	return n
}

// resumePoint is where a paused local file stopped. The loop consumes it on the
// first launch after Resume, so a later normal replay of the same file (repeat
// mode, say) starts from the beginning.
type resumePoint struct {
	mu   sync.Mutex
	item audio.MediaItem
	at   time.Duration
}

func (r *resumePoint) set(item audio.MediaItem, at time.Duration) {
	r.mu.Lock()
	r.item, r.at = item, at
	r.mu.Unlock()
}

func (r *resumePoint) clear() { r.set(nil, 0) }

// take returns the saved position if it belongs to item, and always clears it.
func (r *resumePoint) take(item audio.MediaItem) (time.Duration, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	it, at := r.item, r.at
	r.item, r.at = nil, 0
	if it == nil || it != item || at <= 0 {
		return 0, false
	}
	return at, true
}

// rememberPosition saves pos for item when item can be restarted part-way in.
// Live streams and other non-seekable items are ignored: resuming them rejoins.
func (b *Bot) rememberPosition(item audio.MediaItem, pos time.Duration) {
	if s, ok := item.(audio.Seekable); ok && s.CanSeek() && pos > 0 {
		b.resume.set(item, pos)
	}
}

// captureResume records how far the current item has played, if it is playing
// right now. It runs before the pipeline is interrupted, so the position excludes
// the short fade-out and Resume picks up slightly before the cut, never after it.
func (b *Bot) captureResume() {
	b.resume.clear()
	b.mu.Lock()
	p, playing := b.audio, b.nowPlaying
	b.mu.Unlock()
	if p == nil || playing == nil || playing != b.queue.Current() {
		return
	}
	if pos, running := p.Position(); running {
		b.rememberPosition(playing, pos)
		b.saveLastPlayed(playing, pos)
	}
}

// endPause clears the pause state and any saved position: a control that picks
// what plays next (play, skip, stop, clear) must not leave a stale seek behind.
func (b *Bot) endPause() {
	b.pauseState.Store(pauseNone)
	b.resume.clear()
}

// clock renders d as m:ss, or h:mm:ss from one hour up.
func clock(d time.Duration) string {
	s := int(d.Round(time.Second) / time.Second)
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s%3600/60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}
