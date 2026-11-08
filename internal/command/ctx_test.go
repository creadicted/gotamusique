package command

import (
	"strings"
	"testing"

	"layeh.com/gumble/gumble"
)

func TestCtx_SendWithoutOutIsSafe(t *testing.T) {
	var c *Ctx
	c.Send("x") // nil context
	(&Ctx{}).Send("x")
	if (&Ctx{}).JoinTarget() != nil || c.JoinTarget() != nil {
		t.Error("no sender means no join target")
	}
}

func TestChatCtx_FromMessage(t *testing.T) {
	msg := makeMsgFrom("alice", "!x")
	msg.Channels[0].ID = 7
	c := chatCtx(msg)
	if c.User != "alice" || c.ChannelID != 7 || c.Out == nil {
		t.Errorf("ctx = %+v", c)
	}
	joinCh := &gumble.Channel{}
	msg.Sender.Channel = joinCh
	if chatCtx(msg).JoinTarget() != joinCh {
		t.Error("JoinTarget must be the sender's channel")
	}
	msg.Channels = nil
	msg.Sender = nil
	c = chatCtx(msg)
	c.Send("must not panic")
	if c.User != "" || c.ChannelID != 0 {
		t.Errorf("ctx = %+v", c)
	}
}

// Run executes a command line with no Mumble message at all and returns the output
// through the context, which is what a non-chat front end relies on.
func TestRun_CapturesOutputWithoutMumble(t *testing.T) {
	bot := defaultBot()
	d := NewDispatcher([]string{"!"})
	RegisterAll(bot, d)

	var r replies
	d.Run(bot, &Ctx{User: "web", Out: r.add}, "!volume 40")
	if bot.volume != 0.4 {
		t.Errorf("volume = %v, want 0.4", bot.volume)
	}
	if out := r.all(); len(out) != 1 || !strings.Contains(out[0], "40") {
		t.Errorf("output = %q, want the volume confirmation", out)
	}

	r = replies{}
	d.Run(bot, &Ctx{User: "web", Out: r.add}, "!nosuchcommand")
	if out := r.all(); len(out) != 1 || !strings.Contains(out[0], "Unknown command") {
		t.Errorf("output = %q", out)
	}

	r = replies{}
	d.Run(bot, &Ctx{User: "web", Out: r.add}, "!kill")
	if bot.killCalled || len(r.all()) != 1 || !strings.Contains(r.all()[0], "Permission denied") {
		t.Errorf("a non-admin !kill must be refused: killed=%v out=%q", bot.killCalled, r.all())
	}

	r = replies{}
	d.Run(bot, &Ctx{User: "admin", Out: r.add}, "!kill")
	if !bot.killCalled {
		t.Error("admin !kill must run")
	}
}

func TestRun_IgnoresNonCommands(t *testing.T) {
	bot := defaultBot()
	d := NewDispatcher([]string{"!"})
	RegisterAll(bot, d)
	var r replies
	d.Run(bot, &Ctx{User: "web", Out: r.add}, "just chatting")
	if len(r.all()) != 0 {
		t.Errorf("output = %q, want none", r.all())
	}
}

func TestVersionCommand_RepliesWithVersion(t *testing.T) {
	bot := defaultBot()
	d := NewDispatcher([]string{"!"})
	RegisterAll(bot, d)
	var r replies
	d.Run(bot, &Ctx{User: "u", Out: r.add}, "!version")
	out := r.all()
	if len(out) != 1 || !strings.HasPrefix(out[0], "gotamusique ") || len(out[0]) <= len("gotamusique ") {
		t.Errorf("output = %q", out)
	}
}
