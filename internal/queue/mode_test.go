package queue

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/konradk/gotamusique/internal/audio"
)

func fill(q *Queue, n int) []audio.MediaItem {
	items := make([]audio.MediaItem, n)
	for i := range items {
		items[i] = item(fmt.Sprint(i))
		q.Append(items[i])
	}
	return items
}

func ids(q *Queue) []string {
	var out []string
	for _, it := range q.Items() {
		out = append(out, it.(*testItem).id)
	}
	return out
}

func sameSet(a, b []audio.MediaItem) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[audio.MediaItem]int{}
	for _, x := range a {
		seen[x]++
	}
	for _, x := range b {
		seen[x]--
	}
	for _, v := range seen {
		if v != 0 {
			return false
		}
	}
	return true
}

// --- ParseMode ---

func TestParseMode(t *testing.T) {
	for in, want := range map[string]Mode{
		"one-shot": ModeOneShot, "repeat": ModeRepeat, "random": ModeRandom,
		"  Repeat ": ModeRepeat, "RANDOM": ModeRandom,
	} {
		got, err := ParseMode(in)
		if err != nil || got != want {
			t.Errorf("ParseMode(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if got, err := ParseMode("autoplay"); err != nil || got != ModeAutoplay {
		t.Errorf("ParseMode(autoplay) = %q, %v; want autoplay", got, err)
	}
	if got, err := ParseMode("  AutoPlay "); err != nil || got != ModeAutoplay {
		t.Errorf("ParseMode is case/space-insensitive: got %q, %v", got, err)
	}
	for _, bad := range []string{"", "shuffle", "oneshot", "loop"} {
		if _, err := ParseMode(bad); !errors.Is(err, ErrUnknownMode) {
			t.Errorf("ParseMode(%q) err = %v, want ErrUnknownMode", bad, err)
		}
	}
}

func TestModeList(t *testing.T) {
	if got := ModeList(); got != "one-shot, repeat, random, autoplay" {
		t.Errorf("ModeList = %q", got)
	}
}

// --- default / one-shot ---

func TestMode_DefaultIsOneShot(t *testing.T) {
	if m := NewQueue().Mode(); m != ModeOneShot {
		t.Errorf("Mode = %q, want one-shot", m)
	}
	if m := (&Queue{}).Mode(); m != ModeOneShot {
		t.Errorf("zero-value Mode = %q, want one-shot", m)
	}
}

func TestNext_OneShot_StopsAtEnd(t *testing.T) {
	q := NewQueue()
	fill(q, 2)
	if !q.Next() || q.Index() != 1 {
		t.Fatal("first Next should advance to 1")
	}
	if q.Next() || q.Index() != 2 || q.Current() != nil {
		t.Errorf("end of one-shot: index=%d current=%v", q.Index(), q.Current())
	}
	if q.Next() || q.Index() != 2 {
		t.Error("Next past the end must stay a no-op")
	}
}

// --- repeat ---

func TestNext_Repeat_WrapsToStart(t *testing.T) {
	q := NewQueue()
	items := fill(q, 3)
	q.SetMode(ModeRepeat)
	for lap := 0; lap < 3; lap++ {
		for i := 0; i < 3; i++ {
			if q.Current() != items[i] {
				t.Fatalf("lap %d pos %d: wrong item", lap, i)
			}
			if !q.Next() {
				t.Fatalf("lap %d pos %d: Next returned false in repeat", lap, i)
			}
		}
	}
}

func TestNext_Repeat_SingleItemReplays(t *testing.T) {
	q := NewQueue()
	a := item("a")
	q.Append(a)
	q.SetMode(ModeRepeat)
	for i := 0; i < 3; i++ {
		if !q.Next() || q.Current() != a || q.Index() != 0 {
			t.Fatalf("iteration %d: single item must replay", i)
		}
	}
}

func TestNext_Repeat_EmptyQueue(t *testing.T) {
	q := NewQueue()
	q.SetMode(ModeRepeat)
	if q.Next() {
		t.Error("Next on an empty queue must return false")
	}
}

func TestNext_SwitchToOneShotStopsAgain(t *testing.T) {
	q := NewQueue()
	fill(q, 2)
	q.SetMode(ModeRepeat)
	q.Next()
	q.Next() // wrapped
	q.SetMode(ModeOneShot)
	q.Next() // -> 1
	if q.Next() {
		t.Error("after switching back to one-shot the end must stop playback")
	}
}

// --- random ---

func TestSetMode_RandomShufflesOnlyAfterCurrent(t *testing.T) {
	q := NewQueue()
	items := fill(q, 20)
	q.Next()
	q.Next() // current = items[2]
	q.SetMode(ModeRandom)

	got := q.Items()
	if got[0] != items[0] || got[1] != items[1] || got[2] != items[2] {
		t.Error("items up to and including current must not move")
	}
	if q.Index() != 2 || q.Current() != items[2] {
		t.Error("current item/index must not change")
	}
	if !sameSet(got, items) {
		t.Error("shuffle must keep the same set of items")
	}
	// 17 items shuffled: probability of identity order is 1/17!; treat as impossible.
	same := true
	for i := 3; i < len(items); i++ {
		if got[i] != items[i] {
			same = false
		}
	}
	if same {
		t.Error("upcoming items were not shuffled")
	}
}

func TestSetMode_RandomTwiceDoesNotReshuffle(t *testing.T) {
	q := NewQueue()
	fill(q, 20)
	q.SetMode(ModeRandom)
	before := ids(q)
	q.SetMode(ModeRandom)
	after := ids(q)
	for i := range before {
		if before[i] != after[i] {
			t.Fatal("SetMode(random) while already random must not reshuffle")
		}
	}
}

func TestSetMode_LeavingRandomKeepsOrder(t *testing.T) {
	q := NewQueue()
	fill(q, 20)
	q.SetMode(ModeRandom)
	before := ids(q)
	q.SetMode(ModeOneShot)
	after := ids(q)
	for i := range before {
		if before[i] != after[i] {
			t.Fatal("leaving random must not change the order")
		}
	}
}

func TestRandomize_KeepsCurrentAndBefore(t *testing.T) {
	q := NewQueue()
	items := fill(q, 20)
	q.Next()
	q.Randomize()
	got := q.Items()
	if got[0] != items[0] || got[1] != items[1] || q.Index() != 1 {
		t.Error("Randomize must not move current or earlier items")
	}
	if !sameSet(got, items) {
		t.Error("Randomize must keep the same items")
	}
}

func TestRandomize_NoOpWhenNothingAfterCurrent(t *testing.T) {
	q := NewQueue()
	q.Randomize() // empty
	fill(q, 1)
	q.Randomize() // only the current item
	if q.Len() != 1 {
		t.Error("Randomize changed the queue size")
	}
}

func TestNext_Random_PlaysEveryItemOncePerLap(t *testing.T) {
	q := NewQueue()
	items := fill(q, 10)
	q.SetMode(ModeRandom)
	seen := map[audio.MediaItem]int{q.Current(): 1}
	for i := 0; i < 9; i++ {
		if !q.Next() {
			t.Fatal("Next returned false mid-lap")
		}
		seen[q.Current()]++
	}
	for _, it := range items {
		if seen[it] != 1 {
			t.Errorf("item %v played %d times in one lap, want 1", it, seen[it])
		}
	}
}

func TestNext_Random_ReshufflesAtEndAndKeepsGoing(t *testing.T) {
	q := NewQueue()
	items := fill(q, 8)
	q.SetMode(ModeRandom)
	for i := 0; i < 7; i++ {
		q.Next()
	}
	if q.Index() != 7 {
		t.Fatalf("index = %d, want 7 (last item)", q.Index())
	}
	if !q.Next() {
		t.Fatal("random must keep playing after the last item")
	}
	if q.Index() != 0 || q.Current() == nil {
		t.Errorf("after wrap index = %d, current = %v", q.Index(), q.Current())
	}
	if !sameSet(q.Items(), items) {
		t.Error("reshuffle must keep the same items")
	}
}

func TestNext_Random_NeverRepeatsLastTrackFirst(t *testing.T) {
	// With 2 items the new lap's first track must always differ from the one
	// that just played; run many laps to make a lucky pass impossible.
	for trial := 0; trial < 200; trial++ {
		q := NewQueue()
		fill(q, 2)
		q.SetMode(ModeRandom)
		q.Next() // at last item
		last := q.Current()
		q.Next() // wrap + reshuffle
		if q.Current() == last {
			t.Fatalf("trial %d: same track played twice in a row across a lap", trial)
		}
	}
}

func TestNext_Random_SingleItemReplays(t *testing.T) {
	q := NewQueue()
	a := item("a")
	q.Append(a)
	q.SetMode(ModeRandom)
	if !q.Next() || q.Current() != a {
		t.Error("a one-item random queue must replay that item")
	}
}

func TestAppend_Random_LandsAfterCurrent(t *testing.T) {
	positions := map[int]bool{}
	for trial := 0; trial < 300; trial++ {
		q := NewQueue()
		items := fill(q, 5)
		q.SetMode(ModeRandom)
		q.Next() // current = index 1
		cur := q.Current()
		n := item("new")
		q.Append(n)

		if q.Current() != cur || q.Index() != 1 {
			t.Fatal("Append must not move the current item")
		}
		pos := -1
		for i, it := range q.Items() {
			if it == n {
				pos = i
			}
		}
		if pos <= q.Index() {
			t.Fatalf("new item at %d, must be after current (%d)", pos, q.Index())
		}
		positions[pos] = true
		if q.Len() != len(items)+1 {
			t.Fatal("Append changed the length wrongly")
		}
	}
	// current=1 with 4 upcoming => 5 possible slots (2..6). Expect to see several.
	if len(positions) < 3 {
		t.Errorf("only %d distinct positions seen, want random spread", len(positions))
	}
}

func TestAppend_Random_WhenIdleBecomesCurrent(t *testing.T) {
	q := NewQueue()
	q.SetMode(ModeRandom)
	a := item("a")
	q.Append(a) // empty queue
	if q.Current() != a {
		t.Error("first item appended in random mode must become current")
	}
	q.Next() // wraps (single item) -> still a
	q.SetMode(ModeOneShot)
	q.Next()
	q.Next() // past end
	q.SetMode(ModeRandom)
	b := item("b")
	q.Append(b)
	if q.Current() != b {
		t.Error("appending while idle (past-end) must make the new item current")
	}
}

func TestAppend_NonRandom_AppendsAtEnd(t *testing.T) {
	for _, m := range []Mode{ModeOneShot, ModeRepeat} {
		q := NewQueue()
		q.SetMode(m)
		items := fill(q, 4)
		if got := q.Items(); !sameSet(got, items) || got[3] != items[3] {
			t.Errorf("mode %s: Append must add at the end", m)
		}
	}
}

func TestClear_KeepsMode(t *testing.T) {
	q := NewQueue()
	q.SetMode(ModeRepeat)
	fill(q, 3)
	q.Clear()
	if q.Mode() != ModeRepeat {
		t.Errorf("Clear changed the mode to %q", q.Mode())
	}
}

// --- concurrency (run with -race) ---

func TestConcurrent_ModeShuffleNextAppend(t *testing.T) {
	q := NewQueue()
	fill(q, 10)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 300; i++ {
				switch (g + i) % 7 {
				case 0:
					q.Next()
				case 1:
					q.Append(item("x"))
				case 2:
					q.SetMode(Modes[i%len(Modes)])
				case 3:
					q.Randomize()
				case 4:
					_ = q.Mode()
					_ = q.Items()
				case 5:
					q.Current()
				case 6:
					if q.Len() > 50 {
						q.Clear()
					}
				}
			}
		}(g)
	}
	wg.Wait()
	if idx := q.Index(); idx < 0 || idx > q.Len() {
		t.Errorf("index %d out of range for len %d", idx, q.Len())
	}
}

