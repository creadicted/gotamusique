package bot

import (
	"context"
	"testing"
	"time"
)

func TestFormatTrackAnnouncement(t *testing.T) {
	if got := formatTrackAnnouncement("Artist - Song"); got != "Now playing: Artist - Song" {
		t.Errorf("got %q", got)
	}
	// Titles come from the station: markup must not reach the chat unescaped.
	if got := formatTrackAnnouncement("<b>x</b> & y"); got != "Now playing: &lt;b&gt;x&lt;/b&gt; &amp; y" {
		t.Errorf("got %q", got)
	}
}

// watcherItem is a MediaItem that also reports track changes.
type watcherItem struct {
	mockItem
	titles  []string
	started chan struct{}
	stopped chan struct{}
}

func (w *watcherItem) WatchTracks(ctx context.Context, fn func(string)) {
	close(w.started)
	for _, t := range w.titles {
		fn(t)
	}
	<-ctx.Done()
	close(w.stopped)
}

func newWatcher(titles ...string) *watcherItem {
	return &watcherItem{mockItem: mockItem{"w"}, titles: titles,
		started: make(chan struct{}), stopped: make(chan struct{})}
}

func announceBot(t *testing.T, on bool) *Bot {
	t.Helper()
	cfg := defaultCfg()
	cfg.Bot.AnnounceRadioTracks = on
	b, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestWatchTracks_startsAndStopsWithContext(t *testing.T) {
	b := announceBot(t, true)
	w := newWatcher("A - B")
	ctx, cancel := context.WithCancel(context.Background())
	b.watchTracks(ctx, w)

	select {
	case <-w.started:
	case <-time.After(2 * time.Second):
		t.Fatal("watcher was not started")
	}
	cancel()
	select {
	case <-w.stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("watcher did not stop when the context was cancelled")
	}
}

func TestWatchTracks_disabledByConfig(t *testing.T) {
	b := announceBot(t, false)
	w := newWatcher("A - B")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b.watchTracks(ctx, w)

	select {
	case <-w.started:
		t.Fatal("watcher started although announce_radio_tracks = False")
	case <-time.After(150 * time.Millisecond):
	}
}

func TestWatchTracks_ignoresItemsWithoutWatcher(t *testing.T) {
	b := announceBot(t, true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b.watchTracks(ctx, &mockItem{"plain"}) // must neither panic nor start anything
}
