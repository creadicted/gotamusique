package command

import (
	"strings"
	"testing"

	"github.com/konradk/gotamusique/internal/audio"
	"github.com/konradk/gotamusique/internal/config"
	"layeh.com/gumble/gumble"
)

// --- mock BotAPI ---

type mockBot struct {
	cfg          *config.Config
	enqueueCalls []audio.MediaItem
	stopCalls    int
	muteCalls    int
	unmuteCalls  int
	skipCalls    int
	clearCalls   int
	muted        bool
	volume       float64
	queueItems   []audio.MediaItem
	queueIdx     int
	currentItem  audio.MediaItem
	killCalled   bool
}

func (m *mockBot) Config() *config.Config         { return m.cfg }
func (m *mockBot) Enqueue(item audio.MediaItem)   { m.enqueueCalls = append(m.enqueueCalls, item) }
func (m *mockBot) Stop()                          { m.stopCalls++ }
func (m *mockBot) Mute()                          { m.muteCalls++; m.muted = true }
func (m *mockBot) Unmute()                        { m.unmuteCalls++; m.muted = false }
func (m *mockBot) IsMuted() bool                  { return m.muted }
func (m *mockBot) Skip()                          { m.skipCalls++ }
func (m *mockBot) Clear()                         { m.clearCalls++ }
func (m *mockBot) QueueItems() []audio.MediaItem  { return m.queueItems }
func (m *mockBot) QueueCurrentIndex() int         { return m.queueIdx }
func (m *mockBot) CurrentItem() audio.MediaItem   { return m.currentItem }
func (m *mockBot) TargetVolume() float64          { return m.volume }
func (m *mockBot) SetVolume(pct int)              { m.volume = float64(pct) / 100.0 }
func (m *mockBot) JoinChannel(ch *gumble.Channel) {}
func (m *mockBot) Kill()                          { m.killCalled = true }

func defaultBot() *mockBot {
	return &mockBot{
		cfg: &config.Config{
			Bot: config.BotConfig{
				Volume:    0.8,
				MaxVolume: 1.0,
				Admin:     []string{"admin"},
			},
			Commands: config.CommandsConfig{
				Symbol:  []string{"!"},
				Aliases: map[string][]string{},
			},
			Radio: map[string]config.RadioPreset{},
		},
		volume: 0.8,
	}
}

// --- helpers to build fake gumble messages ---

func fakeChannel() *gumble.Channel {
	// gumble.Channel is a struct; create a minimal one via zero value.
	// We only need non-nil Channels in TextMessage.
	ch := &gumble.Channel{}
	return ch
}

// makeMsg builds a TextMessage sent in a channel (not a private message).
// The channel's Send method is a no-op here since gumble.Channel.Send is
// exported but calls internal gumble plumbing — we only test dispatch logic.
func makeMsg(text string) *gumble.TextMessage {
	ch := fakeChannel()
	sender := &gumble.User{}
	sender.Name = "testuser"
	return &gumble.TextMessage{
		Sender:   sender,
		Channels: []*gumble.Channel{ch},
		Message:  text,
	}
}

func makeMsgFrom(user, text string) *gumble.TextMessage {
	msg := makeMsg(text)
	msg.Sender.Name = user
	return msg
}

func makeDispatcher(symbols []string) *Dispatcher {
	return NewDispatcher(symbols)
}

// --- Dispatcher tests ---

func TestDispatch_ExactMatch(t *testing.T) {
	d := makeDispatcher([]string{"!"})
	bot := defaultBot()
	called := false
	d.Register([]string{"ping"}, func(b BotAPI, user string, msg *gumble.TextMessage, cmd, arg string) {
		called = true
		if cmd != "ping" {
			t.Errorf("cmd = %q, want %q", cmd, "ping")
		}
	}, false, "test")

	d.Dispatch(bot, makeMsg("!ping"))
	if !called {
		t.Error("handler not called for exact match")
	}
}

func TestDispatch_ExactMatchWithArgument(t *testing.T) {
	d := makeDispatcher([]string{"!"})
	bot := defaultBot()
	var gotArg string
	d.Register([]string{"say"}, func(b BotAPI, user string, msg *gumble.TextMessage, cmd, arg string) {
		gotArg = arg
	}, false, "test")

	d.Dispatch(bot, makeMsg("!say hello world"))
	if gotArg != "hello world" {
		t.Errorf("arg = %q, want %q", gotArg, "hello world")
	}
}