// --- autoplay inside the queue ---

func TestNext_Autoplay_GoesIdleAtEndLikeOneShot(t *testing.T) {
	q := NewQueue()
	fill(q, 2)
	q.SetMode(ModeAutoplay)
	if !q.Next() || q.Index() != 1 {
		t.Fatal("Next should advance within the queue")
	}
	if q.Next() || q.Current() != nil || q.Index() != q.Len() {
		t.Errorf("at the end autoplay must go idle so the bot can refill: index=%d", q.Index())
	}
}

func TestSetMode_Autoplay_DoesNotShuffleOrChangeItems(t *testing.T) {
	q := NewQueue()
	fill(q, 20)
	before := ids(q)
	q.SetMode(ModeAutoplay)
	after := ids(q)
	for i := range before {
		if before[i] != after[i] {
			t.Fatal("autoplay must not reorder the queue")
		}
	}
}

func TestAppend_Autoplay_AppendsAtEnd(t *testing.T) {
	q := NewQueue()
	q.SetMode(ModeAutoplay)
	items := fill(q, 5)
	if got := q.Items(); got[4] != items[4] {
		t.Error("autoplay Append must add at the end")
	}
}

// --- ReplaceIfUnchanged ---

func TestReplaceIfUnchanged_ReplacesAndStartsAtFirst(t *testing.T) {
	q := NewQueue()
	q.SetMode(ModeAutoplay)
	fill(q, 3)
	q.Next()
	q.Next()
	q.Next() // idle
	snap := q.Snapshot()

	a, b := item("new-a"), item("new-b")
	if !q.ReplaceIfUnchanged(snap.Version, []audio.MediaItem{a, b}) {
		t.Fatal("unchanged queue: replace must succeed")
	}
	if q.Len() != 2 || q.Current() != a || q.Index() != 0 {
		t.Errorf("len=%d index=%d current=%v", q.Len(), q.Index(), q.Current())
	}
	if q.Mode() != ModeAutoplay {
		t.Error("replace must keep the mode")
	}
}

