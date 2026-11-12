package queue

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"sync"

	"github.com/konradk/gotamusique/internal/audio"
)

// ErrNotFound is returned by the by-ID operations when no entry has that ID,
// typically because another client already removed it.
var ErrNotFound = errors.New("queue entry not found")

// entry is one queue slot. The ID is unique for the lifetime of the Queue and
// never reused, so it stays valid across inserts, removals, moves and shuffles
// (the same MediaItem can sit in several slots, e.g. after RepeatCurrent).
type entry struct {
	id   uint64
	item audio.MediaItem
}

// Queue is a thread-safe, index-based media queue whose entries also carry
// stable IDs for clients that must not act on a shifted index.
// currentIndex == len(items) is the "past-end / idle" sentinel.
//
// The playback mode decides what Next does at the end of the queue. Random mode
// works by shuffling the item list itself, so the list always shows the real
// play order and Next stays sequential in every mode. The zero value behaves as
// ModeOneShot.
type Queue struct {
	mu           sync.RWMutex
	items        []entry
	currentIndex int
	mode         Mode
	version      uint64 // bumped by every mutator; see Version
	lastID       uint64 // last entry ID handed out; IDs start at 1
}

func NewQueue() *Queue { return &Queue{mode: ModeOneShot} }

// newEntryLocked wraps item in an entry with a fresh ID.
func (q *Queue) newEntryLocked(item audio.MediaItem) entry {
	q.lastID++
	return entry{id: q.lastID, item: item}
}

// Mode returns the current playback mode.
func (q *Queue) Mode() Mode {
	q.mu.RLock()
	defer q.mu.RUnlock()
	if q.mode == "" {
		return ModeOneShot
	}
	return q.mode
}

// SetMode changes the playback mode. Entering random mode shuffles the items
// after the current one; setting random while already in random mode does not
// reshuffle (use Randomize for that). Leaving random mode keeps the shuffled
// order. The current item and index are never changed.
func (q *Queue) SetMode(m Mode) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.version++
	prev := q.mode
	q.mode = m
	if m == ModeRandom && prev != ModeRandom {
		q.shuffleAfterCurrentLocked()
	}
}

// Randomize shuffles the items after the current one. The current item and
// everything before it stay where they are.
func (q *Queue) Randomize() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.version++
	q.shuffleAfterCurrentLocked()
}

func (q *Queue) shuffleAfterCurrentLocked() {
	start := q.currentIndex + 1
	if start >= len(q.items) {
		return
	}
	rest := q.items[start:]
	rand.Shuffle(len(rest), func(i, j int) { rest[i], rest[j] = rest[j], rest[i] })
}

// reshuffleLocked shuffles the whole list. last is the index of the item that
// just finished (or -1); when possible the new first item is a different one,
// so the same track never plays twice in a row across a lap.
func (q *Queue) reshuffleLocked(last int) {
	n := len(q.items)
	perm := rand.Perm(n)
	if n > 1 && last >= 0 && perm[0] == last {
		j := 1 + rand.IntN(n-1)
		perm[0], perm[j] = perm[j], perm[0]
	}
	out := make([]entry, n)
	for i, p := range perm {
		out[i] = q.items[p]
	}
	q.items = out
}

// Position says where Add puts a new item.
type Position int

const (
	// AtEnd appends. In random mode the item lands at a random slot after the
	// current one (never before it), like Append.
	AtEnd Position = iota
	// AfterCurrent puts the item directly after the current one, so it plays next.
	// On an idle queue it is simply appended.
	AfterCurrent
)

// Append adds item to the queue. In random mode the item lands at a random
// position after the current one (never before it); when the queue is idle it
// is simply appended and becomes the current item.
func (q *Queue) Append(item audio.MediaItem) { q.Add(item, AtEnd) }

// Add puts item at pos and returns the ID of its new entry. The position is
// decided under the queue's lock, so a concurrent change cannot move it.
func (q *Queue) Add(item audio.MediaItem, pos Position) uint64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.version++
	e := q.newEntryLocked(item)
	playing := q.currentIndex < len(q.items)
	switch {
	case pos == AfterCurrent && playing:
		q.items = slices.Insert(q.items, q.currentIndex+1, e)
	case q.mode == ModeRandom && playing:
		at := q.currentIndex + 1 + rand.IntN(len(q.items)-q.currentIndex)
		q.items = slices.Insert(q.items, at, e)
	default:
		q.items = append(q.items, e)
	}
	return e.id
}

// Insert adds item at index, shifting later items right.
// If index <= currentIndex (and the queue is not already past-end), currentIndex
// is incremented to keep pointing at the same item.
func (q *Queue) Insert(index int, item audio.MediaItem) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.version++
	if index < 0 || index > len(q.items) {
		return fmt.Errorf("insert index %d out of range [0, %d]", index, len(q.items))
	}
	q.items = append(q.items, entry{})
	copy(q.items[index+1:], q.items[index:])
	q.items[index] = q.newEntryLocked(item)
	// Adjust only when not already in past-end state; past-end stays past-end.
	if index <= q.currentIndex && q.currentIndex < len(q.items)-1 {
		q.currentIndex++
	}
	return nil
}

