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
	d.Register(a("rb_query"), makeRBQueryHandler(rbcache, rb.Search), false, "Search radio-browser.info; [-n N] results (default 10, max 50)")
	d.Register(a("rb_play"), makeRBPlayHandler(rbcache, rb.ByUUID), false, "Play station by UUID or by index from last !rbquery")
	d.Register(a("stop"), handleStop, false, "Stop playback and reset queue")
	d.Register(a("mute"), handleMute, false, "Silence bot (stream stays connected)")
	d.Register(a("unmute"), handleUnmute, false, "Restore volume after mute")
	d.Register(a("skip"), handleSkip, false, "Skip to next queued station")
	d.Register(a("clear"), handleClear, false, "Stop and empty the queue")
	d.Register(a("queue"), handleQueue, false, "List queued stations")
	d.Register(a("current_music"), handleNP, false, "Show currently playing station")
	d.Register(a("volume"), handleVolume, false, "Get or set volume (0–100)")
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
