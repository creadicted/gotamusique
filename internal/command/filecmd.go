package command

import (
	"regexp"
	"strconv"
	"strings"
)

// makeFileHandler: !file <path|folder|part of a path>. With a library it tries an
// exact path, then a folder (queues everything below it), then a case-insensitive
// partial match: one hit is queued, several fill the channel's shortlist. Without a
// library, or when the library has no match, it falls back to playing the path
// straight from music_folder (handleFile), so unscanned files still work.
func makeFileHandler(sl *shortlist) HandlerFunc {
	return func(bot BotAPI, c *Ctx, cmd, arg string) {
		cfg := bot.Config()
		arg = strings.TrimSpace(arg)
		if arg == "" || !bot.HasLibrary() {
			handleFile(bot, c, cmd, arg)
			return
		}
		res, err := bot.LookupFile(arg)
		if err != nil {
			text := libraryErrorText(err)
			reply(cfg, c, esc(text), text)
			return
		}
		switch {
		case res.Kind == LookupExact:
			enqueueTracks(bot, cfg, c, res.Tracks, "")
		case res.Kind == LookupFolder:
			enqueueTracks(bot, cfg, c, res.Tracks, " from "+arg)
		case len(res.Tracks) == 1:
			enqueueTracks(bot, cfg, c, res.Tracks, "")
		case len(res.Tracks) > 1:
			sendTrackPage(cfg, c, sl, res.Tracks, 1, "", cmd)
		default:
			handleFile(bot, c, cmd, arg) // not in the library: try the disk
		}
	}
}

// handleFileMatch: !filematch <regex> — queue every library track whose path or title
// matches (case-insensitive), in path order, capped at maxBulkAdd.
func handleFileMatch(bot BotAPI, c *Ctx, cmd, arg string) {
	cfg := bot.Config()
	arg = strings.TrimSpace(arg)
	if arg == "" {
		c.Send("Usage: " + symbol(cfg) + cmd + " <regex>")
		return
	}
	if !bot.HasLibrary() {
		noLibrary(cfg, c)
		return
	}
	re, err := regexp.Compile("(?i)" + arg)
	if err != nil {
		text := "Invalid regex: " + err.Error()
		reply(cfg, c, esc(text), text)
		return
	}
	tracks, err := bot.ListLibrary(re)
	if err != nil {
		text := libraryErrorText(err)
		reply(cfg, c, esc(text), text)
		return
	}
	if len(tracks) == 0 {
		reply(cfg, c, "No files match.", "No files match.")
		return
	}
	enqueueTracks(bot, cfg, c, tracks, " matching "+arg)
}

// handleLast: !last — jump to the last queue item and play it.
func handleLast(bot BotAPI, c *Ctx, cmd, arg string) {
	if !bot.Last() {
		c.Send("Queue is empty.")
	}
}

// handleRandom: !random — shuffle the upcoming queue items (the current track keeps playing).
func handleRandom(bot BotAPI, c *Ctx, cmd, arg string) {
	if len(bot.QueueItems()) == 0 {
		c.Send("Queue is empty.")
		return
	}
	bot.RandomizeQueue()
	c.Send("Queue shuffled.")
}

// handleRepeat: !repeat [n] — play the current item n more times (default 1).
func handleRepeat(bot BotAPI, c *Ctx, cmd, arg string) {
	cfg := bot.Config()
	n := 1
	if arg = strings.TrimSpace(arg); arg != "" {
		v, err := strconv.Atoi(arg)
		if err != nil || v < 1 || v > maxBulkAdd {
			c.Send("Usage: " + symbol(cfg) + cmd + " [n] (n: 1-" + strconv.Itoa(maxBulkAdd) + ")")
			return
		}
		n = v
	}
	item, ok := bot.RepeatCurrent(n)
	if !ok {
		c.Send("Nothing is playing.")
		return
	}
	text := "Repeating " + item.FormatTitle() + " " + strconv.Itoa(n) + " more time(s)"
	reply(cfg, c, esc(text), text)
}
