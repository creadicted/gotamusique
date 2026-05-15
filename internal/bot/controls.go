package bot

import (
	"fmt"
	"math"
	"os"

	"github.com/konradk/gotamusique/internal/audio"
	"github.com/konradk/gotamusique/internal/config"
	"layeh.com/gumble/gumble"
)

// Play interrupts the current track, jumps to the item at index, and wakes
// the loop to start it. Returns an error if index is out of range.
func (b *Bot) Play(index int) error {
	if err := b.queue.JumpTo(index); err != nil {
		return fmt.Errorf("play: %w", err)
	}
	// Increment version so the previous track's onTrackEnd callback does not
	// also advance the queue after we've already set the desired index.
	b.launchVersion.Add(1)
	b.mu.Lock()
	if b.audio != nil {
		b.audio.Interrupt()
	}
	b.mu.Unlock()
	b.wakeLoop()
	return nil
}

// Mute sets volume to 0 via VolumeHelper without killing ffmpeg.
// The previous volume is saved so Unmute can restore it.
func (b *Bot) Mute() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.isMuted {
		return
	}
	b.isMuted = true
	if b.audio != nil {
		b.prevVol = b.audio.Volume().TargetVolume
		b.audio.Volume().SetTargetVolume(0)
	} else {
		b.prevVol = b.cfg.Bot.Volume
	}
}

// Unmute restores the volume saved by Mute.
func (b *Bot) Unmute() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.isMuted {
		return
	}
	b.isMuted = false
	if b.audio != nil {
		b.audio.Volume().SetTargetVolume(b.prevVol)
	}
}

// IsMuted reports whether the bot is currently muted.
func (b *Bot) IsMuted() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.isMuted
}

// Stop interrupts the current track and resets the queue to the beginning
// without clearing the items. Use Clear to also empty the queue.
func (b *Bot) Stop() {
	b.mu.Lock()
	if b.audio != nil {
		b.audio.Interrupt()
	}
	b.mu.Unlock()
	b.queue.Reset()
}

// Skip ends the current track and starts the next one in the queue.
// If the pipeline is not running (already idle), the queue is advanced
// directly so the loop picks up the next item.
func (b *Bot) Skip() {
	b.mu.Lock()
	pipeline := b.audio
	running := pipeline != nil && pipeline.IsRunning()
	b.mu.Unlock()

	if running {
		// onTrackEnd will call queue.Next() and wake the loop.
		pipeline.Interrupt()
	} else {
		b.queue.Next()
		b.wakeLoop()
	}
}

// Clear interrupts playback and empties the queue.
func (b *Bot) Clear() {
	b.mu.Lock()
	if b.audio != nil {
		b.audio.Interrupt()
	}
	b.mu.Unlock()
	b.queue.Clear()
}

// wakeLoop sends a non-blocking signal to the loop goroutine.
func (b *Bot) wakeLoop() {
	select {
	case b.wakeCh <- struct{}{}:
	default:
	}
}

// --- BotAPI implementation ---

// Config returns the bot's configuration.
func (b *Bot) Config() *config.Config { return b.cfg }

// Enqueue appends item to the play queue and wakes the loop.
func (b *Bot) Enqueue(item audio.MediaItem) {
	b.queue.Append(item)
	b.wakeLoop()
}

// QueueItems returns a snapshot of the current queue contents.
func (b *Bot) QueueItems() []audio.MediaItem { return b.queue.Items() }

// QueueCurrentIndex returns the index of the currently playing item.
func (b *Bot) QueueCurrentIndex() int { return b.queue.Index() }

// CurrentItem returns the item at the current queue position, or nil if idle.
func (b *Bot) CurrentItem() audio.MediaItem { return b.queue.Current() }

// TargetVolume returns the current target volume in [0, 1].
// When the pipeline has not started yet, returns cfg.Bot.Volume.
func (b *Bot) TargetVolume() float64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.audio != nil {
		return b.audio.Volume().TargetVolume
	}
	return b.cfg.Bot.Volume
}

// SetVolume converts pct (0–100) to a float and applies it, clamped to MaxVolume.
// Also clears the muted state so the change is immediately audible.
func (b *Bot) SetVolume(pct int) {
	vol := math.Max(0, math.Min(1, float64(pct)/100.0))
	b.mu.Lock()
	defer b.mu.Unlock()
	b.isMuted = false
	if b.audio != nil {
		b.audio.Volume().SetTargetVolume(vol)
		b.cfg.Bot.Volume = b.audio.Volume().TargetVolume
	} else {
		if vol > b.cfg.Bot.MaxVolume {
			vol = b.cfg.Bot.MaxVolume
		}
		b.cfg.Bot.Volume = vol
	}
}

// JoinChannel moves the bot to ch. No-op when not connected or ch is nil.
func (b *Bot) JoinChannel(ch *gumble.Channel) {
	if ch == nil {
		return
	}
	b.mu.Lock()
	client := b.client
	b.mu.Unlock()
	if client != nil {
		client.Self.Move(ch)
	}
}

// Kill shuts down the bot and exits the process.
func (b *Bot) Kill() {
	b.Shutdown()
	os.Exit(0)
}
