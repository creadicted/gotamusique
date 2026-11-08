package command

import "github.com/konradk/gotamusique/internal/version"

// handleVersion: !version — report the running bot version.
func handleVersion(bot BotAPI, c *Ctx, cmd, arg string) {
	cfg := bot.Config()
	text := "gotamusique " + version.Get()
	reply(cfg, c, "gotamusique <b>"+esc(version.Get())+"</b>", text)
}
