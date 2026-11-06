package bot

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/konradk/gotamusique/internal/audio"
)

// scriptItem is a Resolver whose outcome is chosen per attempt. It counts
// attempts and can block inside Resolve until released, to hold a resolve open
// while the test changes the queue.
type scriptItem struct {
	mockItem
	fail    bool
	calls   atomic.Int64
	started chan struct{} // closed on the first Resolve call when non-nil
	gate    chan struct{} // Resolve blocks until closed when non-nil
	once    sync.Once
}

func newFailing(id string) *scriptItem { return &scriptItem{mockItem: mockItem{id}, fail: true} }

func (s *scriptItem) Resolve(ctx context.Context) error {
	s.calls.Add(1)
	if s.started != nil {
		s.once.Do(func() { close(s.started) })
	}
	if s.gate != nil {
		select {
		case <-s.gate:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if s.fail {
		return errors.New("unavailable")
	}
	return nil
}

// startLoop runs the play loop against a pipeline whose Launch is reachable but
// never produces audio, until the test ends.
func startLoop(t *testing.T, mode string) *Bot {
	t.Helper()
	cfg := defaultCfg()
	cfg.Bot.PlaybackMode = mode
	b, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	b.mu.Lock()
	b.audio = audio.New(nil, cfg, b.log)
	b.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { b.loop(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	return b
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func idle(b *Bot) bool { return b.queue.Current() == nil }

// Regression: when every item fails, the loop must give up and go idle (it used
// to call Stop(), which reset the queue to index 0 and retried forever).
func TestLoop_AllItemsFail_GoesIdleAndStops(t *testing.T) {
	for _, mode := range []string{"one-shot", "repeat", "random"} {
		t.Run(mode, func(t *testing.T) {
			b := startLoop(t, mode)
			items := make([]*scriptItem, 5)
			for i := range items {
				items[i] = newFailing(string(rune('a' + i)))
				b.Enqueue(items[i])
			}
			waitFor(t, "queue to go idle", func() bool { return idle(b) })

			total := func() (n int64) {
				for _, it := range items {
					n += it.calls.Load()
				}
				return
			}
			settled := total()
			time.Sleep(400 * time.Millisecond)
			if got := total(); got != settled {
				t.Fatalf("loop kept retrying after giving up: %d -> %d attempts", settled, got)
			}
			if b.queue.Len() != 5 {
				t.Errorf("items were dropped: len = %d, want 5", b.queue.Len())
			}
			// Every entry must have been tried; no entry is skipped untried.
			for _, it := range items {
				if it.calls.Load() == 0 {
					t.Errorf("item %s was never tried", it.id)
				}
			}
		})
	}
}

func TestLoop_AfterGivingUp_NewItemIsTried(t *testing.T) {
	b := startLoop(t, "repeat")
	for i := 0; i < 3; i++ {
		b.Enqueue(newFailing(string(rune('a' + i))))
	}
	waitFor(t, "queue to go idle", func() bool { return idle(b) })

	n := newFailing("new")
	b.Enqueue(n)
	waitFor(t, "new item to be tried", func() bool { return n.calls.Load() > 0 })
}

// Review finding: with a playable A and failing B/C, a random reshuffle can retry
// B and C before A comes up. The bot must not give up while A is still playable.
// Here A cannot actually reach audio in this harness, so assert on the guard
// decision instead: the failing entries alone must not exhaust a queue that has
// an untried playable entry.
func TestLoop_DoesNotGiveUpWhileAnUntriedEntryRemains(t *testing.T) {
	b := startLoop(t, "random")
	good := &scriptItem{mockItem: mockItem{"good"}, gate: make(chan struct{}), started: make(chan struct{})}
	bad1, bad2 := newFailing("b1"), newFailing("b2")
	// good blocks in Resolve, so it is "untried" from the guard's point of view.
	b.Enqueue(bad1)
	b.Enqueue(bad2)
	b.Enqueue(good)

	waitFor(t, "loop to reach the untried entry", func() bool {
		select {
		case <-good.started:
			return true
		default:
			return false
		}
	})
	if idle(b) {
		t.Error("gave up while an untried entry was still in the queue")
	}
	close(good.gate)
}

// Review finding: an item appended during the final failing resolve must not be
// skipped by the transition to idle.
func TestLoop_ItemQueuedDuringFinalFailingResolve_IsNotSkipped(t *testing.T) {
	b := startLoop(t, "one-shot")
	last := newFailing("last")
	last.gate = make(chan struct{})
	last.started = make(chan struct{})
	b.Enqueue(last)

	<-last.started // the loop is resolving the only item
	added := newFailing("added")
	b.Enqueue(added) // user queues something while it is failing
	close(last.gate) // now `last` fails

	waitFor(t, "the appended item to be tried", func() bool { return added.calls.Load() > 0 })
}

// Review finding: !play re-selecting the SAME item during its failing resolve
// must not be overwritten by the failure handling.
func TestLoop_ReselectSameItemDuringFailingResolve_NotOverwritten(t *testing.T) {
	b := startLoop(t, "repeat")
	only := newFailing("only")
	only.gate = make(chan struct{})
	only.started = make(chan struct{})
	b.Enqueue(only)

	<-only.started
	if err := b.Play(0); err != nil { // re-selects the same (only) item
		t.Fatal(err)
	}
	close(only.gate)
	// The stale failure must not push the queue anywhere; the loop re-evaluates and
	// tries `only` again (calls > 1) rather than treating the stale result as final.
	waitFor(t, "the re-selected item to be retried", func() bool { return only.calls.Load() >= 2 })
}

// Same item twice in a one-item repeat queue: the failure handler's snapshot goes
// stale after a wrap, so it must not finish the queue on the strength of an old
// item-equality check.
func TestLoop_SkipWrapsOneItemRepeatDuringFailingResolve_NotOverwritten(t *testing.T) {
	b := startLoop(t, "repeat")
	only := newFailing("only")
	only.gate = make(chan struct{})
	only.started = make(chan struct{})
	b.Enqueue(only)

	<-only.started
	b.Skip() // idle pipeline: advances the queue itself, wrapping back onto `only`
	close(only.gate)
	waitFor(t, "the item to be retried after the skip", func() bool { return only.calls.Load() >= 2 })
}
