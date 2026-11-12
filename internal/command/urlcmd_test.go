package command

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

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

// blockingBody is a fake yt-dlp that signals it started (a file per process) and
// then waits until the test creates "release", so a test can hold yt-dlp slots.
func blockingBody(dir string) string {
	return "touch \"" + dir + "/started.$$\"\n" +
		"while [ ! -f \"" + dir + "/release\" ]; do sleep 0.02; done\n" +
		`echo '{"id":"a","title":"Song","duration":100}'` + "\n"
}

func waitStarted(t *testing.T, dir string, n int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if m, _ := filepath.Glob(filepath.Join(dir, "started.*")); len(m) >= n {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("fewer than %d yt-dlp processes started", n)
}

// The slots are shared with everything else that runs yt-dlp. With all of them
// taken, a chat request waits only as long as its own timeout and then says so,
// instead of running on top of the others or hanging.
func TestHandleURL_RepliesBusyWhenEverySlotIsTaken(t *testing.T) {
	old := ytTimeout
	ytTimeout = 250 * time.Millisecond
	defer func() { ytTimeout = old }()

	dir := t.TempDir()
	bot := defaultBot()
	fakeYT(t, bot, blockingBody(dir))

	// Hold every slot, the way a playback-time resolution or a web add would.
	hold, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for i := 0; i < urlitem.MaxConcurrent; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			urlitem.GetInfo(hold, "https://example.com/hold", bot.cfg) //nolint:errcheck
		}()
	}
	waitStarted(t, dir, urlitem.MaxConcurrent)

	var r replies
	handleURL(bot, testCtx(&r), "url", "https://example.com/late")
	ytWG.Wait()

	got := strings.Join(r.all(), "\n")
	if !strings.Contains(strings.ToLower(got), "too many requests") {
		t.Errorf("replies = %q, want a busy message", got)
	}
	if len(bot.enqueueCalls) != 0 {
		t.Errorf("enqueued %d, want 0", len(bot.enqueueCalls))
	}
	if m, _ := filepath.Glob(filepath.Join(dir, "started.*")); len(m) != urlitem.MaxConcurrent {
		t.Errorf("%d yt-dlp processes ran, want %d: a refused request must not start one", len(m), urlitem.MaxConcurrent)
	}

	if err := os.WriteFile(filepath.Join(dir, "release"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
}

func TestRunYT_RunsWithADeadline(t *testing.T) {
	ran := make(chan struct{}, 1)
	runYT(func(ctx context.Context) {
		if _, ok := ctx.Deadline(); !ok {
			t.Error("ctx must carry a deadline")
		}
		ran <- struct{}{}
	})
	ytWG.Wait()
	select {
	case <-ran:
	default:
		t.Error("fn did not run")
	}
}
