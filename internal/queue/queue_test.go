package queue

import (
	"errors"
	"fmt"
	"sync"
	"testing"
)

// testItem is a minimal audio.MediaItem for testing.
type testItem struct{ id string }

func (t *testItem) StreamURL() string   { return "http://example.com/" + t.id }
func (t *testItem) FormatTitle() string { return "[Radio] " + t.id }

func item(id string) *testItem { return &testItem{id: id} }

// --- Append / Current / Len ---

func TestAppend_Current(t *testing.T) {
	q := NewQueue()
	if q.Current() != nil {
		t.Fatal("empty queue: Current should be nil")
	}
	q.Append(item("A"))
	if got := q.Current(); got == nil || got.FormatTitle() != "[Radio] A" {
		t.Errorf("Current after Append = %v", got)
	}
}

func TestLen(t *testing.T) {
	q := NewQueue()
	if q.Len() != 0 {
		t.Errorf("Len = %d, want 0", q.Len())
	}
	q.Append(item("A"))
	q.Append(item("B"))
	if q.Len() != 2 {
		t.Errorf("Len = %d, want 2", q.Len())
	}
}

func TestItems_snapshot(t *testing.T) {
	q := NewQueue()
	q.Append(item("A"))
	q.Append(item("B"))
	snap := q.Items()
	if len(snap) != 2 {
		t.Fatalf("Items len = %d, want 2", len(snap))
	}
	// Mutating the snapshot must not affect the queue.
	snap[0] = item("X")
	if q.Items()[0].FormatTitle() != "[Radio] A" {
		t.Error("snapshot mutation affected the queue")
	}
}

// --- Next ---

func TestNext_advancesIndex(t *testing.T) {
	q := NewQueue()
	q.Append(item("A"))
	q.Append(item("B"))
	if !q.Next() {
		t.Fatal("Next returned false before end")
	}
	if got := q.Current(); got == nil || got.FormatTitle() != "[Radio] B" {
		t.Errorf("Current after Next = %v", got)
	}
}

func TestNext_atLastReturnsFalse(t *testing.T) {
	q := NewQueue()
	q.Append(item("A"))
	if q.Next() {
		t.Fatal("Next should return false at last item")
	}
	if q.Current() != nil {
		t.Error("Current should be nil after exhausting single-item queue")
	}
}

func TestNext_pastEndIdempotent(t *testing.T) {
	q := NewQueue()
	q.Append(item("A"))
	q.Next()
	if q.Next() {
		t.Error("repeated Next past end should return false")
	}
	if q.Current() != nil {
		t.Error("Current should remain nil")
	}
}

func TestNext_emptyQueue(t *testing.T) {
	q := NewQueue()
	if q.Next() {
		t.Error("Next on empty queue should return false")
	}
	if q.Current() != nil {
		t.Error("Current should be nil on empty queue")
	}
}

func TestNext_multipleItems(t *testing.T) {
	q := NewQueue()
	q.Append(item("A"))
	q.Append(item("B"))
	q.Append(item("C"))
	q.Next()
	q.Next()
	if got := q.Current(); got == nil || got.FormatTitle() != "[Radio] C" {
		t.Errorf("Current = %v, want C", got)
	}
	if q.Next() {
		t.Error("Next at last item should return false")
	}
	if q.Current() != nil {
		t.Error("Current should be nil after exhausting queue")
	}
}

// --- Append resumes exhausted queue ---

func TestAppend_resumesExhaustedQueue(t *testing.T) {
	q := NewQueue()
	q.Append(item("A"))
	q.Next() // exhaust
	if q.Current() != nil {
		t.Fatal("expected nil after exhausting")
	}
	q.Append(item("B"))
	if got := q.Current(); got == nil || got.FormatTitle() != "[Radio] B" {
		t.Errorf("Append should resume exhausted queue; Current = %v", got)
	}
}

// --- JumpTo ---

func TestJumpTo_valid(t *testing.T) {
	q := NewQueue()
	q.Append(item("A"))
	q.Append(item("B"))
	q.Append(item("C"))
	if err := q.JumpTo(2); err != nil {
		t.Fatalf("JumpTo: %v", err)
	}
	if got := q.Current(); got == nil || got.FormatTitle() != "[Radio] C" {
		t.Errorf("Current after JumpTo(2) = %v", got)
	}
}

