package queue

import (
	"fmt"
	"sync"

	"github.com/konradk/gotamusique/internal/audio"
)

// Queue is a thread-safe, index-based media queue.
// currentIndex == len(items) is the "past-end / idle" sentinel.
type Queue struct {
	mu           sync.RWMutex
	items        []audio.MediaItem
	currentIndex int
}

func NewQueue() *Queue { return &Queue{} }

func (q *Queue) Append(item audio.MediaItem) {
	q.mu.Lock()
	defer q.mu.Unlock()
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

// Next advances currentIndex by one and returns true. If already at the last
// item it sets currentIndex to len(items) (the past-end sentinel) and returns
// false. Further calls while past-end are no-ops that also return false.
func (q *Queue) Next() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 || q.currentIndex >= len(q.items)-1 {
		if len(q.items) > 0 {
			q.currentIndex = len(q.items)
		}
		return false
	}
	q.currentIndex++
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
