package bot

import (
	"errors"
	"testing"
	"time"

	fileitem "github.com/konradk/gotamusique/internal/media/file"
	"github.com/konradk/gotamusique/internal/queue"
)

func idsOf(b *Bot) []uint64 {
	var out []uint64
	for _, e := range b.Snapshot().Queue {
		out = append(out, e.ID)
	}
	return out
}

func TestPlayEntry_JumpsByIDAndInvalidates(t *testing.T) {
	b := pauseBot(t, false)
	enqueue3(b)
	id := idsOf(b)[2]
	before := b.launchVersion.Load()
	if err := b.PlayEntry(id); err != nil {
		t.Fatal(err)
	}
	if b.QueueCurrentIndex() != 2 {
		t.Errorf("index = %d, want 2", b.QueueCurrentIndex())
	}
	if b.launchVersion.Load() == before {
		t.Error("launchVersion unchanged: the old track's onTrackEnd would advance the queue")
	}
	if err := b.PlayEntry(999); !errors.Is(err, queue.ErrNotFound) {
		t.Errorf("unknown id = %v, want ErrNotFound", err)
	}
}

func TestPlayEntry_EndsPause(t *testing.T) {
	b := pauseBot(t, false)
	enqueue3(b)
	b.Pause()
	if err := b.PlayEntry(idsOf(b)[1]); err != nil {
		t.Fatal(err)
	}
	if b.IsPaused() {
		t.Error("explicitly playing an entry must end the pause")
	}
}

func TestRemoveEntry_NotCurrentLeavesPlaybackAlone(t *testing.T) {
	b := pauseBot(t, false)
	enqueue3(b)
	_ = b.Play(1)
	before := b.launchVersion.Load()
	if err := b.RemoveEntry(idsOf(b)[2]); err != nil {
		t.Fatal(err)
	}
	if b.launchVersion.Load() != before {
		t.Error("removing a queued track must not interrupt the playing one")
	}
	if b.QueueCurrentIndex() != 1 || b.CurrentItem().FormatTitle() != "[Radio] b" {
		t.Errorf("current changed: index %d", b.QueueCurrentIndex())
	}
	if err := b.RemoveEntry(idsOf(b)[0]); err != nil { // before the current one
		t.Fatal(err)
	}
	if b.CurrentItem().FormatTitle() != "[Radio] b" {
		t.Errorf("current = %v, want b", b.CurrentItem().FormatTitle())
	}
}

func TestRemoveEntry_CurrentStartsNext(t *testing.T) {
	b := pauseBot(t, false)
	enqueue3(b)
	_ = b.Play(1)
	before := b.launchVersion.Load()
	if err := b.RemoveEntry(idsOf(b)[1]); err != nil {
		t.Fatal(err)
	}
	if b.CurrentItem().FormatTitle() != "[Radio] c" {
		t.Errorf("current = %v, want c", b.CurrentItem())
	}
	if b.launchVersion.Load() == before {
		t.Error("removing the playing track must interrupt it without advancing the queue")
	}
}

func TestRemoveEntry_LastCurrentWrapsOnlyInRepeatAndRandom(t *testing.T) {
	for mode, wantWrap := range map[queue.Mode]bool{
		queue.ModeOneShot: false, queue.ModeRepeat: true, queue.ModeRandom: true,
	} {
		b := pauseBot(t, false)
		enqueue3(b)
		b.queue.SetMode(mode)
		_ = b.Play(2)
		if err := b.RemoveEntry(idsOf(b)[2]); err != nil {
			t.Fatal(err)
		}
		if got := b.CurrentItem() != nil; got != wantWrap {
			t.Errorf("%s: current present = %v, want %v", mode, got, wantWrap)
		}
	}
}

func TestRemoveEntry_Missing(t *testing.T) {
	b := pauseBot(t, false)
	enqueue3(b)
	id := idsOf(b)[0]
	if err := b.RemoveEntry(id); err != nil {
		t.Fatal(err)
	}
	if err := b.RemoveEntry(id); !errors.Is(err, queue.ErrNotFound) {
		t.Errorf("second remove = %v, want ErrNotFound", err)
	}
	if len(idsOf(b)) != 2 {
		t.Error("a stale remove must not touch other entries")
	}
}

func TestMoveEntry_KeepsPlaybackAndCurrent(t *testing.T) {
	b := pauseBot(t, false)
	enqueue3(b)
	_ = b.Play(0)
	i := idsOf(b)
	before := b.launchVersion.Load()
	if err := b.MoveEntry(i[0], i[2]); err != nil { // move the playing "a" to the end
		t.Fatal(err)
	}
	if b.launchVersion.Load() != before {
		t.Error("moving must not interrupt playback")
	}
	if b.CurrentItem().FormatTitle() != "[Radio] a" || b.QueueCurrentIndex() != 2 {
		t.Errorf("current = %v at %d, want a at 2", b.CurrentItem().FormatTitle(), b.QueueCurrentIndex())
	}
	if err := b.MoveEntry(12345, i[0]); !errors.Is(err, queue.ErrNotFound) {
		t.Errorf("unknown id = %v", err)
	}
}

