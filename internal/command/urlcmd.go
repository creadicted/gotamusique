package command

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/konradk/gotamusique/internal/audio"
	"github.com/konradk/gotamusique/internal/config"
	urlitem "github.com/konradk/gotamusique/internal/media/url"
	"layeh.com/gumble/gumble"
)

const maxConcurrentYT = 3

// ytTimeout bounds one request, including time spent waiting for a free slot.
// A var so tests can shorten it.
var ytTimeout = 30 * time.Second

// ytSem limits concurrent yt-dlp processes so chat spam cannot fork-bomb the host.
var ytSem = make(chan struct{}, maxConcurrentYT)

// ytWG tracks in-flight yt-dlp jobs; tests wait on it.
var ytWG sync.WaitGroup

// runYT runs fn in a goroutine so slow yt-dlp calls never block gumble's event
// goroutine. The ytTimeout deadline starts before waiting for a semaphore slot,
// so a burst of requests can't pile up goroutines or run long after the
// advertised timeout. If no slot frees up in time, busy is called instead of fn.
func runYT(fn func(ctx context.Context), busy func()) {
	ytWG.Add(1)
	go func() {
		defer ytWG.Done()
		ctx, cancel := context.WithTimeout(context.Background(), ytTimeout)
		defer cancel()
		select {
		case ytSem <- struct{}{}:
			defer func() { <-ytSem }()
		case <-ctx.Done():
			busy()
			return
		}
		fn(ctx)
	}()
}

// busyReply tells the user all yt-dlp slots were occupied for the whole timeout.
func busyReply(cfg *config.Config, msg *gumble.TextMessage) func() {
	return func() {
		sendToChannel(msg, "Too many requests in progress, try again in a moment.")
	}
}

func reply(cfg *config.Config, msg *gumble.TextMessage, htmlText, plain string) {
	sendToChannel(msg, format(cfg.Bot.FormattedReplies, htmlText, plain))
}

func ytError(cfg *config.Config, msg *gumble.TextMessage, err error) {
	reply(cfg, msg, "Could not add track: "+esc(err.Error()), "Could not add track: "+err.Error())
}

// handleURL: queue exactly one track resolved through yt-dlp.
func handleURL(bot BotAPI, user string, msg *gumble.TextMessage, cmd, arg string) {
	cfg := bot.Config()
	if arg == "" {
		sendToChannel(msg, "Usage: "+symbol(cfg)+cmd+" <url>")
		return
	}
	if err := urlitem.CheckURL(arg); err != nil {
		ytError(cfg, msg, err)
		return
	}
	sendToChannel(msg, "Resolving…")
	runYT(func(ctx context.Context) {
		item := urlitem.New(arg, cfg)
		if err := item.ValidateCtx(ctx); err != nil {
			if errors.Is(err, urlitem.ErrIsPlaylist) {
				err = fmt.Errorf("that's a playlist, use %splaylist <url>", symbol(cfg))
			}
			ytError(cfg, msg, err)
			return
		}
		bot.Enqueue(item)
		reply(cfg, msg, "Queued: <b>"+esc(item.FormatTitle())+"</b>", "Queued: "+item.FormatTitle())
	}, busyReply(cfg, msg))
}

// handlePlaylist: expand a playlist (capped at max_track_playlist). Entries are
// resolved lazily just before they play.
func handlePlaylist(bot BotAPI, user string, msg *gumble.TextMessage, cmd, arg string) {
	cfg := bot.Config()
	if arg == "" {
		sendToChannel(msg, "Usage: "+symbol(cfg)+cmd+" <playlist url>")
		return
	}
	if err := urlitem.CheckURL(arg); err != nil {
		ytError(cfg, msg, err)
		return
	}
	sendToChannel(msg, "Resolving…")
	runYT(func(ctx context.Context) {
		items, err := urlitem.Playlist(ctx, arg, cfg)
		if err != nil {
			ytError(cfg, msg, err)
			return
		}
		for _, it := range items {
			bot.Enqueue(audio.MediaItem(it))
		}
		text := fmt.Sprintf("Queued %d track(s)", len(items))
		reply(cfg, msg, text, text)
	}, busyReply(cfg, msg))
}