func TestReplaceIfUnchanged_RejectsStaleVersion(t *testing.T) {
	q := NewQueue()
	fill(q, 2)
	snap := q.Snapshot()
	q.Append(item("queued-meanwhile")) // user queues something during the DB query
	if q.ReplaceIfUnchanged(snap.Version, []audio.MediaItem{item("x")}) {
		t.Fatal("a queue that changed must not be replaced")
	}
	if q.Len() != 3 {
		t.Errorf("rejected replace altered the queue: len=%d", q.Len())
	}
}

func TestReplaceIfUnchanged_RejectsAfterEveryKindOfChange(t *testing.T) {
	changes := map[string]func(q *Queue){
		"append":  func(q *Queue) { q.Append(item("n")) },
		"next":    func(q *Queue) { q.Next() },
		"jump":    func(q *Queue) { _ = q.JumpTo(0) },
		"clear":   func(q *Queue) { q.Clear() },
		"mode":    func(q *Queue) { q.SetMode(ModeRepeat) },
		"reset":   func(q *Queue) { q.Reset() },
		"remove":  func(q *Queue) { _ = q.Remove(0) },
		"shuffle": func(q *Queue) { q.Randomize() },
	}
	for name, change := range changes {
		q := NewQueue()
		fill(q, 3)
		snap := q.Snapshot()
		change(q)
		if q.ReplaceIfUnchanged(snap.Version, []audio.MediaItem{item("x")}) {
			t.Errorf("%s: stale replace succeeded", name)
		}
	}
}