// RepeatCurrent inserts n copies of the current item right after it, so it plays
// n more times. It returns the item, or false when nothing is current or n < 1.
func (q *Queue) RepeatCurrent(n int) (audio.MediaItem, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if n < 1 || q.currentIndex >= len(q.items) {
		return nil, false
	}
	q.version++
	item := q.items[q.currentIndex].item
	copies := make([]entry, n)
	for i := range copies {
		copies[i] = q.newEntryLocked(item)
	}
	q.items = slices.Insert(q.items, q.currentIndex+1, copies...)
	return item, true
}

// Remove deletes the item at index, shifting later items left.
// If index < currentIndex, currentIndex is decremented to keep pointing at the
// same item. Removing the current item leaves currentIndex pointing at the next one.
func (q *Queue) Remove(index int) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.version++
	if index < 0 || index >= len(q.items) {
		return fmt.Errorf("remove index %d out of range [0, %d)", index, len(q.items))
	}
	q.items = append(q.items[:index], q.items[index+1:]...)
	if index < q.currentIndex {
		q.currentIndex--
	}
	return nil
}

// Current returns the item at currentIndex, or nil when the queue is empty or
// the index is at the past-end sentinel.
func (q *Queue) Current() audio.MediaItem {
	q.mu.RLock()
	defer q.mu.RUnlock()
	if q.currentIndex >= len(q.items) {
		return nil
	}
	return q.items[q.currentIndex].item
}

// Next advances to the following item and returns true. At the end of the
// queue the mode decides:
//
//	one-shot: currentIndex becomes len(items) (the past-end sentinel) and Next
//	          returns false; further calls are no-ops that also return false.
//	repeat:   wraps to index 0 and returns true.
//	random:   reshuffles the whole list, wraps to index 0 and returns true; the
//	          first item of the new lap is not the one that just played.
func (q *Queue) Next() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.version++
	return q.nextLocked()
}

// NextIfUnchanged is Next, but only if nothing changed since version was read
// (see Snapshot). It returns false without moving when the queue changed, so a
// failure handler never advances past a track that was just selected or queued.
func (q *Queue) NextIfUnchanged(version uint64) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.version != version {
		return false
	}
	q.version++
	return q.nextLocked()
}

func (q *Queue) nextLocked() bool {
	n := len(q.items)
	if n == 0 {
		return false
	}
	if q.currentIndex < n-1 {
		q.currentIndex++
		return true
	}
	switch q.mode {
	case ModeRepeat:
		q.currentIndex = 0
		return true
	case ModeRandom:
		last := -1
		if q.currentIndex < n {
			last = q.currentIndex
		}
		q.reshuffleLocked(last)
		q.currentIndex = 0
		return true
	default:
		q.currentIndex = n
		return false
	}
}

// Version returns a counter that every mutator (Append, Insert, Remove, Next,
// JumpTo, Reset, Clear, SetMode, Randomize, FinishIfUnchanged, RemoveByID,
// JumpToID, MoveAfter) increments. Two
// reads returning the same value mean nothing changed in between. A mutator may
// also bump it on a rejected call (e.g. an out-of-range Insert); callers must
// treat a changed value as "re-evaluate", never as proof something moved.
func (q *Queue) Version() uint64 {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.version
}

// Snapshot is a consistent copy of the queue taken under one lock. IDs is
// parallel to Items.
type Snapshot struct {
	Items   []audio.MediaItem
	IDs     []uint64
	Index   int
	Version uint64
}

// IndexOf returns the position of the entry with the given ID, or -1.
func (s Snapshot) IndexOf(id uint64) int { return slices.Index(s.IDs, id) }

// Current returns the snapshot's current item, or nil when idle or empty.
func (s Snapshot) Current() audio.MediaItem {
	if s.Index < 0 || s.Index >= len(s.Items) {
		return nil
	}
	return s.Items[s.Index]
}

// Snapshot returns the items, current index and version atomically.
func (q *Queue) Snapshot() Snapshot {
	q.mu.RLock()
	defer q.mu.RUnlock()
	items := make([]audio.MediaItem, len(q.items))
	ids := make([]uint64, len(q.items))
	for i, e := range q.items {
		items[i], ids[i] = e.item, e.id
	}
	return Snapshot{Items: items, IDs: ids, Index: q.currentIndex, Version: q.version}
}

// FinishIfUnchanged moves the queue to the past-end (idle) position, but only if
// nothing has changed since version was read (see Snapshot). The check and the
// move are atomic, so it never overwrites a newer selection or skips an item
// appended in the meantime, even when the current item is the same one (a
// re-selected item, or a one-item repeat/random queue that wrapped). It reports
// whether it went idle. Unlike Reset, the items are kept and the queue does not
// restart from index 0 until something new is queued or played.
func (q *Queue) FinishIfUnchanged(version uint64) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.version != version || q.currentIndex >= len(q.items) {
		return false
	}
	q.currentIndex = len(q.items)
	q.version++
	return true
}