func TestSeek(t *testing.T) {
	b := pauseBot(t, false)
	if err := b.Seek(time.Second); !errors.Is(err, ErrNothingPlaying) {
		t.Errorf("empty queue = %v, want ErrNothingPlaying", err)
	}

	b.Enqueue(&mockItem{"live"})
	if err := b.Seek(time.Second); !errors.Is(err, ErrNotSeekable) {
		t.Errorf("live item = %v, want ErrNotSeekable", err)
	}

	b.Clear()
	it := &seekItem{mockItem{"song"}}
	b.Enqueue(it)
	if err := b.Seek(-time.Second); !errors.Is(err, ErrSeekRange) {
		t.Errorf("negative = %v, want ErrSeekRange", err)
	}
	before := b.launchVersion.Load()
	if err := b.Seek(90 * time.Second); err != nil {
		t.Fatal(err)
	}
	if b.launchVersion.Load() == before {
		t.Error("seek must restart the current item without advancing the queue")
	}
	if at, ok := b.resume.peek(it); !ok || at != 90*time.Second {
		t.Errorf("resume point = %v, %v, want 90s", at, ok)
	}
}

func TestSeek_WhilePausedOnlyMovesResumePoint(t *testing.T) {
	b := pauseBot(t, false)
	it := &seekItem{mockItem{"song"}}
	b.Enqueue(it)
	b.Pause()
	before := b.launchVersion.Load()
	if err := b.Seek(30 * time.Second); err != nil {
		t.Fatal(err)
	}
	if b.launchVersion.Load() != before || !b.IsPaused() {
		t.Error("seeking while paused must stay paused and not restart")
	}
	if at, _ := b.resume.peek(it); at != 30*time.Second {
		t.Errorf("resume point = %v, want 30s", at)
	}
	if s := b.Snapshot(); !s.PositionKnown || s.Position != 30*time.Second {
		t.Errorf("snapshot position = %v (known %v), want 30s", s.Position, s.PositionKnown)
	}
}

func TestSeek_PastKnownDuration(t *testing.T) {
	b := pauseBot(t, false)
	fi := fileitem.New("a.mp3", t.TempDir())
	fi.Duration = 60 * time.Second
	b.Enqueue(fi)
	if err := b.Seek(60 * time.Second); !errors.Is(err, ErrSeekRange) {
		t.Errorf("at the end = %v, want ErrSeekRange", err)
	}
	if err := b.Seek(59 * time.Second); err != nil {
		t.Errorf("inside = %v", err)
	}
}

func TestSnapshot_Content(t *testing.T) {
	b := pauseBot(t, false)
	b.cfg.Bot.MaxVolume = 1
	enqueue3(b)
	b.Enqueue(&seekItem{mockItem{"s"}})
	_ = b.Play(1)
	b.SetVolume(40)

	s := b.Snapshot()
	if len(s.Queue) != 4 || s.Index != 1 || s.Mode != "one-shot" {
		t.Fatalf("snapshot = %+v", s)
	}
	if s.Queue[1].Title != "[Radio] b" || s.Queue[1].Seekable || !s.Queue[3].Seekable {
		t.Errorf("entries = %+v", s.Queue)
	}
	if s.Volume != 0.4 || s.Muted || s.Pause != "" || s.Playing || s.PositionKnown {
		t.Errorf("state = %+v", s)
	}
	b.Pause()
	if got := b.Snapshot().Pause; got != "manual" {
		t.Errorf("Pause = %q, want manual", got)
	}
	b.Resume()
	b.Mute()
	if !b.Snapshot().Muted {
		t.Error("Muted not reported")
	}
}

func TestSnapshot_VersionChangesOnlyWithState(t *testing.T) {
	b := pauseBot(t, false)
	enqueue3(b)
	v := b.Snapshot().Version
	if b.Snapshot().Version != v {
		t.Fatal("version changed between two readings of an unchanged bot")
	}
	steps := map[string]func(){
		"queue edit": func() { b.Enqueue(&mockItem{"d"}) },
		"pause":      func() { b.Pause() },
		"resume":     func() { b.Resume() },
		"volume":     func() { b.cfg.Bot.MaxVolume = 1; b.SetVolume(33) },
		"mute":       func() { b.Mute() },
		"mode":       func() { _ = b.SetPlaybackMode("repeat") },
		"jump":       func() { _ = b.Play(2) },
	}
	for _, name := range []string{"queue edit", "pause", "resume", "volume", "mute", "mode", "jump"} {
		steps[name]()
		nv := b.Snapshot().Version
		if nv == v {
			t.Errorf("%s did not change the version", name)
		}
		v = nv
	}
}
