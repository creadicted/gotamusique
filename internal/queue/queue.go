package queue

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"sync"

	"github.com/konradk/gotamusique/internal/audio"
)

// Queue is a thread-safe, index-based media queue.
// currentIndex == len(items) is the "past-end / idle" sentinel.
//
// The playback mode decides what Next does at the end of the queue. Random mode
// works by shuffling the item list itself, so the list always shows the real
// play order and Next stays sequential in every mode. The zero value behaves as
// ModeOneShot.
type Queue struct {
	mu           sync.RWMutex
	items        []audio.MediaItem
	currentIndex int
	mode         Mode
}

func NewQueue() *Queue { return &Queue{mode: ModeOneShot} }

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
	out := make([]audio.MediaItem, n)
	for i, p := range perm {
		out[i] = q.items[p]
	}
	q.items = out
}

// Append adds item to the queue. In random mode the item lands at a random
// position after the current one (never before it); when the queue is idle it
// is simply appended and becomes the current item.
func (q *Queue) Append(item audio.MediaItem) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.mode == ModeRandom && q.currentIndex < len(q.items) {
		pos := q.currentIndex + 1 + rand.IntN(len(q.items)-q.currentIndex)
		q.items = slices.Insert(q.items, pos, item)
		return
	}
	q.items = append(q.items, item)
}

// Insert adds item at index, shifting later items right.
// If index <= currentIndex (and the queue is not already past-end), currentIndex
// is incremented to keep pointing at the same item.
func (q *Queue) Insert(index int, item audio.MediaItem) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if index < 0 || index > len(q.items) {
		return fmt.Errorf("insert index %d out of range [0, %d]", index, len(q.items))
	}
	q.items = append(q.items, nil)
	copy(q.items[index+1:], q.items[index:])
	q.items[index] = item
	// Adjust only when not already in past-end state; past-end stays past-end.
	if index <= q.currentIndex && q.currentIndex < len(q.items)-1 {
		q.currentIndex++
	}
	return nil
}

// Remove deletes the item at index, shifting later items left.
// If index < currentIndex, currentIndex is decremented to keep pointing at the
// same item. Removing the current item leaves currentIndex pointing at the next one.
func (q *Queue) Remove(index int) error {
	q.mu.Lock()
	defer q.mu.Unlock()
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
	return q.items[q.currentIndex]
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

// FinishIfCurrent moves the queue to the past-end (idle) position, but only if
// item is still the current item. It reports whether it did. The check and the
// move are atomic, so a concurrent skip/jump/clear that changed the selection
// is never overwritten. Unlike Reset it keeps playback idle: the queue does not
// restart from index 0 until something new is queued or played.
func (q *Queue) FinishIfCurrent(item audio.MediaItem) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.currentIndex >= len(q.items) || q.items[q.currentIndex] != item {
		return false
	}
	q.currentIndex = len(q.items)
	return true
}

// JumpTo sets currentIndex to index. Returns an error if index is out of range.
func (q *Queue) JumpTo(index int) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if index < 0 || index >= len(q.items) {
		return fmt.Errorf("index %d out of range [0, %d)", index, len(q.items))
	}
	q.currentIndex = index
	return nil
}

// Reset sets currentIndex to 0 without clearing the items.
func (q *Queue) Reset() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.currentIndex = 0
}

// Clear empties the queue and resets currentIndex to 0.
func (q *Queue) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()
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
	copy(cp, q.items)
	return cp
}
