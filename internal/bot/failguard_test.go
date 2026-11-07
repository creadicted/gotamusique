package bot

import (
	"testing"

	"github.com/konradk/gotamusique/internal/audio"
)

func TestFailGuard_AllFailedOnlyWhenEveryEntryFailed(t *testing.T) {
	var g failGuard
	a, b, c := &mockItem{"a"}, &mockItem{"b"}, &mockItem{"c"}
	all := []audio.MediaItem{a, b, c}

	if g.allFailed(all) {
		t.Fatal("nothing failed yet")
	}
	g.fail(b)
	g.fail(c)
	if g.allFailed(all) {
		t.Error("a is still untried/playable: must not be 'all failed'")
	}
	g.fail(a)
	if !g.allFailed(all) {
		t.Error("every entry failed")
	}
}

// Review finding: with playable A and failing B/C, the sequence A -> B -> C -> B
// has three consecutive-looking failures only if attempts are counted. Distinct
// entries must be counted, and a played track clears the streak.
func TestFailGuard_RetryingSameEntriesDoesNotCount(t *testing.T) {
	var g failGuard
	a, b, c := &mockItem{"a"}, &mockItem{"b"}, &mockItem{"c"}
	all := []audio.MediaItem{a, b, c}
	g.fail(b)
	g.fail(c)
	g.fail(b) // random reshuffle retried B before A came up
	g.fail(b)
	g.fail(c)
	if g.allFailed(all) {
		t.Error("repeated failures of the same entries must not exhaust a queue that still has A")
	}
}

func TestFailGuard_ResetClearsStreak(t *testing.T) {
	var g failGuard
	a := &mockItem{"a"}
	g.fail(a)
	g.reset()
	if g.allFailed([]audio.MediaItem{a}) {
		t.Error("reset must clear the streak")
	}
}

func TestFailGuard_EmptyListIsNotAllFailed(t *testing.T) {
	var g failGuard
	g.fail(&mockItem{"a"})
	if g.allFailed(nil) {
		t.Error("empty queue must not be 'all failed'")
	}
}

func TestFailGuard_ItemAddedAfterStreakIsUntried(t *testing.T) {
	var g failGuard
	a, b := &mockItem{"a"}, &mockItem{"b"}
	g.fail(a)
	if g.allFailed([]audio.MediaItem{a, b}) {
		t.Error("a newly queued item has not failed")
	}
}
