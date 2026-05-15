package queue

import (
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
