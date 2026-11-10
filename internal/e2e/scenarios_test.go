//go:build e2e

package e2e

import (
	"testing"
	"time"

	"github.com/konradk/gotamusique/internal/radio"
)

func inChannelWith(h *human, n int) func() bool {
	return func() bool {
		c := 0
		h.client.Do(func() { c = len(h.client.Self.Channel.Users) })
		return c >= n
	}
}

// The server goes away and comes back: the bot reconnects by itself and is
// usable again. (The first retry waits backoffInitial, 5 s.)
func TestReconnect_afterServerRestart(t *testing.T) {
	addr, restart := startMumbleRestartable(t)
	startBot(t, addr, "")
	alice := joinHuman(t, addr, "alice")
	eventually(t, 20*time.Second, "bot in the channel", inChannelWith(alice, 2))

	restart()

	bob := joinHuman(t, addr, "bob")
	eventually(t, 60*time.Second, "bot back after the server restart", inChannelWith(bob, 2))
	eventually(t, 20*time.Second, "!version reply after reconnect", func() bool {
		bob.say("!version")
		time.Sleep(500 * time.Millisecond)
		return bob.heard("gotamusique")
	})
}

// An auto-pause belongs to the old connection: after a reconnect into a channel
// with a listener the bot must not stay paused.
func TestReconnect_clearsAutoPause(t *testing.T) {
	needFFmpeg(t)
	addr, restart := startMumbleRestartable(t)
	b := startBot(t, addr, "auto_pause_when_alone = True")
	alice := joinHuman(t, addr, "alice")
	eventually(t, 20*time.Second, "bot in the channel", inChannelWith(alice, 2))

	b.Enqueue(&fakeItem{url: silenceServer(t)})
	alice.leave()
	eventually(t, 10*time.Second, "auto-pause", b.IsPaused)

	restart()
	bob := joinHuman(t, addr, "bob")
	eventually(t, 60*time.Second, "bot back after the server restart", inChannelWith(bob, 2))
	eventually(t, 20*time.Second, "playback allowed again with a listener", func() bool { return !b.IsPaused() })
}

// A station that sends ICY metadata: each song change is announced in chat once.
func TestRadio_announcesSongChanges(t *testing.T) {
	needFFmpeg(t)
	addr := startMumble(t)
	b := startBot(t, addr, "")
	alice := joinHuman(t, addr, "alice")
	eventually(t, 20*time.Second, "bot in the channel", inChannelWith(alice, 2))

	url := icyServer(t, [2]string{"Artist A - Song 1", "Artist B - Song 2"}, 3*time.Second)
	b.Enqueue(radio.NewRadioItemFromURL(url))

	eventually(t, 20*time.Second, "first song announced", func() bool { return alice.heard("Now playing: Artist A - Song 1") })
	eventually(t, 20*time.Second, "song change announced", func() bool { return alice.heard("Now playing: Artist B - Song 2") })
}

// announce_radio_tracks = False: the station's titles stay out of the chat.
func TestRadio_announcementsCanBeDisabled(t *testing.T) {
	needFFmpeg(t)
	addr := startMumble(t)
	b := startBot(t, addr, "announce_radio_tracks = False")
	alice := joinHuman(t, addr, "alice")
	eventually(t, 20*time.Second, "bot in the channel", inChannelWith(alice, 2))

	url := icyServer(t, [2]string{"Artist A - Song 1", "Artist B - Song 2"}, 2*time.Second)
	b.Enqueue(radio.NewRadioItemFromURL(url))

	eventually(t, 20*time.Second, "track start announced", func() bool { return alice.heard("[Radio]") })
	never(t, 6*time.Second, "a song announcement although it is disabled", func() bool { return alice.heard("Artist A") || alice.heard("Artist B") })
}
