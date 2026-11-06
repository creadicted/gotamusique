package bot

import (
	"context"
	"log/slog"
	"time"

	"github.com/konradk/gotamusique/internal/audio"
)

// resolveTimeout bounds a single Resolver.Resolve call.
const resolveTimeout = 30 * time.Second

func (b *Bot) loop(ctx context.Context) {
	consecutiveFails := 0

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

		if pipeline == nil || pipeline.IsRunning() {
			continue
		}

		epoch := b.playEpoch.Load() // before reading the item, so a control in between is detected
		item := b.queue.Current()
		if item == nil {
			consecutiveFails = 0
			continue
		}

		threshold := b.queue.Len()
		if threshold < 3 {
			threshold = 3
		}

		launchVer := b.launchVersion.Load()

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
		if err == nil {
			err = pipeline.Launch(item.StreamURL(), audio.IsFinite(item), func(endErr error) {
				// Advance the queue only when Play() hasn't already set a new index.
				if b.launchVersion.Load() == launchVer {
					b.queue.Next()
				}
				b.wakeLoop()
			})
		}

		if err != nil {
			b.log.Warn("failed to launch stream", slog.String("url", item.StreamURL()), slog.String("err", err.Error()))
			b.sendChannelMessage(formatAnnouncement(item) + failMsg + err.Error())
			consecutiveFails++
			if consecutiveFails >= threshold {
				b.sendChannelMessage("too many consecutive failures, stopping")
				b.Stop()
				consecutiveFails = 0
				continue
			}
			b.queue.Next()
			b.wakeLoop()
			continue
		}

		consecutiveFails = 0
		if b.cfg.Bot.AnnounceCurrentMusic {
			b.sendChannelMessage(formatAnnouncement(item))
		}
	}
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
