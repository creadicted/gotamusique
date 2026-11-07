package bot

import (
	"sync"

	"github.com/konradk/gotamusique/internal/audio"
)

// failGuard remembers which queue entries failed since the last successful
// track. The play loop gives up only when every entry currently in the queue is
// in that set, i.e. a full pass produced nothing playable. Counting attempts
// instead would stop a queue that still has playable tracks: in random mode a
// reshuffle can retry the same failing entries before a playable one comes up.
//
// It is shared between the play loop and the pipeline's end-of-track callback,
// so it has its own lock.
type failGuard struct {
	mu     sync.Mutex
	failed map[audio.MediaItem]struct{}
}

// fail records item as failed in the current streak.
func (g *failGuard) fail(item audio.MediaItem) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.failed == nil {
		g.failed = make(map[audio.MediaItem]struct{})
	}
	g.failed[item] = struct{}{}
}

// reset clears the streak (a track played, or the user intervened).
func (g *failGuard) reset() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.failed = nil
}

// allFailed reports whether every item in items failed in the current streak.
// An empty list is not "all failed" (there is nothing to give up on).
func (g *failGuard) allFailed(items []audio.MediaItem) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(items) == 0 {
		return false
	}
	for _, it := range items {
		if _, ok := g.failed[it]; !ok {
			return false
		}
	}
	return true
}