func TestJumpTo_outOfRange(t *testing.T) {
	q := NewQueue()
	q.Append(item("A"))
	if err := q.JumpTo(1); err == nil {
		t.Error("JumpTo(1) on 1-item queue should return error")
	}
	if err := q.JumpTo(-1); err == nil {
		t.Error("JumpTo(-1) should return error")
	}
}

func TestJumpTo_emptyQueue(t *testing.T) {
	q := NewQueue()
	if err := q.JumpTo(0); err == nil {
		t.Error("JumpTo on empty queue should return error")
	}
}

// --- Insert ---

func TestInsert_middle(t *testing.T) {
	q := NewQueue()
	q.Append(item("A"))
	q.Append(item("C"))
	if err := q.Insert(1, item("B")); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	snap := q.Items()
	titles := make([]string, len(snap))
	for i, it := range snap {
		titles[i] = it.FormatTitle()
	}
	want := []string{"[Radio] A", "[Radio] B", "[Radio] C"}
	for i, w := range want {
		if titles[i] != w {
			t.Errorf("items[%d] = %q, want %q", i, titles[i], w)
		}
	}
}

func TestInsert_beginning(t *testing.T) {
	q := NewQueue()
	q.Append(item("B"))
	// B is current at index 0. Insert A before it.
	if err := q.Insert(0, item("A")); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	// B should remain current (currentIndex adjusted to 1); A is queued before it.
	if got := q.Current(); got == nil || got.FormatTitle() != "[Radio] B" {
		t.Errorf("Current = %v, want B (current item should not change)", got)
	}
	if q.Index() != 1 {
		t.Errorf("Index = %d, want 1", q.Index())
	}
	if got := q.Items()[0].FormatTitle(); got != "[Radio] A" {
		t.Errorf("items[0] = %q, want A", got)
	}
}

func TestInsert_end(t *testing.T) {
	q := NewQueue()
	q.Append(item("A"))
	if err := q.Insert(1, item("B")); err != nil {
		t.Fatalf("Insert at end: %v", err)
	}
	if q.Len() != 2 {
		t.Errorf("Len = %d, want 2", q.Len())
	}
}

func TestInsert_outOfRange(t *testing.T) {
	q := NewQueue()
	q.Append(item("A"))
	if err := q.Insert(2, item("X")); err == nil {
		t.Error("Insert out of range should return error")
	}
	if err := q.Insert(-1, item("X")); err == nil {
		t.Error("Insert negative index should return error")
	}
}

func TestInsert_adjustsCurrentIndex(t *testing.T) {
	q := NewQueue()
	q.Append(item("A"))
	q.Append(item("B"))
	q.Next() // currentIndex = 1 (B is current)

	// Insert before current → currentIndex should increment.
	if err := q.Insert(0, item("Z")); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if got := q.Current(); got == nil || got.FormatTitle() != "[Radio] B" {
		t.Errorf("Current after Insert before: %v", got)
	}
	if q.Index() != 2 {
		t.Errorf("Index = %d, want 2", q.Index())
	}
}

func TestInsert_doesNotAdjustWhenAfterCurrent(t *testing.T) {
	q := NewQueue()
	q.Append(item("A"))
	q.Append(item("B"))
	// currentIndex = 0 (A is current). Insert after current.
	if err := q.Insert(1, item("Z")); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if q.Index() != 0 {
		t.Errorf("Index = %d, want 0 (no adjustment)", q.Index())
	}
}

// --- Remove ---

func TestRemove_middle(t *testing.T) {
	q := NewQueue()
	q.Append(item("A"))
	q.Append(item("B"))
	q.Append(item("C"))
	if err := q.Remove(1); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if q.Len() != 2 {
		t.Errorf("Len = %d, want 2", q.Len())
	}
	if got := q.Items()[1].FormatTitle(); got != "[Radio] C" {
		t.Errorf("items[1] = %q, want C", got)
	}
}

func TestRemove_outOfRange(t *testing.T) {
	q := NewQueue()
	q.Append(item("A"))
	if err := q.Remove(1); err == nil {
		t.Error("Remove(1) on 1-item queue should return error")
	}
	if err := q.Remove(-1); err == nil {
		t.Error("Remove(-1) should return error")
	}
}

func TestRemove_adjustsCurrentIndex(t *testing.T) {
	q := NewQueue()
	q.Append(item("A"))
	q.Append(item("B"))
	q.Append(item("C"))
	q.Next()
	q.Next() // currentIndex = 2 (C is current)

	// Remove item before current → currentIndex should decrement.
	if err := q.Remove(0); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if got := q.Current(); got == nil || got.FormatTitle() != "[Radio] C" {
		t.Errorf("Current after Remove before: %v", got)
	}
	if q.Index() != 1 {
		t.Errorf("Index = %d, want 1", q.Index())
	}
}

