package command

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/konradk/gotamusique/internal/web"
	"layeh.com/gumble/gumble"
)

func webSetup(sender *gumble.User) (*mockBot, *Dispatcher, *Ctx, *replies, *replies) {
	bot := defaultBot()
	bot.webLink = "https://music.example.com/login?token=SECRET"
	bot.webTTL = 5 * time.Minute
	d := NewDispatcher([]string{"!"})
	RegisterAll(bot, d)
	var channel, private replies
	c := &Ctx{User: sender.Name, Sender: sender, Out: channel.add, Private: private.add}
	return bot, d, c, &channel, &private
}

func TestWeb_LinkGoesOnlyToThePrivateChat(t *testing.T) {
	bot, d, c, channel, private := webSetup(&gumble.User{Name: "alice", Hash: "cert-hash"})
	d.Run(bot, c, "!web")

	if len(private.list) != 1 || !strings.Contains(private.list[0], "token=SECRET") {
		t.Fatalf("private = %v", private.list)
	}
	for _, line := range channel.list {
		if strings.Contains(line, "SECRET") || strings.Contains(line, "login?") {
			t.Errorf("the token leaked into the channel: %q", line)
		}
	}
	if len(channel.list) != 1 {
		t.Errorf("channel should get one short notice, got %v", channel.list)
	}
	if !strings.Contains(private.list[0], "5 minutes") {
		t.Errorf("expiry not mentioned: %q", private.list[0])
	}
}

func TestWeb_IdentityUsesCertHashElseName(t *testing.T) {
	bot, d, c, _, _ := webSetup(&gumble.User{Name: "alice", Hash: "cert-hash"})
	d.Run(bot, c, "!web")
	if got := bot.webIDs[0]; got.Subject != "cert-hash" || got.Name != "alice" || !got.Has(web.RoleUser) || got.Has(web.RoleAdmin) {
		t.Errorf("identity = %+v", got)
	}

	bot, d, c, _, _ = webSetup(&gumble.User{Name: "bob"})
	d.Run(bot, c, "!web")
	if got := bot.webIDs[0]; got.Subject != "name:bob" {
		t.Errorf("subject without a cert = %q, want name:bob", got.Subject)
	}
}

// "admin" is in the configured admin list. The name alone must not be enough:
// anyone can connect under an unregistered name.
func TestWeb_AdminRoleNeedsARegisteredAccount(t *testing.T) {
	bot, d, c, _, _ := webSetup(&gumble.User{Name: "admin", Hash: "h1"})
	d.Run(bot, c, "!web")
	if bot.webIDs[0].Has(web.RoleAdmin) {
		t.Error("an unregistered user named like an admin got the admin role")
	}

	registered := &gumble.User{Name: "admin", Hash: "h2"}
	registered.UserID = 42
	if !registered.IsRegistered() {
		t.Skip("gumble does not treat UserID 42 as registered")
	}
	bot, d, c, _, _ = webSetup(registered)
	d.Run(bot, c, "!web")
	if !bot.webIDs[0].Has(web.RoleAdmin) {
		t.Error("a registered admin should get the admin role")
	}
}

func TestWeb_RefusesWithoutAPrivateChannel(t *testing.T) {
	bot := defaultBot()
	d := NewDispatcher([]string{"!"})
	RegisterAll(bot, d)
	var channel replies
	d.Run(bot, &Ctx{User: "x", Out: channel.add}, "!web") // no sender, no Private
	if len(bot.webIDs) != 0 {
		t.Error("a link was created although it could not be delivered privately")
	}
	if len(channel.list) != 1 || strings.Contains(channel.list[0], "token") {
		t.Errorf("channel = %v", channel.list)
	}
}

func TestWeb_DisabledAndErrors(t *testing.T) {
	bot, d, c, channel, private := webSetup(&gumble.User{Name: "alice"})
	bot.webErr = web.ErrDisabled
	d.Run(bot, c, "!web")
	if len(private.list) != 0 || len(channel.list) != 1 || !strings.Contains(channel.list[0], "not enabled") {
		t.Errorf("disabled: channel %v private %v", channel.list, private.list)
	}

	bot, d, c, channel, private = webSetup(&gumble.User{Name: "alice"})
	bot.webErr = errors.New("too many pending logins")
	d.Run(bot, c, "!web")
	if len(private.list) != 0 || len(channel.list) != 1 || !strings.Contains(channel.list[0], "too many pending logins") {
		t.Errorf("error: channel %v private %v", channel.list, private.list)
	}
}

func TestWeb_NoLoginRequiredSendsThePlainAddress(t *testing.T) {
	bot, d, c, _, private := webSetup(&gumble.User{Name: "alice"})
	bot.webLink, bot.webTTL = "http://192.168.1.5:8181", 0
	d.Run(bot, c, "!web")
	if len(private.list) != 1 || strings.Contains(private.list[0], "valid for") {
		t.Errorf("private = %v", private.list)
	}
}

func TestChatCtx_PrivateIsSetForAnySender(t *testing.T) {
	msg := makeMsgFrom("alice", "!web")
	if c := chatCtx(msg); c.Private == nil {
		t.Error("chatCtx must provide a private reply path")
	}
}