// ReplaceIfUnchanged replaces the whole item list with items and makes the first
// one current, but only if nothing changed since version was read (see
// Snapshot). It reports whether it replaced. The check and the swap are atomic,
// so a track queued, a mode change or a control that landed while the caller was
// building items (e.g. querying the database) is never overwritten. The mode is
// kept. An empty items slice is refused.
func (q *Queue) ReplaceIfUnchanged(version uint64, items []audio.MediaItem) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.version != version || len(items) == 0 {
		return false
	}
	q.items = make([]entry, len(items))
	for i, it := range items {
		q.items[i] = q.newEntryLocked(it)
	}
	q.currentIndex = 0
	q.version++
	return true
}

// JumpTo sets currentIndex to index. Returns an error if index is out of range.
func (q *Queue) JumpTo(index int) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.version++
	if index < 0 || index >= len(q.items) {
		return fmt.Errorf("index %d out of range [0, %d)", index, len(q.items))
	}
	q.currentIndex = index
	return nil
}

// JumpToLast makes the final item current under one lock and returns its index, so a
// concurrent enqueue cannot change which item is last between the length check and the
// jump. False when the queue is empty.
func (q *Queue) JumpToLast() (int, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return 0, false
	}
	q.version++
	q.currentIndex = len(q.items) - 1
	return q.currentIndex, true
}

// Reset sets currentIndex to 0 without clearing the items.
func (q *Queue) Reset() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.version++
	q.currentIndex = 0
}

// Clear empties the queue and resets currentIndex to 0.
func (q *Queue) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.version++
	q.items = nil
	q.currentIndex = 0
}

// Index returns the current index value.
func (q *Queue) Index() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.currentIndex
}

// Len returns the number of items in the queue.
func (q *Queue) Len() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.items)
}

// Items returns a snapshot copy of the queue slice, safe to iterate without holding a lock.
func (q *Queue) Items() []audio.MediaItem {
	q.mu.RLock()
	defer q.mu.RUnlock()
	cp := make([]audio.MediaItem, len(q.items))
	for i, e := range q.items {
		cp[i] = e.item
	}
	return cp
}

func (q *Queue) indexOfLocked(id uint64) int {
	return slices.IndexFunc(q.items, func(e entry) bool { return e.id == id })
}

// RemoveByID deletes the entry with the given ID and keeps the current item
// pointing at the same entry (as Remove does); removing the current entry leaves
// the next one current. wasCurrent says whether the removed entry was the current
// one, decided under the same lock so the caller knows whether the playing track
// is gone. It returns ErrNotFound, without touching the queue or its version,
// when the entry is already gone.
func (q *Queue) RemoveByID(id uint64) (wasCurrent bool, err error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	i := q.indexOfLocked(id)
	if i < 0 {
		return false, ErrNotFound
	}
	wasCurrent = i == q.currentIndex
	q.version++
	q.items = slices.Delete(q.items, i, i+1)
	if i < q.currentIndex {
		q.currentIndex--
	}
	return wasCurrent, nil
}

// JumpToID makes the entry with the given ID current. It returns its index, or
// ErrNotFound (queue and version untouched) when it is gone.
func (q *Queue) JumpToID(id uint64) (int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	i := q.indexOfLocked(id)
	if i < 0 {
		return 0, ErrNotFound
	}
	q.version++
	q.currentIndex = i
	return i, nil
}

// MoveAfter moves the entry id to sit directly after the entry afterID; an
// afterID of 0 moves it to the front. The current entry stays current, whichever
// slot it ends up in, and a queue that is past its end stays past its end. Moving
// an entry after itself is a no-op that succeeds. It returns ErrNotFound (nothing
// changed) when either entry is missing, so a stale request never reorders the
// wrong items.
func (q *Queue) MoveAfter(id, afterID uint64) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	from := q.indexOfLocked(id)
	if from < 0 {
		return ErrNotFound
	}
	if afterID != 0 && q.indexOfLocked(afterID) < 0 {
		return ErrNotFound
	}
	if id == afterID {
		return nil
	}
	var curID uint64
	pastEnd := q.currentIndex >= len(q.items)
	if !pastEnd {
		curID = q.items[q.currentIndex].id
	}
	moved := q.items[from]
	q.items = slices.Delete(q.items, from, from+1)
	to := 0
	if afterID != 0 {
		to = q.indexOfLocked(afterID) + 1
	}
	q.items = slices.Insert(q.items, to, moved)
	if pastEnd {
		q.currentIndex = len(q.items)
	} else {
		q.currentIndex = q.indexOfLocked(curID)
	}
	q.version++
	return nil
}