func TestRemove_doesNotAdjustWhenAfterCurrent(t *testing.T) {
	q := NewQueue()
	q.Append(item("A"))
	q.Append(item("B"))
	// currentIndex = 0. Remove item after current.
	if err := q.Remove(1); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if q.Index() != 0 {
		t.Errorf("Index = %d, want 0", q.Index())
	}
}

// --- Reset / Clear ---

func TestReset(t *testing.T) {
	q := NewQueue()
	q.Append(item("A"))
	q.Append(item("B"))
	q.Next() // currentIndex = 1
	q.Reset()
	if q.Index() != 0 {
		t.Errorf("Index after Reset = %d, want 0", q.Index())
	}
	if q.Len() != 2 {
		t.Error("Reset should not remove items")
	}
}

func TestClear(t *testing.T) {
	q := NewQueue()
	q.Append(item("A"))
	q.Append(item("B"))
	q.Next()
	q.Clear()
	if q.Len() != 0 {
		t.Errorf("Len after Clear = %d, want 0", q.Len())
	}
	if q.Index() != 0 {
		t.Errorf("Index after Clear = %d, want 0", q.Index())
	}
	if q.Current() != nil {
		t.Error("Current after Clear should be nil")
	}
}

// --- concurrent safety ---

func TestConcurrentAppendAndRead(t *testing.T) {
	q := NewQueue()
	const n = 100
	var wg sync.WaitGroup

	// Concurrent appenders.
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			q.Append(item(fmt.Sprintf("item%d", i)))
		}(i)
	}

	// Concurrent readers.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = q.Len()
			_ = q.Current()
			_ = q.Items()
		}()
	}

	wg.Wait()
	if q.Len() != n {
		t.Errorf("Len = %d after %d concurrent appends", q.Len(), n)
	}
}

func TestConcurrentNextAndAppend(t *testing.T) {
	q := NewQueue()
	const n = 50
	for i := 0; i < n; i++ {
		q.Append(item(fmt.Sprintf("item%d", i)))
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			q.Next()
		}()
	}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			q.Append(item(fmt.Sprintf("extra%d", i)))
		}(i)
	}
	wg.Wait()
}

// --- Version / Snapshot / FinishIfUnchanged / NextIfUnchanged ---

func TestVersion_EveryMutatorBumps(t *testing.T) {
	mutators := map[string]func(q *Queue){
		"Append":    func(q *Queue) { q.Append(item("n")) },
		"Insert":    func(q *Queue) { _ = q.Insert(0, item("n")) },
		"Remove":    func(q *Queue) { _ = q.Remove(0) },
		"Next":      func(q *Queue) { q.Next() },
		"JumpTo":    func(q *Queue) { _ = q.JumpTo(0) },
		"Reset":     func(q *Queue) { q.Reset() },
		"Clear":     func(q *Queue) { q.Clear() },
		"SetMode":   func(q *Queue) { q.SetMode(ModeRepeat) },
		"Randomize": func(q *Queue) { q.Randomize() },
		"FinishIfUnchanged": func(q *Queue) {
			q.FinishIfUnchanged(q.Version())
		},
		"NextIfUnchanged": func(q *Queue) { q.NextIfUnchanged(q.Version()) },
		"RemoveByID":      func(q *Queue) { _, _ = q.RemoveByID(q.Snapshot().IDs[0]) },
		"JumpToID":        func(q *Queue) { _, _ = q.JumpToID(q.Snapshot().IDs[1]) },
		"MoveAfter": func(q *Queue) {
			ids := q.Snapshot().IDs
			_ = q.MoveAfter(ids[0], ids[1])
		},
	}
	for name, mutate := range mutators {
		q := NewQueue()
		q.Append(item("a"))
		q.Append(item("b"))
		before := q.Version()
		mutate(q)
		if q.Version() == before {
			t.Errorf("%s did not bump Version", name)
		}
	}
}

func TestVersion_ReadersDoNotBump(t *testing.T) {
	q := NewQueue()
	q.Append(item("a"))
	v := q.Version()
	q.Current()
	q.Items()
	q.Len()
	q.Index()
	q.Mode()
	q.Snapshot()
	if q.Version() != v {
		t.Error("a read-only method bumped Version")
	}
}

