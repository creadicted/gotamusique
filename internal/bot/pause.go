package bot

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