func TestDispatch_PrefixMatch_Single(t *testing.T) {
	d := makeDispatcher([]string{"!"})
	bot := defaultBot()
	called := false
	d.Register([]string{"help"}, func(b BotAPI, user string, msg *gumble.TextMessage, cmd, arg string) {
		called = true
	}, false, "test")

	d.Dispatch(bot, makeMsg("!hel"))
	if !called {
		t.Error("prefix match did not dispatch to help")
	}
}

func TestDispatch_PrefixMatch_Ambiguous(t *testing.T) {
	d := makeDispatcher([]string{"!"})
	bot := defaultBot()
	var replies []string

	d.Register([]string{"stop"}, func(b BotAPI, user string, msg *gumble.TextMessage, cmd, arg string) {
		replies = append(replies, "stop")
	}, false, "test")
	d.Register([]string{"skip"}, func(b BotAPI, user string, msg *gumble.TextMessage, cmd, arg string) {
		replies = append(replies, "skip")
	}, false, "test")

	// Neither handler should be called — dispatcher sends "did you mean".
	d.Dispatch(bot, makeMsg("!s"))
	if len(replies) != 0 {
		t.Errorf("expected no handler called for ambiguous prefix, got %v", replies)
	}
}

func TestDispatch_UnknownCommand(t *testing.T) {
	d := makeDispatcher([]string{"!"})
	bot := defaultBot()
	// Just ensure Dispatch does not panic on unknown command.
	d.Dispatch(bot, makeMsg("!doesnotexist"))
}

func TestDispatch_AdminGuard_Denied(t *testing.T) {
	d := makeDispatcher([]string{"!"})
	bot := defaultBot()
	called := false
	d.Register([]string{"kill"}, func(b BotAPI, user string, msg *gumble.TextMessage, cmd, arg string) {
		called = true
	}, true, "admin only")

	d.Dispatch(bot, makeMsgFrom("notadmin", "!kill"))
	if called {
		t.Error("admin-only handler should not be called for non-admin user")
	}
}

func TestDispatch_AdminGuard_Allowed(t *testing.T) {
	d := makeDispatcher([]string{"!"})
	bot := defaultBot()
	called := false
	d.Register([]string{"kill"}, func(b BotAPI, user string, msg *gumble.TextMessage, cmd, arg string) {
		called = true
	}, true, "admin only")

	d.Dispatch(bot, makeMsgFrom("admin", "!kill"))
	if !called {
		t.Error("admin handler should be called for admin user")
	}
}

func TestDispatch_PrivateMessage_Ignored(t *testing.T) {
	d := makeDispatcher([]string{"!"})
	bot := defaultBot()
	called := false
	d.Register([]string{"ping"}, func(b BotAPI, user string, msg *gumble.TextMessage, cmd, arg string) {
		called = true
	}, false, "test")

	// Private message: no Channels
	msg := makeMsg("!ping")
	msg.Channels = nil
	d.Dispatch(bot, msg)
	if called {
		t.Error("private messages should be silently ignored")
	}
}

func TestDispatch_ServerMessage_Ignored(t *testing.T) {
	d := makeDispatcher([]string{"!"})
	bot := defaultBot()
	called := false
	d.Register([]string{"ping"}, func(b BotAPI, user string, msg *gumble.TextMessage, cmd, arg string) {
		called = true
	}, false, "test")

	msg := makeMsg("!ping")
	msg.Sender = nil
	d.Dispatch(bot, msg)
	if called {
		t.Error("server messages (Sender==nil) should be silently ignored")
	}
}

func TestDispatch_HTMLStripped(t *testing.T) {
	d := makeDispatcher([]string{"!"})
	bot := defaultBot()
	var gotCmd string
	d.Register([]string{"stop"}, func(b BotAPI, user string, msg *gumble.TextMessage, cmd, arg string) {
		gotCmd = cmd
	}, false, "test")

	// Mumble wraps messages in HTML spans/paragraphs.
	d.Dispatch(bot, makeMsg("<p><b>!stop</b></p>"))
	if gotCmd != "stop" {
		t.Errorf("cmd = %q after HTML strip, want %q", gotCmd, "stop")
	}
}

func TestDispatch_HTMLEntitiesDecoded(t *testing.T) {
	d := makeDispatcher([]string{"!"})
	bot := defaultBot()
	var gotArg string
	d.Register([]string{"say"}, func(b BotAPI, user string, msg *gumble.TextMessage, cmd, arg string) {
		gotArg = arg
	}, false, "test")

	d.Dispatch(bot, makeMsg("!say hello &amp; world"))
	if gotArg != "hello & world" {
		t.Errorf("arg = %q, want %q", gotArg, "hello & world")
	}
}