func TestSnapshot_IsConsistentCopy(t *testing.T) {
	q := NewQueue()
	a, b := item("a"), item("b")
	q.Append(a)
	q.Append(b)
	q.Next()
	snap := q.Snapshot()
	if snap.Current() != b || snap.Index != 1 || len(snap.Items) != 2 || snap.Version != q.Version() {
		t.Errorf("bad snapshot: %+v", snap)
	}
	q.Clear()
	if len(snap.Items) != 2 || snap.Current() != b {
		t.Error("snapshot must not change when the queue does")
	}
	if (Snapshot{}).Current() != nil {
		t.Error("empty snapshot Current must be nil")
	}
}

func TestFinishIfUnchanged_GoesIdleAndKeepsItems(t *testing.T) {
	q := NewQueue()
	q.Append(item("a"))
	q.Append(item("b"))
	if !q.FinishIfUnchanged(q.Snapshot().Version) {
		t.Fatal("FinishIfUnchanged(current version) = false")
	}
	if q.Current() != nil || q.Index() != q.Len() || q.Len() != 2 {
		t.Errorf("not idle with items kept: current=%v index=%d len=%d", q.Current(), q.Index(), q.Len())
	}
}

func TestFinishIfUnchanged_RejectsAfterAnyChange(t *testing.T) {
	changes := map[string]func(q *Queue, a *testItem){
		"append":          func(q *Queue, a *testItem) { q.Append(item("new")) },
		"same item again": func(q *Queue, a *testItem) { _ = q.JumpTo(0) }, // re-selects the SAME item
		"next":            func(q *Queue, a *testItem) { q.Next() },
		"clear":           func(q *Queue, a *testItem) { q.Clear() },
	}
	for name, change := range changes {
		q := NewQueue()
		a := item("a")
		q.Append(a)
		snap := q.Snapshot()
		change(q, a)
		cur := q.Current()
		if q.FinishIfUnchanged(snap.Version) {
			t.Errorf("%s: must not finish after the queue changed", name)
		}
		if q.Current() != cur {
			t.Errorf("%s: a rejected finish must not move the queue", name)
		}
	}
}

// A one-item repeat queue wraps back to the very same item; a stale finish
// comparing only the item would wrongly succeed.
func TestFinishIfUnchanged_OneItemRepeatWrap(t *testing.T) {
	q := NewQueue()
	a := item("a")
	q.Append(a)
	q.SetMode(ModeRepeat)
	snap := q.Snapshot()
	q.Next() // wraps to a again
	if q.Current() != a {
		t.Fatal("setup: expected the same item")
	}
	if q.FinishIfUnchanged(snap.Version) {
		t.Error("a wrap that lands on the same item must still invalidate the snapshot")
	}
}

func TestFinishIfUnchanged_ItemAppendedDuringFailureIsNotSkipped(t *testing.T) {
	q := NewQueue()
	a := item("a")
	q.Append(a)
	snap := q.Snapshot() // loop starts resolving a
	n := item("new")
	q.Append(n) // user queues something while a is failing
	if q.FinishIfUnchanged(snap.Version) {
		t.Fatal("must not go idle: a new item arrived")
	}
	if q.Index() == q.Len() {
		t.Error("the new item must still be reachable")
	}
}

func TestFinishIfUnchanged_EmptyAndIdle(t *testing.T) {
	q := NewQueue()
	if q.FinishIfUnchanged(q.Version()) {
		t.Error("empty queue: want false")
	}
	q.Append(item("a"))
	q.FinishIfUnchanged(q.Version())
	if q.FinishIfUnchanged(q.Version()) {
		t.Error("already idle: want false")
	}
}

func TestFinishIfUnchanged_AppendAfterwardsBecomesCurrent(t *testing.T) {
	q := NewQueue()
	q.Append(item("a"))
	q.FinishIfUnchanged(q.Version())
	n := item("new")
	q.Append(n)
	if q.Current() != n {
		t.Error("an item queued after going idle must become current")
	}
}

func TestNextIfUnchanged(t *testing.T) {
	q := NewQueue()
	a, b := item("a"), item("b")
	q.Append(a)
	q.Append(b)
	snap := q.Snapshot()
	if !q.NextIfUnchanged(snap.Version) || q.Current() != b {
		t.Fatal("unchanged queue: NextIfUnchanged must advance")
	}
	// snap is now stale
	if q.NextIfUnchanged(snap.Version) {
		t.Error("stale version: NextIfUnchanged must not advance")
	}
	if q.Current() != b {
		t.Error("stale NextIfUnchanged moved the queue")
	}
}

