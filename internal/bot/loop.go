package bot

import (
	"context"
	"log/slog"
	"time"

	"github.com/konradk/gotamusique/internal/audio"
)

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
		err := pipeline.Launch(item.StreamURL(), func(endErr error) {
			// Advance the queue only when Play() hasn't already set a new index.
			if b.launchVersion.Load() == launchVer {
				b.queue.Next()
			}
			b.wakeLoop()
		})

		if err != nil {
			b.log.Warn("failed to launch stream", slog.String("url", item.StreamURL()), slog.String("err", err.Error()))
			b.sendChannelMessage(formatAnnouncement(item) + ": failed to start — " + err.Error())
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