func TestReplaceIfUnchanged_RefusesEmptyList(t *testing.T) {
	q := NewQueue()
	fill(q, 2)
	if q.ReplaceIfUnchanged(q.Version(), nil) || q.ReplaceIfUnchanged(q.Version(), []audio.MediaItem{}) {
		t.Error("an empty replacement must be refused (it would wipe the queue)")
	}
	if q.Len() != 2 {
		t.Error("queue changed")
	}
}

func TestReplaceIfUnchanged_DoesNotAliasCallerSlice(t *testing.T) {
	q := NewQueue()
	fill(q, 1)
	src := []audio.MediaItem{item("a"), item("b")}
	q.ReplaceIfUnchanged(q.Version(), src)
	src[0] = item("mutated")
	if q.Current().(*testItem).id != "a" {
		t.Error("the queue must copy the slice it is given")
	}
}

func TestReplaceIfUnchanged_BumpsVersion(t *testing.T) {
	q := NewQueue()
	fill(q, 1)
	v := q.Version()
	q.ReplaceIfUnchanged(v, []audio.MediaItem{item("x")})
	if q.Version() == v {
		t.Error("a successful replace must bump the version")
	}
}

func TestReplaceIfUnchanged_ConcurrentOnlyOneWins(t *testing.T) {
	q := NewQueue()
	fill(q, 2)
	v := q.Version()
	var wg sync.WaitGroup
	var wins int32
	var mu sync.Mutex
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if q.ReplaceIfUnchanged(v, []audio.MediaItem{item("w")}) {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Errorf("%d replacements won with the same version, want exactly 1", wins)
	}
}