func TestRepeatCurrent(t *testing.T) {
	q := NewQueue()
	a, b := item("a"), item("b")
	q.Append(a)
	q.Append(b)

	got1, ok := q.RepeatCurrent(2)
	if !ok || got1 != a {
		t.Fatalf("got %v %v, want a", got1, ok)
	}
	got := q.Items()
	if len(got) != 4 || got[0] != a || got[1] != a || got[2] != a || got[3] != b {
		t.Errorf("items = %v, want a a a b", got)
	}
	if q.Index() != 0 {
		t.Errorf("index = %d, want the current item unchanged", q.Index())
	}
}

func TestRepeatCurrent_NothingCurrentOrBadCount(t *testing.T) {
	q := NewQueue()
	if _, ok := q.RepeatCurrent(1); ok {
		t.Error("empty queue has no current item")
	}
	q.Append(item("a"))
	v := q.Version()
	if _, ok := q.RepeatCurrent(0); ok || q.Version() != v || q.Len() != 1 {
		t.Error("n < 1 must change nothing")
	}
	q.Next() // past the end
	if _, ok := q.RepeatCurrent(1); ok {
		t.Error("past-end queue has no current item")
	}
}

func TestJumpToLast(t *testing.T) {
	q := NewQueue()
	if _, ok := q.JumpToLast(); ok {
		t.Error("empty queue has no last item")
	}
	q.Append(item("a"))
	q.Append(item("b"))
	q.Append(item("c"))
	v := q.Version()
	idx, ok := q.JumpToLast()
	if !ok || idx != 2 || q.Index() != 2 || q.Version() == v {
		t.Errorf("idx=%d ok=%v index=%d, want 2 true 2 and a version bump", idx, ok, q.Index())
	}
}

// A concurrent enqueue must never make JumpToLast land on a different item than the
// one that was last at the moment of the jump: the returned index is always the last
// position at that instant, so Current() equals Items()[idx] taken in the same call.
func TestJumpToLast_ConcurrentAppend(t *testing.T) {
	q := NewQueue()
	q.SetMode(ModeRandom) // random mode inserts before the end, the case that raced
	for i := 0; i < 5; i++ {
		q.Append(item(fmt.Sprint(i)))
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 5; ; i++ {
			select {
			case <-stop:
				return
			default:
				q.Append(item(fmt.Sprint(i)))
			}
		}
	}()
	for i := 0; i < 2000; i++ {
		idx, ok := q.JumpToLast()
		if !ok {
			t.Fatal("queue is not empty")
		}
		if idx < 0 || idx >= q.Len() {
			t.Fatalf("index %d out of range %d", idx, q.Len())
		}
	}
	close(stop)
	<-done
}

// --- Entry IDs ---

func titles(q *Queue) []string {
	var out []string
	for _, it := range q.Items() {
		out = append(out, it.(*testItem).id)
	}
	return out
}

func TestIDs_UniqueAndNeverReused(t *testing.T) {
	q := NewQueue()
	a := item("a")
	q.Append(a)
	q.Append(a) // same item twice -> distinct entries
	if _, ok := q.RepeatCurrent(2); !ok {
		t.Fatal("RepeatCurrent failed")
	}
	ids := q.Snapshot().IDs
	seen := map[uint64]bool{}
	for _, id := range ids {
		if id == 0 || seen[id] {
			t.Fatalf("bad or duplicate id in %v", ids)
		}
		seen[id] = true
	}
	_, _ = q.RemoveByID(ids[0])
	q.Append(item("z"))
	for _, id := range q.Snapshot().IDs {
		if id == ids[0] {
			t.Fatal("removed ID was reused")
		}
	}
}

func TestIDs_SurviveShuffleAndInsert(t *testing.T) {
	q := NewQueue()
	for _, n := range []string{"a", "b", "c", "d", "e"} {
		q.Append(item(n))
	}
	before := map[uint64]string{}
	snap := q.Snapshot()
	for i, id := range snap.IDs {
		before[id] = snap.Items[i].(*testItem).id
	}
	q.Randomize()
	_ = q.Insert(0, item("x"))
	after := q.Snapshot()
	for i, id := range after.IDs {
		if want, ok := before[id]; ok && after.Items[i].(*testItem).id != want {
			t.Errorf("id %d now points at %v, was %s", id, after.Items[i], want)
		}
	}
}

