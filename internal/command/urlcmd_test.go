package command

import (
	"os"
	"path/filepath"
	"testing"

	urlitem "github.com/konradk/gotamusique/internal/media/url"
)

// fakeYT installs a fake yt-dlp script and points the bot's config at it.
func fakeYT(t *testing.T, bot *mockBot, body string) {
	t.Helper()
	script := filepath.Join(t.TempDir(), "yt-dlp")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	bot.cfg.YoutubeDL.Binary = script
	bot.cfg.Bot.MaxTrackDuration = 60
	bot.cfg.Bot.MaxTrackPlaylist = 20
}

const fakeBody = `
case "$*" in
  *--dump-single-json*)
    case "$*" in
      *plist*) echo '{"_type":"playlist","id":"p"}' ;;
      *)       echo '{"id":"a","title":"Song","duration":100}' ;;
    esac ;;
  *--flat-playlist*)
    echo '{"title":"A","url":"https://example.com/a"}'
    echo '{"title":"B","url":"https://example.com/b"}'
    echo '{"title":"C","url":"https://example.com/c"}' ;;
esac
`

func TestHandleURL_NoArg_NoEnqueue(t *testing.T) {
	bot := defaultBot()
	callHandler(handleURL, bot, "")
	ytWG.Wait()
	if len(bot.enqueueCalls) != 0 {
		t.Errorf("enqueued %d, want 0", len(bot.enqueueCalls))
	}
}

func TestHandleURL_RejectsBadInput_NoEnqueue(t *testing.T) {
	for _, arg := range []string{"file:///etc/passwd", "--exec=id", "ftp://x/y", "garbage"} {
		bot := defaultBot()
		fakeYT(t, bot, fakeBody)
		callHandler(handleURL, bot, arg)
		ytWG.Wait()
		if len(bot.enqueueCalls) != 0 {
			t.Errorf("%q: enqueued %d, want 0", arg, len(bot.enqueueCalls))
		}
	}
}

func TestHandleURL_Valid_EnqueuesURLItem(t *testing.T) {
	bot := defaultBot()
	fakeYT(t, bot, fakeBody)
	callHandler(handleURL, bot, "https://example.com/watch?v=a")
	ytWG.Wait()
	if len(bot.enqueueCalls) != 1 {
		t.Fatalf("enqueued %d, want 1", len(bot.enqueueCalls))
	}
	it, ok := bot.enqueueCalls[0].(*urlitem.URLItem)
	if !ok || it.Title != "Song" {
		t.Errorf("unexpected item: %#v", bot.enqueueCalls[0])
	}
}

func TestHandleURL_PlaylistURL_NoEnqueue(t *testing.T) {
	bot := defaultBot()
	fakeYT(t, bot, fakeBody)
	callHandler(handleURL, bot, "https://example.com/plist")
	ytWG.Wait()
	if len(bot.enqueueCalls) != 0 {
		t.Errorf("enqueued %d, want 0", len(bot.enqueueCalls))
	}
}

func TestHandleURL_MissingBinary_NoEnqueue(t *testing.T) {
	bot := defaultBot()
	bot.cfg.YoutubeDL.Binary = filepath.Join(t.TempDir(), "absent")
	callHandler(handleURL, bot, "https://example.com/a")
	ytWG.Wait()
	if len(bot.enqueueCalls) != 0 {
		t.Errorf("enqueued %d, want 0", len(bot.enqueueCalls))
	}
}

func TestHandlePlaylist_EnqueuesCapped(t *testing.T) {
	bot := defaultBot()
	fakeYT(t, bot, fakeBody)
	bot.cfg.Bot.MaxTrackPlaylist = 2
	callHandler(handlePlaylist, bot, "https://example.com/list")
	ytWG.Wait()
	if len(bot.enqueueCalls) != 2 {
		t.Fatalf("enqueued %d, want 2", len(bot.enqueueCalls))
	}
	for _, it := range bot.enqueueCalls {
		if it.StreamURL() != "" {
			t.Error("playlist items must be unresolved at enqueue time")
		}
	}
}

func TestHandlePlaylist_BadURL_NoEnqueue(t *testing.T) {
	bot := defaultBot()
	fakeYT(t, bot, fakeBody)
	callHandler(handlePlaylist, bot, "file:///x")
	ytWG.Wait()
	if len(bot.enqueueCalls) != 0 {
		t.Errorf("enqueued %d, want 0", len(bot.enqueueCalls))
	}
}
