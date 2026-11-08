package command

import (
	"github.com/konradk/gotamusique/internal/config"
	"github.com/konradk/gotamusique/internal/radio"
)

// RegisterAll registers all radio commands with d using the aliases configured in cfg.
// It must be called once during bot initialisation, before any messages are dispatched.
func RegisterAll(bot BotAPI, d *Dispatcher) {
	cfg := bot.Config()
	a := func(canonical string) []string { return aliasesFor(cfg, canonical) }

	rbcache := newRBCache()
	rb := radio.NewRadioBrowser()

	d.Register(a("play_radio"), handleRadio, false, "List presets or play by name/URL")
	sl := newShortlist()
	d.Register(a("play_file"), makeFileHandler(sl), false, "Queue a library file or folder: <path|folder|part of a path>")
	d.Register(a("play_file_match"), handleFileMatch, false, "Queue every library track matching a regex (path or title)")
	d.Register(a("last"), handleLast, false, "Jump to the last queued item")
	d.Register(a("random"), handleRandom, false, "Shuffle the upcoming queue items")
	d.Register(a("repeat"), handleRepeat, false, "Play the current item n more times: [n]")
	d.Register(a("play_url"), handleURL, false, "Play a single track from a URL via yt-dlp")
	d.Register(a("play_playlist"), handlePlaylist, false, "Queue a playlist from a URL via yt-dlp")
	d.Register(a("play_hls"), handleHLS, false, "Play an HLS/M3U8 stream by URL")
	d.Register(a("rb_query"), makeRBQueryHandler(rbcache, rb.Search), false, "Search radio-browser.info; [-n N] results (default 10, max 50)")
	d.Register(a("rb_play"), makeRBPlayHandler(rbcache, rb.ByUUID), false, "Play station by UUID or by index from the last search")
	d.Register(a("mode"), handleMode, false, "Get or set playback mode: one-shot, repeat, random, autoplay (autoplay starts playing at once)")
	d.Register(a("list_file"), makeListFileHandler(sl), false, "List scanned library files: [regex] [-p page]")
	d.Register(a("search"), makeSearchHandler(sl), false, "Search the library: <keywords> [-p page]; all keywords must match")
	d.Register(a("find_tagged"), makeFindTaggedHandler(sl), false, "List library tracks with all the given tags: <tag,tag> [-p page]")
	d.Register(a("add_from_shortlist"), makeShortlistHandler(sl), false, "Queue entries of the last list: <n> [n ...] | *")
	d.Register(a("play_tag"), handlePlayTag, false, "Queue every library track with all the given tags")
	d.Register(a("add_tag"), handleAddTag, false, "Tag a queued library file: [n|*] <tag,tag>")
	d.Register(a("remove_tag"), handleUntag, false, "Remove tags from a queued library file: [n|*] <tag,tag|*>")
	d.Register(a("rescan"), handleRescan, false, "Scan music_folder into the library (incremental)")
	d.Register(a("stop"), handleStop, false, "Stop playback and reset queue")
	d.Register(a("mute"), handleMute, false, "Silence bot (stream stays connected)")
	d.Register(a("unmute"), handleUnmute, false, "Restore volume after mute")
	d.Register(a("pause"), handlePause, false, "Pause playback, keeping the queue position")
	d.Register(a("resume"), handleResume, false, "Resume playback after a pause")
	d.Register(a("skip"), handleSkip, false, "Skip to next queued station")
	d.Register(a("clear"), handleClear, false, "Stop and empty the queue")
	d.Register(a("queue"), handleQueue, false, "List queued stations")
	d.Register(a("current_music"), handleNP, false, "Show currently playing station")
	d.Register(a("volume"), handleVolume, false, "Get or set volume (0–100)")
	d.Register(a("version"), handleVersion, false, "Show the bot version")
	d.Register(a("joinme"), handleJoinMe, false, "Move bot to your channel")
	d.Register(a("kill"), handleKill, true, "Disconnect and exit")
	d.Register(a("help"), d.HelpHandler(), false, "List available commands")
}

// aliasesFor returns the configured aliases for canonical, falling back to
// []string{canonical} if none are set so commands always have at least one alias.
func aliasesFor(cfg *config.Config, canonical string) []string {
	if aliases := cfg.Commands.Aliases[canonical]; len(aliases) > 0 {
		return aliases
	}
	return []string{canonical}
}
