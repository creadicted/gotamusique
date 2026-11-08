package bot

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/konradk/gotamusique/internal/audio"
	"github.com/konradk/gotamusique/internal/queue"
)

// resolveTimeout bounds a single Resolver.Resolve call.
const resolveTimeout = 30 * time.Second

func (b *Bot) loop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-b.wakeCh:
		case <-time.After(100 * time.Millisecond):
		}

		b.mu.Lock()
		pipeline := b.audio
		b.mu.Unlock()

		if pipeline == nil || pipeline.IsRunning() || b.IsPaused() {
			continue
		}

		// Read the epoch and the queue revision before looking at the item, so a
		// control or a queue change that slips in afterwards is detected: the
		// failure handling below only moves the queue if the revision is unchanged.
		epoch := b.playEpoch.Load()
		snap := b.queue.Snapshot()
		item := snap.Current()
		if item == nil {
			b.fails.reset()
			// Nothing to play. In autoplay mode this is the "queue ran dry" moment.
			b.autoplayTick(snap)
			continue
		}

		launchVer := b.launchVersion.Load()
		// Pause bumps launchVersion after setting its state, so a pause that lands
		// after this point makes onTrackEnd skip queue.Next(); one that landed
		// before it is seen here.
		if b.IsPaused() {
			continue
		}

		// Items such as yt-dlp URLs resolve their stream just before playback.
		var err error
		failMsg := ": failed to start — "
		if _, ok := item.(audio.Resolver); ok {
			var stale bool
			err, stale = b.resolveItem(ctx, item, epoch)
			if stale {
				// A control (skip/clear/stop/play) changed the selection while we
				// were resolving; discard the result and re-evaluate the queue.
				b.wakeLoop()
				continue
			}
			if err != nil {
				failMsg = ": skipped — "
			}
		}
		opts := audio.OptionsFor(item)
		resumedAt, resumed := b.resume.take(item)
		if resumed {
			opts.StartAt = resumedAt
		}
		if err == nil {
			b.mu.Lock()
			b.nowPlaying = item // before Launch, so a pause right after still finds it
			b.mu.Unlock()
			err = pipeline.Launch(item.StreamURL(), opts, func(endErr error) {
				b.onTrackEnd(item, snap, launchVer, endErr)
			})
		}

		if err != nil {
			b.trackFailed(item, snap, failMsg, err)
			continue
		}

		b.saveLastPlayed(item, opts.StartAt)

		// A pause that arrived between the checks above and Launch found no running
		// pipeline to interrupt; stop the track we just started.
		if b.IsPaused() {
			pipeline.Interrupt()
			continue
		}

		if b.cfg.Bot.AnnounceCurrentMusic {
			msg := formatAnnouncement(item)
			if resumed {
				msg += " (resumed at " + clock(resumedAt) + ")"
			}
			b.sendChannelMessage(msg)
		}
	}
}

// onTrackEnd is the pipeline's end-of-track callback. endErr is nil when the
// track played to the end or was interrupted, audio.ErrNoAudio when ffmpeg never
// produced audio, or another error for an unexpected read failure.
func (b *Bot) onTrackEnd(item audio.MediaItem, snap queue.Snapshot, launchVer int64, endErr error) {
	if b.launchVersion.Load() != launchVer {
		// Play() already chose a new index; don't move it.
		b.wakeLoop()
		return
	}
	if errors.Is(endErr, audio.ErrNoAudio) {
		// ffmpeg started but never produced audio (e.g. the file was deleted after
		// it was queued): a failure, not a finished track. Feeding it into the
		// failure guard is what stops an unplayable repeat/random queue.
		b.trackFailed(item, snap, ": failed to start — ", endErr)
		return
	}
	// The track played (or a control interrupted it): the failure streak is over.
	b.fails.reset()
	b.autoFails.Store(0)
	b.queue.Next()
	b.wakeLoop()
}

// trackFailed handles a track that could not be played. It records the failure,
// and then either gives up (every queue entry has failed) or moves on.
//
// The queue is only moved if it is unchanged since snap was taken. Otherwise a
// control or a newly queued item got in first and the loop simply re-evaluates;
// that costs at most one more attempt on the same item, but never skips a newer
// selection or an item appended in the meantime. Safe to call from the loop or
// from the pipeline's end-of-track callback.
func (b *Bot) trackFailed(item audio.MediaItem, snap queue.Snapshot, failMsg string, err error) {
	b.log.Warn("track failed", slog.String("err", err.Error()))
	b.sendChannelMessage(formatAnnouncement(item) + failMsg + err.Error())
	b.fails.fail(item)
	if b.queue.Mode() == queue.ModeAutoplay {
		b.autoFails.Add(1)
	}

	if b.fails.allFailed(snap.Items) {
		// Go idle instead of Stop(): Stop resets the queue to index 0, which would
		// make the loop immediately retry the same failing items forever.
		if b.queue.FinishIfUnchanged(snap.Version) {
			b.sendChannelMessage("too many consecutive failures, stopping")
			b.fails.reset()
		}
		b.wakeLoop()
		return
	}
	b.queue.NextIfUnchanged(snap.Version)
	b.wakeLoop()
}

// resolveItem runs item's Resolver with a timeout. stale is true when a playback
// control invalidated the selection while resolving (or the current item is no
// longer item); the caller must then ignore err and not touch the queue.
func (b *Bot) resolveItem(ctx context.Context, item audio.MediaItem, epoch int64) (err error, stale bool) {
	rctx, cancel := context.WithTimeout(ctx, resolveTimeout)
	defer cancel()

	b.mu.Lock()
	b.resolveCancel = cancel
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		b.resolveCancel = nil
		b.mu.Unlock()
	}()

	err = item.(audio.Resolver).Resolve(rctx)
	if b.playEpoch.Load() != epoch || b.queue.Current() != item {
		return err, true
	}
	return err, false
}

// formatAnnouncement returns the channel message for a starting track.
// Plain text for now; replace this function to switch to HTML formatting.
func formatAnnouncement(item audio.MediaItem) string {
	return "Now playing: " + item.FormatTitle()
}

// sendChannelMessage posts text to the bot's current Mumble channel.
// Safe to call from any goroutine; no-ops when not connected.
func (b *Bot) sendChannelMessage(text string) {
	b.mu.Lock()
	client := b.client
	b.mu.Unlock()
	if client == nil {
		return
	}
	ch := client.Self.Channel
	if ch == nil {
		return
	}
	ch.Send(text, false) //nolint:errcheck
}