func TestDispatch_MultipleSymbols(t *testing.T) {
	d := makeDispatcher([]string{"!", "！"})
	bot := defaultBot()
	calls := 0
	d.Register([]string{"ping"}, func(b BotAPI, user string, msg *gumble.TextMessage, cmd, arg string) {
		calls++
	}, false, "test")

	d.Dispatch(bot, makeMsg("!ping"))
	d.Dispatch(bot, makeMsg("！ping"))
	if calls != 2 {
		t.Errorf("calls = %d, want 2 (both symbols should trigger dispatch)", calls)
	}
}

func TestDispatch_NoSymbol_Ignored(t *testing.T) {
	d := makeDispatcher([]string{"!"})
	bot := defaultBot()
	called := false
	d.Register([]string{"ping"}, func(b BotAPI, user string, msg *gumble.TextMessage, cmd, arg string) {
		called = true
	}, false, "test")

	d.Dispatch(bot, makeMsg("ping"))
	if called {
		t.Error("message without prefix symbol should be ignored")
	}
}

func TestDispatch_PrefixAdminGuard_Denied(t *testing.T) {
	d := makeDispatcher([]string{"!"})
	bot := defaultBot()
	called := false
	d.Register([]string{"killall"}, func(b BotAPI, user string, msg *gumble.TextMessage, cmd, arg string) {
		called = true
	}, true, "admin only")

	d.Dispatch(bot, makeMsgFrom("notadmin", "!killa"))
	if called {
		t.Error("admin-only handler reached via prefix should still be blocked")
	}
}

// --- stripHTML ---

func TestStripHTML(t *testing.T) {
	cases := []struct{ in, want string }{
		{"<b>hello</b>", "hello"},
		{"<p><b>!stop</b></p>", "!stop"},
		{"hello &amp; world", "hello & world"},
		{"<a href=\"x\">link</a>", "link"},
		{"  spaced  ", "spaced"},
		{"no tags", "no tags"},
	}
	for _, c := range cases {
		if got := stripHTML(c.in); got != c.want {
			t.Errorf("stripHTML(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// --- Register / HelpHandler ---

func TestRegister_Empty_Aliases_NoOp(t *testing.T) {
	d := makeDispatcher([]string{"!"})
	bot := defaultBot()
	d.Register(nil, func(b BotAPI, user string, msg *gumble.TextMessage, cmd, arg string) {}, false, "x")
	// Dispatching anything should not panic.
	d.Dispatch(bot, makeMsg("!anything"))
}

func TestHelpHandler_ContainsAllCommands(t *testing.T) {
	d := makeDispatcher([]string{"!"})
	d.Register([]string{"stop"}, handleStop, false, "Stop playback")
	d.Register([]string{"np", "now"}, handleNP, false, "Now playing")

	out := d.buildHelp("!", false)
	if !strings.Contains(out, "!stop") {
		t.Errorf("help output missing !stop: %q", out)
	}
	if !strings.Contains(out, "!np") {
		t.Errorf("help output missing !np: %q", out)
	}
}

func TestHelpHandler_FormattedWrapsInPre(t *testing.T) {
	d := makeDispatcher([]string{"!"})
	d.Register([]string{"stop"}, handleStop, false, "Stop playback")
	out := d.buildHelp("!", true)
	if !strings.HasPrefix(out, "<pre>") || !strings.HasSuffix(out, "</pre>") {
		t.Errorf("formatted help should be wrapped in <pre>: %q", out)
	}
}

func TestHelpHandler_SortedAlphabetically(t *testing.T) {
	d := makeDispatcher([]string{"!"})
	d.Register([]string{"zoo"}, handleStop, false, "Z cmd")
	d.Register([]string{"aardvark"}, handleMute, false, "A cmd")
	out := d.buildHelp("!", false)
	iAard := strings.Index(out, "!aardvark")
	iZoo := strings.Index(out, "!zoo")
	if iAard < 0 || iZoo < 0 || iAard >= iZoo {
		t.Errorf("help not sorted alphabetically: aardvark at %d, zoo at %d\n%s", iAard, iZoo, out)
	}
}

// --- isAdmin ---

func TestIsAdmin_CaseSensitive(t *testing.T) {
	cfg := &config.Config{Bot: config.BotConfig{Admin: []string{"Admin"}}}
	if isAdmin(cfg, "admin") {
		t.Error("admin check should be case-sensitive: 'admin' != 'Admin'")
	}
	if !isAdmin(cfg, "Admin") {
		t.Error("'Admin' should be recognised as admin")
	}
}
