package bot

import "fmt"

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
