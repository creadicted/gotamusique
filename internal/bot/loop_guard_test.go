package bot

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/konradk/gotamusique/internal/audio"
)

// failItem is a Resolver that always fails and counts attempts.
type failItem struct {
	mockItem
	calls *atomic.Int64
}

func (f *failItem) Resolve(ctx context.Context) error {
	f.calls.Add(1)
	return errors.New("unavailable")
}

// startLoop runs the play loop against an idle pipeline until the test ends.
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

// Regression: when every item fails, the loop must give up and go idle. It used
// to call Stop(), which reset the queue to index 0, so it retried the same
// failing items forever (in every mode).
func TestLoop_AllItemsFail_GoesIdleAndStops(t *testing.T) {
	for _, mode := range []string{"one-shot", "repeat", "random"} {
		t.Run(mode, func(t *testing.T) {
			b := startLoop(t, mode)
			var calls atomic.Int64
			for i := 0; i < 5; i++ {
				b.Enqueue(&failItem{mockItem{string(rune('a' + i))}, &calls})
			}

			waitFor(t, "queue to go idle", func() bool { return b.queue.Current() == nil })
			settled := calls.Load()
			time.Sleep(400 * time.Millisecond) // several loop ticks
			if got := calls.Load(); got != settled {
				t.Fatalf("loop kept retrying after giving up: %d -> %d attempts", settled, got)
			}
			if settled != 5 {
				t.Errorf("attempts before giving up = %d, want 5 (one per item)", settled)
			}
			if b.queue.Len() != 5 {
				t.Errorf("items were dropped: len = %d, want 5", b.queue.Len())
			}
		})
	}
}

// After giving up, the bot must be usable again: a newly queued item is tried.
func TestLoop_AfterGivingUp_NewItemIsTried(t *testing.T) {
	b := startLoop(t, "repeat")
	var calls atomic.Int64
	for i := 0; i < 3; i++ {
		b.Enqueue(&failItem{mockItem{string(rune('a' + i))}, &calls})
	}
	waitFor(t, "queue to go idle", func() bool { return b.queue.Current() == nil })
	before := calls.Load()

	b.Enqueue(&failItem{mockItem{"new"}, &calls})
	waitFor(t, "new item to be tried", func() bool { return calls.Load() > before })
}

// A successful track in between resets the failure count, so isolated failures
// never trigger the give-up path.
func TestLoop_FewerFailuresThanThreshold_DoesNotGoIdleEarly(t *testing.T) {
	b := startLoop(t, "one-shot")
	var calls atomic.Int64
	// 2 failing items: threshold is max(len, 3) = 3, so it just runs out of queue.
	b.Enqueue(&failItem{mockItem{"a"}, &calls})
	b.Enqueue(&failItem{mockItem{"b"}, &calls})
	waitFor(t, "queue to go idle", func() bool { return b.queue.Current() == nil })
	if got := calls.Load(); got != 2 {
		t.Errorf("attempts = %d, want 2", got)
	}
}