func TestRemoveByID(t *testing.T) {
	q := NewQueue()
	for _, n := range []string{"a", "b", "c"} {
		q.Append(item(n))
	}
	_ = q.JumpTo(2)
	ids := q.Snapshot().IDs

	// A non-current entry before the current one: current keeps pointing at "c".
	if was, err := q.RemoveByID(ids[0]); err != nil || was {
		t.Fatalf("remove non-current = %v, %v", was, err)
	}
	if q.Index() != 1 || q.Current().(*testItem).id != "c" {
		t.Errorf("current should still be c, got index %d", q.Index())
	}

	// The same ID again: not found, queue and version untouched.
	v, n := q.Version(), q.Len()
	if _, err := q.RemoveByID(ids[0]); !errors.Is(err, ErrNotFound) {
		t.Errorf("second remove = %v, want ErrNotFound", err)
	}
	if q.Version() != v || q.Len() != n {
		t.Error("a missing ID must not touch the queue or bump the version")
	}

	// Removing the current (last) entry reports it and leaves the queue past its end.
	if was, err := q.RemoveByID(ids[2]); err != nil || !was {
		t.Errorf("removing the current entry = %v, %v, want was=true", was, err)
	}
	if q.Index() != 1 || q.Current() != nil {
		t.Errorf("index %d current %v, want past-end 1 / nil", q.Index(), q.Current())
	}
}

func TestJumpToID(t *testing.T) {
	q := NewQueue()
	for _, n := range []string{"a", "b", "c"} {
		q.Append(item(n))
	}
	ids := q.Snapshot().IDs
	// Another client removes "a" first; the ID still finds "c".
	_, _ = q.RemoveByID(ids[0])
	i, err := q.JumpToID(ids[2])
	if err != nil || i != 1 || q.Current().(*testItem).id != "c" {
		t.Errorf("JumpToID = %d, %v, current %v", i, err, q.Current())
	}
	if _, err := q.JumpToID(ids[0]); !errors.Is(err, ErrNotFound) {
		t.Errorf("gone ID = %v, want ErrNotFound", err)
	}
}

func TestMoveAfter(t *testing.T) {
	setup := func() (*Queue, []uint64) {
		q := NewQueue()
		for _, n := range []string{"a", "b", "c", "d"} {
			q.Append(item(n))
		}
		return q, q.Snapshot().IDs
	}
	cases := []struct {
		name     string
		cur      int // JumpTo before moving
		from, to int // positions; to == -1 means front
		want     string
		wantCur  string
	}{
		{"forward", 0, 0, 2, "bcad", "a"},
		{"backward", 0, 3, 0, "adbc", "a"},
		{"to front", 2, 2, -1, "cabd", "c"},
		{"move current", 1, 1, 3, "acdb", "b"},
		{"after current keeps current", 1, 3, 1, "abdc", "b"},
	}
	for _, c := range cases {
		q, ids := setup()
		_ = q.JumpTo(c.cur)
		after := uint64(0)
		if c.to >= 0 {
			after = ids[c.to]
		}
		if err := q.MoveAfter(ids[c.from], after); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		got := ""
		for _, s := range titles(q) {
			got += s
		}
		if got != c.want || q.Current().(*testItem).id != c.wantCur {
			t.Errorf("%s: order %s current %v, want %s / %s", c.name, got, q.Current(), c.want, c.wantCur)
		}
	}
}

func TestMoveAfter_MissingAndSelfAndPastEnd(t *testing.T) {
	q := NewQueue()
	q.Append(item("a"))
	q.Append(item("b"))
	ids := q.Snapshot().IDs
	v := q.Version()
	if err := q.MoveAfter(999, ids[0]); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing id = %v", err)
	}
	if err := q.MoveAfter(ids[0], 999); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing after = %v", err)
	}
	if err := q.MoveAfter(ids[0], ids[0]); err != nil {
		t.Errorf("self move = %v", err)
	}
	if q.Version() != v {
		t.Error("rejected or no-op moves must not bump the version")
	}
	q.Next()
	q.Next() // past end
	if err := q.MoveAfter(ids[0], ids[1]); err != nil {
		t.Fatal(err)
	}
	if q.Current() != nil || q.Index() != 2 {
		t.Errorf("past-end queue must stay past end, index %d", q.Index())
	}
}

func TestSnapshot_IndexOf(t *testing.T) {
	q := NewQueue()
	q.Append(item("a"))
	q.Append(item("b"))
	s := q.Snapshot()
	if s.IndexOf(s.IDs[1]) != 1 || s.IndexOf(12345) != -1 {
		t.Error("IndexOf wrong")
	}
}
