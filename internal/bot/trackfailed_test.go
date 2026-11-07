package bot

import (
	"errors"
	"testing"

	"github.com/konradk/gotamusique/internal/audio"
)

func newBot(t *testing.T, mode string) *Bot {
	t.Helper()
	cfg := defaultCfg()
	cfg.Bot.PlaybackMode = mode
	b, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

var errBoom = errors.New("boom")

// Review comment 3: with playable A and failing B/C, the sequence A -> B -> C -> B
// reaches "3 consecutive failures" if attempts are counted, even though A was
// never tried in the streak. Only distinct failed entries count, so the bot must
// not give up while A is still in the queue.
func TestTrackFailed_RetriesOfSameEntriesDoNotExhaustQueue(t *testing.T) {
	b := newBot(t, "repeat")
	a, bad1, bad2 := &mockItem{"a"}, &mockItem{"b"}, &mockItem{"c"}
	b.Enqueue(a)
	b.Enqueue(bad1)
	b.Enqueue(bad2)

	for i, it := range []audio.MediaItem{bad1, bad2, bad1, bad2, bad1} {
		b.trackFailed(it, b.queue.Snapshot(), ": failed — ", errBoom)
		if b.queue.Current() == nil {
			t.Fatalf("failure #%d: went idle while playable A was never tried", i+1)
		}
	}
}

func TestTrackFailed_GivesUpWhenEveryEntryFailed(t *testing.T) {
	b := newBot(t, "repeat")
	items := []*mockItem{{"a"}, {"b"}, {"c"}}
	for _, it := range items {
		b.Enqueue(it)
	}
	for _, it := range items {
		b.trackFailed(it, b.queue.Snapshot(), ": failed — ", errBoom)
	}
	if b.queue.Current() != nil {
		t.Error("every entry failed: the queue must go idle")
	}
	if b.queue.Len() != 3 {
		t.Errorf("items dropped: len = %d", b.queue.Len())
	}
}

func TestTrackFailed_PlayedTrackClearsStreak(t *testing.T) {
	b := newBot(t, "repeat")
	a, bad := &mockItem{"a"}, &mockItem{"b"}
	b.Enqueue(a)
	b.Enqueue(bad)
	b.trackFailed(bad, b.queue.Snapshot(), ": failed — ", errBoom)
	b.fails.reset() // what onTrackEnd does when a track plays
	b.trackFailed(a, b.queue.Snapshot(), ": failed — ", errBoom)
	if b.queue.Current() == nil {
		t.Error("a played track in between must reset the streak")
	}
}

// Review comment 2: an item appended while the last entry was failing must not be
// skipped by the idle transition.
func TestTrackFailed_ItemAppendedDuringFailureIsNotSkipped(t *testing.T) {
	b := newBot(t, "one-shot")
	only := &mockItem{"only"}
	b.Enqueue(only)
	snap := b.queue.Snapshot() // the loop starts on `only`

	added := &mockItem{"added"}
	b.Enqueue(added) // user queues something while `only` is failing

	b.trackFailed(only, snap, ": failed — ", errBoom)
	if b.queue.Current() == nil {
		t.Fatal("went idle: the appended item was skipped without being tried")
	}
	if b.queue.Index() != 0 {
		t.Errorf("a stale failure moved the queue to %d", b.queue.Index())
	}
}

// Review comment 2: !play re-selecting the same item (or a skip wrapping a
// one-item repeat queue back onto it) leaves item-equality true; the revision
// check must still reject the stale failure.
func TestTrackFailed_SameItemReselected_NotOverwritten(t *testing.T) {
	for name, change := range map[string]func(b *Bot){
		"play same":      func(b *Bot) { _ = b.Play(0) },
		"skip wraps":     func(b *Bot) { b.Skip() },
		"stop then play": func(b *Bot) { b.Stop(); _ = b.Play(0) },
	} {
		t.Run(name, func(t *testing.T) {
			b := newBot(t, "repeat")
			only := &mockItem{"only"}
			b.Enqueue(only)
			snap := b.queue.Snapshot()

			change(b)
			idx := b.queue.Index()
			b.trackFailed(only, snap, ": failed — ", errBoom)

			if b.queue.Current() != only || b.queue.Index() != idx {
				t.Errorf("stale failure overwrote the newer selection (index %d -> %d)", idx, b.queue.Index())
			}
		})
	}
}

// Review comment 1: an input ffmpeg cannot open ends with ErrNoAudio; that must
// count as a failure and not reset the guard or advance as a finished track.
func TestOnTrackEnd_NoAudioCountsAsFailure(t *testing.T) {
	b := newBot(t, "repeat")
	items := []*mockItem{{"a"}, {"b"}}
	for _, it := range items {
		b.Enqueue(it)
	}
	ver := b.launchVersion.Load()
	for _, it := range items {
		b.onTrackEnd(it, b.queue.Snapshot(), ver, audio.ErrNoAudio)
	}
	if b.queue.Current() != nil {
		t.Error("two unplayable files in a repeat queue must end in idle, not loop forever")
	}
}

func TestOnTrackEnd_CleanEndAdvancesAndResetsStreak(t *testing.T) {
	b := newBot(t, "one-shot")
	a, c := &mockItem{"a"}, &mockItem{"c"}
	b.Enqueue(a)
	b.Enqueue(c)
	b.fails.fail(c) // a stale streak entry
	b.onTrackEnd(a, b.queue.Snapshot(), b.launchVersion.Load(), nil)
	if b.queue.Current() != c {
		t.Error("a finished track must advance the queue")
	}
	if b.fails.allFailed([]audio.MediaItem{c}) {
		t.Error("a finished track must reset the failure streak")
	}
}

func TestOnTrackEnd_NewSelectionIsNotMoved(t *testing.T) {
	b := newBot(t, "one-shot")
	a, c := &mockItem{"a"}, &mockItem{"c"}
	b.Enqueue(a)
	b.Enqueue(c)
	ver := b.launchVersion.Load()
	_ = b.Play(1) // bumps launchVersion
	b.onTrackEnd(a, b.queue.Snapshot(), ver, nil)
	if b.queue.Current() != c || b.queue.Index() != 1 {
		t.Error("onTrackEnd must not move an index Play() just set")
	}
}
