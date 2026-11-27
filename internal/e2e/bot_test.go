//go:build e2e

package e2e

import (
	"testing"
	"time"
)

// The bot connects to a real server and answers a chat command.
func TestBotAnswersCommand(t *testing.T) {
	addr := startMumble(t)
	startBot(t, addr, "")
	h := joinHuman(t, addr, "alice")

	// The bot may still be joining; ask until it answers.
	eventually(t, 20*time.Second, "!version reply", func() bool {
		h.say("!version")
		time.Sleep(500 * time.Millisecond)
		return h.heard("gotamusique")
	})
}

// The reported bug: the bot is alone, something is queued, and nobody joins or
// leaves. It must not start playing to an empty channel.
func TestAutoPause_trackQueuedWhileAlone(t *testing.T) {
	addr := startMumble(t)
	b := startBot(t, addr, "auto_pause_when_alone = True")

	// Wait for the bot to be connected: a human sees it join.
	probe := joinHuman(t, addr, "probe")
	eventually(t, 20*time.Second, "bot in the channel", func() bool {
		n := 0
		probe.client.Do(func() { n = len(probe.client.Self.Channel.Users) })
		return n >= 2
	})
	probe.leave()
	time.Sleep(500 * time.Millisecond)

	b.Enqueue(&fakeItem{url: "http://127.0.0.1:9/never.mp3"})
	eventually(t, 10*time.Second, "auto-pause with nobody listening", b.IsPaused)

	// Someone arrives: playback is allowed again.
	joinHuman(t, addr, "alice")
	eventually(t, 10*time.Second, "auto-resume when a listener joins", func() bool { return !b.IsPaused() })
}

// A listener leaving pauses the bot; coming back resumes it.
func TestAutoPause_lastListenerLeaves(t *testing.T) {
	addr := startMumble(t)
	b := startBot(t, addr, "auto_pause_when_alone = True")
	h := joinHuman(t, addr, "alice")
	eventually(t, 20*time.Second, "bot in the channel", func() bool {
		n := 0
		h.client.Do(func() { n = len(h.client.Self.Channel.Users) })
		return n >= 2
	})

	b.Enqueue(&fakeItem{url: silenceServer(t)})
	never(t, 1500*time.Millisecond, "paused while a listener is present", b.IsPaused)

	h.leave()
	eventually(t, 10*time.Second, "pause after the last listener left", b.IsPaused)

	joinHuman(t, addr, "bob")
	eventually(t, 10*time.Second, "resume after a listener returned", func() bool { return !b.IsPaused() })
}

// With the feature off the bot never pauses itself.
func TestAutoPause_offByDefault(t *testing.T) {
	addr := startMumble(t)
	b := startBot(t, addr, "")
	h := joinHuman(t, addr, "alice")
	eventually(t, 20*time.Second, "bot in the channel", func() bool {
		n := 0
		h.client.Do(func() { n = len(h.client.Self.Channel.Users) })
		return n >= 2
	})
	b.Enqueue(&fakeItem{url: "http://127.0.0.1:9/never.mp3"})
	h.leave()
	never(t, 2*time.Second, "auto-pause although the feature is off", b.IsPaused)
}
