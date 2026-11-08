package command

import (
	"regexp"
	"strconv"
	"strings"

	"layeh.com/gumble/gumble"
)

// makeFileHandler: !file <path|folder|part of a path>. With a library it tries an
// exact path, then a folder (queues everything below it), then a case-insensitive
// partial match: one hit is queued, several fill the channel's shortlist. Without a
// library, or when the library has no match, it falls back to playing the path
// straight from music_folder (handleFile), so unscanned files still work.
func makeFileHandler(sl *shortlist) HandlerFunc {
	return func(bot BotAPI, user string, msg *gumble.TextMessage, cmd, arg string) {
		cfg := bot.Config()
		arg = strings.TrimSpace(arg)
		if arg == "" || !bot.HasLibrary() {
			handleFile(bot, user, msg, cmd, arg)
			return
		}
		res, err := bot.LookupFile(arg)
		if err != nil {
			text := libraryErrorText(err)
			reply(cfg, msg, esc(text), text)
			return
		}
		switch {
		case res.Kind == LookupExact:
			enqueueTracks(bot, cfg, msg, res.Tracks, "")
		case res.Kind == LookupFolder:
			enqueueTracks(bot, cfg, msg, res.Tracks, " from "+arg)
		case len(res.Tracks) == 1:
			enqueueTracks(bot, cfg, msg, res.Tracks, "")
		case len(res.Tracks) > 1:
			sendTrackPage(cfg, msg, sl, res.Tracks, 1, "", cmd)
		default:
			handleFile(bot, user, msg, cmd, arg) // not in the library: try the disk
		}
	}
}

// handleFileMatch: !filematch <regex> — queue every library track whose path or title
// matches (case-insensitive), in path order, capped at maxBulkAdd.
func handleFileMatch(bot BotAPI, user string, msg *gumble.TextMessage, cmd, arg string) {
	cfg := bot.Config()
	arg = strings.TrimSpace(arg)
	if arg == "" {
		sendToChannel(msg, "Usage: "+symbol(cfg)+cmd+" <regex>")
		return
	}
	if !bot.HasLibrary() {
		noLibrary(cfg, msg)
		return
	}
	re, err := regexp.Compile("(?i)" + arg)
	if err != nil {
		text := "Invalid regex: " + err.Error()
		reply(cfg, msg, esc(text), text)
		return
	}
	tracks, err := bot.ListLibrary(re)
	if err != nil {
		text := libraryErrorText(err)
		reply(cfg, msg, esc(text), text)
		return
	}
	if len(tracks) == 0 {
		reply(cfg, msg, "No files match.", "No files match.")
		return
	}
	enqueueTracks(bot, cfg, msg, tracks, " matching "+arg)
}

// handleLast: !last — jump to the last queue item and play it.
func handleLast(bot BotAPI, user string, msg *gumble.TextMessage, cmd, arg string) {
	if !bot.Last() {
		sendToChannel(msg, "Queue is empty.")
	}
}

// handleRandom: !random — shuffle the upcoming queue items (the current track keeps playing).
func handleRandom(bot BotAPI, user string, msg *gumble.TextMessage, cmd, arg string) {
	if len(bot.QueueItems()) == 0 {
		sendToChannel(msg, "Queue is empty.")
		return
	}
	bot.RandomizeQueue()
	sendToChannel(msg, "Queue shuffled.")
}

// handleRepeat: !repeat [n] — play the current item n more times (default 1).
func handleRepeat(bot BotAPI, user string, msg *gumble.TextMessage, cmd, arg string) {
	cfg := bot.Config()
	n := 1
	if arg = strings.TrimSpace(arg); arg != "" {
		v, err := strconv.Atoi(arg)
		if err != nil || v < 1 || v > maxBulkAdd {
			sendToChannel(msg, "Usage: "+symbol(cfg)+cmd+" [n] (n: 1-"+strconv.Itoa(maxBulkAdd)+")")
			return
		}
		n = v
	}
	item, ok := bot.RepeatCurrent(n)
	if !ok {
		sendToChannel(msg, "Nothing is playing.")
		return
	}
	text := "Repeating " + item.FormatTitle() + " " + strconv.Itoa(n) + " more time(s)"
	reply(cfg, msg, esc(text), text)
}
