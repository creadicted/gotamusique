package command

import (
	"errors"
	"strings"

	"github.com/konradk/gotamusique/internal/queue"
	"layeh.com/gumble/gumble"
)

// handleMode: no argument shows the current mode; one-shot|repeat|random sets it.
// "autoplay" is recognised but refused until the music library exists.
func handleMode(bot BotAPI, user string, msg *gumble.TextMessage, cmd, arg string) {
	cfg := bot.Config()
	arg = strings.TrimSpace(arg)

	if arg == "" {
		cur := bot.PlaybackMode()
		reply(cfg, msg,
			"Playback mode: <b>"+esc(cur)+"</b> (available: "+esc(queue.ModeList())+")",
			"Playback mode: "+cur+" (available: "+queue.ModeList()+")")
		return
	}

	m, err := queue.ParseMode(arg)
	if err != nil {
		var text string
		switch {
		case errors.Is(err, queue.ErrAutoplayUnavailable):
			text = err.Error()
		default:
			text = "Unknown mode. Usage: " + symbol(cfg) + cmd + " [" + strings.ReplaceAll(queue.ModeList(), ", ", "|") + "]"
		}
		reply(cfg, msg, esc(text), text)
		return
	}
	if err := bot.SetPlaybackMode(string(m)); err != nil {
		reply(cfg, msg, esc(err.Error()), err.Error())
		return
	}
	reply(cfg, msg, "Playback mode set to <b>"+esc(string(m))+"</b>", "Playback mode set to "+string(m))
}
