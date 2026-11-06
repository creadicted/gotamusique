package bot

import (
	"context"
	"errors"
	"testing"
	"time"
)

// resolverItem is a MediaItem+Resolver whose Resolve blocks until released or
// its context is cancelled.
type resolverItem struct {
	mockItem
	started chan struct{}
	release chan error
}

func newResolverItem(id string) *resolverItem {
	return &resolverItem{mockItem: mockItem{id}, started: make(chan struct{}), release: make(chan error, 1)}
}

func (r *resolverItem) Resolve(ctx context.Context) error {
	close(r.started)
	select {
	case err := <-r.release:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// resolveAsync runs resolveItem the way the loop does and returns its result channel.
func resolveAsync(b *Bot, item *resolverItem) <-chan [2]any {
	out := make(chan [2]any, 1)
	epoch := b.playEpoch.Load()
	go func() {
		err, stale := b.resolveItem(context.Background(), item, epoch)
		out <- [2]any{err, stale}
	}()
	return out
}

func wait(t *testing.T, ch <-chan [2]any) (error, bool) {
	t.Helper()
	select {
	case r := <-ch:
		err, _ := r[0].(error)
		return err, r[1].(bool)
	case <-time.After(3 * time.Second):
		t.Fatal("resolveItem did not return")
		return nil, false
	}
}

func newTestBot(t *testing.T) *Bot {
	t.Helper()
	b, err := New(defaultCfg())
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestResolveItem_NoControl_NotStale(t *testing.T) {
	b := newTestBot(t)
	it := newResolverItem("a")
	b.Enqueue(it)
	res := resolveAsync(b, it)
	<-it.started
	it.release <- nil
	err, stale := wait(t, res)
	if err != nil || stale {
		t.Errorf("err=%v stale=%v, want nil/false", err, stale)
	}
}

func TestResolveItem_FailureNotStale_ReportsError(t *testing.T) {
	b := newTestBot(t)
	it := newResolverItem("a")
	b.Enqueue(it)
	res := resolveAsync(b, it)
	<-it.started
	boom := errors.New("boom")
	it.release <- boom
	err, stale := wait(t, res)
	if !errors.Is(err, boom) || stale {
		t.Errorf("err=%v stale=%v, want boom/false", err, stale)
	}
}

func TestResolveItem_SkipDuringResolve_Stale(t *testing.T) {
	b := newTestBot(t)
	a, c := newResolverItem("a"), &mockItem{"c"}
	b.Enqueue(a)
	b.Enqueue(c)
	res := resolveAsync(b, a)
	<-a.started
	b.Skip() // idle pipeline: advances the queue itself
	a.release <- nil
	if _, stale := wait(t, res); !stale {
		t.Error("Skip during resolve must make the result stale")
	}
	if got := b.QueueCurrentIndex(); got != 1 {
		t.Errorf("index = %d, want 1 (resolver must not advance again)", got)
	}
}

func TestResolveItem_ClearDuringResolve_Stale(t *testing.T) {
	b := newTestBot(t)
	a := newResolverItem("a")
	b.Enqueue(a)
	res := resolveAsync(b, a)
	<-a.started
	b.Clear()
	a.release <- nil
	if _, stale := wait(t, res); !stale {
		t.Error("Clear during resolve must make the result stale")
	}
}

func TestResolveItem_StopDuringResolve_Stale(t *testing.T) {
	b := newTestBot(t)
	a := newResolverItem("a")
	b.Enqueue(a)
	res := resolveAsync(b, a)
	<-a.started
	b.Stop()
	a.release <- nil
	if _, stale := wait(t, res); !stale {
		t.Error("Stop during resolve must make the result stale")
	}
}

func TestResolveItem_PlayDuringResolve_Stale(t *testing.T) {
	b := newTestBot(t)
	a := newResolverItem("a")
	b.Enqueue(a)
	b.Enqueue(&mockItem{"b"})
	res := resolveAsync(b, a)
	<-a.started
	if err := b.Play(1); err != nil {
		t.Fatal(err)
	}
	a.release <- nil
	if _, stale := wait(t, res); !stale {
		t.Error("Play during resolve must make the result stale")
	}
}

func TestResolveItem_ControlCancelsInFlightResolve(t *testing.T) {
	b := newTestBot(t)
	a := newResolverItem("a")
	b.Enqueue(a)
	b.Enqueue(&mockItem{"b"})
	res := resolveAsync(b, a)
	<-a.started
	b.Skip() // never releases a: the resolve must be cancelled, not wait out the timeout
	err, stale := wait(t, res)
	if !stale {
		t.Error("want stale")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}
