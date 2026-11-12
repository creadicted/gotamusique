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
)

// ytTimeout bounds one request, including time spent waiting for a free slot.
// A var so tests can shorten it.
var ytTimeout = 30 * time.Second

// ytWG tracks in-flight yt-dlp jobs; tests wait on it.
var ytWG sync.WaitGroup

// runYT runs fn in a goroutine so slow yt-dlp calls never block gumble's event
// goroutine. The ytTimeout deadline covers everything fn does, including time
// spent waiting for one of the shared yt-dlp slots (urlitem takes them where it
// launches the process), so a burst of requests can't pile up goroutines or run
// long after the advertised timeout. When no slot frees up in time the call fails
// with urlitem.ErrBusy, which the handlers report like any other error.
func runYT(fn func(ctx context.Context)) {
	ytWG.Add(1)
	go func() {
		defer ytWG.Done()
		ctx, cancel := context.WithTimeout(context.Background(), ytTimeout)
		defer cancel()
		fn(ctx)
	}()
}

func reply(cfg *config.Config, c *Ctx, htmlText, plain string) {
	c.Send(format(cfg.Bot.FormattedReplies, htmlText, plain))
}

func ytError(cfg *config.Config, c *Ctx, err error) {
	reply(cfg, c, "Could not add track: "+esc(err.Error()), "Could not add track: "+err.Error())
}

// handleURL: queue exactly one track resolved through yt-dlp.
func handleURL(bot BotAPI, c *Ctx, cmd, arg string) {
	cfg := bot.Config()
	if arg == "" {
		c.Send("Usage: " + symbol(cfg) + cmd + " <url>")
		return
	}
	if err := urlitem.CheckURL(arg); err != nil {
		ytError(cfg, c, err)
		return
	}
	c.Send("Resolving…")
	runYT(func(ctx context.Context) {
		item := urlitem.New(arg, cfg)
		if err := item.ValidateCtx(ctx); err != nil {
			if errors.Is(err, urlitem.ErrIsPlaylist) {
				err = fmt.Errorf("that's a playlist, use %splaylist <url>", symbol(cfg))
			}
			ytError(cfg, c, err)
			return
		}
		bot.Enqueue(item)
		reply(cfg, c, "Queued: <b>"+esc(item.FormatTitle())+"</b>", "Queued: "+item.FormatTitle())
	})
}

// handlePlaylist: expand a playlist (capped at max_track_playlist). Entries are
// resolved lazily just before they play.
func handlePlaylist(bot BotAPI, c *Ctx, cmd, arg string) {
	cfg := bot.Config()
	if arg == "" {
		c.Send("Usage: " + symbol(cfg) + cmd + " <playlist url>")
		return
	}
	if err := urlitem.CheckURL(arg); err != nil {
		ytError(cfg, c, err)
		return
	}
	c.Send("Resolving…")
	runYT(func(ctx context.Context) {
		items, err := urlitem.Playlist(ctx, arg, cfg)
		if err != nil {
			ytError(cfg, c, err)
			return
		}
		for _, it := range items {
			bot.Enqueue(audio.MediaItem(it))
		}
		text := fmt.Sprintf("Queued %d track(s)", len(items))
		reply(cfg, c, text, text)
	})
}
