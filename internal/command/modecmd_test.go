package command

import (
	"errors"
	"testing"

	"github.com/konradk/gotamusique/internal/library"
)

func TestHandleMode_NoArg_DoesNotSet(t *testing.T) {
	bot := defaultBot()
	callHandler(handleMode, bot, "")
	if len(bot.setModeCalls) != 0 {
		t.Errorf("no-arg !mode must not set anything, got %v", bot.setModeCalls)
	}
}

func TestHandleMode_SetsEachMode(t *testing.T) {
	for _, m := range []string{"one-shot", "repeat", "random"} {
		bot := defaultBot()
		callHandler(handleMode, bot, m)
		if len(bot.setModeCalls) != 1 || bot.setModeCalls[0] != m {
			t.Errorf("!mode %s: setModeCalls = %v", m, bot.setModeCalls)
		}
		if bot.PlaybackMode() != m {
			t.Errorf("!mode %s: mode = %q", m, bot.PlaybackMode())
		}
	}
}

func TestHandleMode_CaseInsensitive(t *testing.T) {
	bot := defaultBot()
	callHandler(handleMode, bot, "  REPEAT ")
	if bot.PlaybackMode() != "repeat" {
		t.Errorf("mode = %q, want repeat", bot.PlaybackMode())
	}
}

func TestHandleMode_Autoplay_IsForwarded(t *testing.T) {
	bot := defaultBot()
	callHandler(handleMode, bot, "autoplay")
	if len(bot.setModeCalls) != 1 || bot.setModeCalls[0] != "autoplay" {
		t.Errorf("setModeCalls = %v, want [autoplay]", bot.setModeCalls)
	}
	if bot.PlaybackMode() != "autoplay" {
		t.Errorf("mode = %q, want autoplay", bot.PlaybackMode())
	}
}

func TestHandleMode_Autoplay_LibraryErrorsLeaveModeUnchanged(t *testing.T) {
	for name, err := range map[string]error{
		"no library":    library.ErrUnavailable,
		"empty library": library.ErrEmpty,
	} {
		bot := defaultBot()
		bot.setModeErr = err
		callHandler(handleMode, bot, "autoplay")
		if bot.PlaybackMode() != "one-shot" {
			t.Errorf("%s: mode = %q, want unchanged", name, bot.PlaybackMode())
		}
	}
}

func TestHandleMode_Invalid_Refused(t *testing.T) {
	for _, arg := range []string{"shuffle", "oneshot", "repeat now", "--x"} {
		bot := defaultBot()
		callHandler(handleMode, bot, arg)
		if len(bot.setModeCalls) != 0 {
			t.Errorf("%q must not set a mode, got %v", arg, bot.setModeCalls)
		}
	}
}

func TestHandleMode_SetError_NoChange(t *testing.T) {
	bot := defaultBot()
	bot.setModeErr = errors.New("boom")
	callHandler(handleMode, bot, "repeat")
	if bot.PlaybackMode() != "one-shot" {
		t.Errorf("mode = %q, want unchanged", bot.PlaybackMode())
	}
}

func TestHandleMode_OpenToNonAdmins(t *testing.T) {
	d := NewDispatcher([]string{"!"})
	RegisterAll(defaultBot(), d)
	e, ok := d.handlers["mode"]
	if !ok {
		t.Fatal("mode command not registered")
	}
	if e.adminOnly {
		t.Error("!mode must be open to all users")
	}
}
