package command

import (
	"errors"
	"fmt"
	"time"

	"github.com/konradk/gotamusique/internal/web"
)

// handleWeb: !web — send the caller a private link that logs them into the web UI.
// The link carries a one-time token, so it only ever goes to the caller's private
// chat, never to the channel.
func handleWeb(bot BotAPI, c *Ctx, cmd, arg string) {
	cfg := bot.Config()
	if c.Sender == nil || c.Private == nil {
		c.Send("The web login link can only be requested from Mumble.")
		return
	}

	// A certificate hash survives a rename; without one the name is all there is.
	subject := "name:" + c.Sender.Name
	if c.Sender.Hash != "" {
		subject = c.Sender.Hash
	}
	id := web.Identity{Subject: subject, Name: c.Sender.Name, Roles: []web.Role{web.RoleUser}}
	// The admin list matches names only, and anyone can connect under an
	// unregistered name, so the admin role also needs a registered account.
	if isAdmin(cfg, c.Sender.Name) && c.Sender.IsRegistered() {
		id.Roles = append(id.Roles, web.RoleAdmin)
	}

	link, ttl, err := bot.WebLogin(id)
	switch {
	case errors.Is(err, web.ErrDisabled):
		c.Send("The web interface is not enabled.")
		return
	case err != nil:
		c.Send("Could not create a login link: " + err.Error())
		return
	}

	// The address is also the link text. Some servers strip HTML from chat messages (Kallisti
	// does, href included) and some clients do not render it: the address must still be there,
	// readable and copyable, when the tags are gone.
	htmlText := "Web UI: <a href=\"" + esc(link) + "\">" + esc(link) + "</a>"
	plain := "Web UI: " + link
	if ttl > 0 {
		note := fmt.Sprintf(" (one use, valid for %d minutes)", max(1, int(ttl/time.Minute)))
		htmlText += note
		plain += note
	}
	if err := c.Private(format(cfg.Bot.FormattedReplies, htmlText, plain)); err != nil {
		// Nobody got the link, so it must not stay usable.
		bot.WebRevoke(id)
		c.Send("Could not send you the web link in a private message, try again.")
		return
	}
	c.Send("Sent you the web link in a private message.")
}
