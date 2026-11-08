package bot

import (
	"testing"
)

func pauseBot(t *testing.T, autoPause bool) *Bot {
	t.Helper()
	cfg := defaultCfg()
	cfg.Bot.AutoPauseWhenAlone = autoPause
	cfg.Bot.AutoResumeOnJoin = true
	b, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func enqueue3(b *Bot) {
	for _, id := range []string{"a", "b", "c"} {
		b.Enqueue(&mockItem{id})
	}
}

func TestPause_keepsQueueIndex(t *testing.T) {
	b := pauseBot(t, false)
	enqueue3(b)
	if err := b.Play(1); err != nil {
		t.Fatal(err)
	}
	if !b.Pause() {
		t.Fatal("Pause() = false, want true")
	}
	if got := b.QueueCurrentIndex(); got != 1 {
		t.Errorf("index after Pause = %d, want 1", got)
	}
	if !b.IsPaused() {
		t.Error("IsPaused = false after Pause")
	}
}

func TestStop_resetsIndexUnlikePause(t *testing.T) {
	b := pauseBot(t, false)
	enqueue3(b)
	b.Play(2)
	b.Stop()
	if got := b.QueueCurrentIndex(); got != 0 {
		t.Errorf("index after Stop = %d, want 0", got)
	}
}

func TestPause_bumpsLaunchVersion(t *testing.T) {
	b := pauseBot(t, false)
	enqueue3(b)
	before := b.launchVersion.Load()
	b.Pause()
	if b.launchVersion.Load() == before {
		t.Error("launchVersion unchanged: the interrupted track's onTrackEnd would advance the queue")
	}
}

func TestPause_twiceReportsNoChange(t *testing.T) {
	b := pauseBot(t, false)
	if !b.Pause() {
		t.Fatal("first Pause() = false")
	}
	if b.Pause() {
		t.Error("second Pause() = true, want false")
	}
}

func TestResume(t *testing.T) {
	b := pauseBot(t, false)
	if b.Resume() {
		t.Error("Resume() on a playing bot = true, want false")
	}
	b.Pause()
	if !b.Resume() {
		t.Error("Resume() after Pause = false, want true")
	}
	if b.IsPaused() {
		t.Error("still paused after Resume")
	}
	select {
	case <-b.wakeCh:
	default:
		t.Error("Resume did not wake the loop")
	}
}

func TestPlayAndSkipAndStopEndAPause(t *testing.T) {
	for name, act := range map[string]func(*Bot){
		"play":  func(b *Bot) { b.Play(0) },
		"skip":  func(b *Bot) { b.Skip() },
		"stop":  func(b *Bot) { b.Stop() },
		"clear": func(b *Bot) { b.Clear() },
	} {
		b := pauseBot(t, false)
		enqueue3(b)
		b.Pause()
		act(b)
		if b.IsPaused() {
			t.Errorf("%s left the bot paused", name)
		}
	}
}

func TestOccupancy_autoPauseWhenAlone(t *testing.T) {
	b := pauseBot(t, true)
	enqueue3(b)
	b.occupancyChanged(0)
	if b.pauseState.Load() != pauseAuto {
		t.Errorf("state = %d, want pauseAuto", b.pauseState.Load())
	}
	if got := b.QueueCurrentIndex(); got != 0 {
		t.Errorf("index = %d, want unchanged 0", got)
	}
}

func TestOccupancy_autoResumeOnJoin(t *testing.T) {
	b := pauseBot(t, true)
	enqueue3(b)
	b.occupancyChanged(0)
	b.occupancyChanged(1)
	if b.IsPaused() {
		t.Error("still paused after someone joined")
	}
}

func TestOccupancy_manualPauseNotAutoResumed(t *testing.T) {
	b := pauseBot(t, true)
	enqueue3(b)
	b.Pause()
	b.occupancyChanged(0) // must not downgrade the manual pause to auto
	b.occupancyChanged(1)
	if b.pauseState.Load() != pauseManual {
		t.Errorf("state = %d, want pauseManual", b.pauseState.Load())
	}
}

func TestOccupancy_autoResumeOffKeepsPause(t *testing.T) {
	b := pauseBot(t, true)
	b.cfg.Bot.AutoResumeOnJoin = false
	enqueue3(b)
	b.occupancyChanged(0)
	b.occupancyChanged(1)
	if !b.IsPaused() {
		t.Error("resumed although auto_resume_on_join = False")
	}
}

func TestOccupancy_featureOffIsNoop(t *testing.T) {
	b := pauseBot(t, false)
	enqueue3(b)
	b.occupancyChanged(0)
	if b.IsPaused() {
		t.Error("paused although auto_pause_when_alone = False")
	}
}

func TestOccupancy_emptyQueueDoesNotPause(t *testing.T) {
	b := pauseBot(t, true)
	b.occupancyChanged(0)
	if b.IsPaused() {
		t.Error("paused an idle bot")
	}
}

func TestOccupancy_unknownCountIgnored(t *testing.T) {
	b := pauseBot(t, true)
	enqueue3(b)
	b.occupancyChanged(-1)
	if b.IsPaused() {
		t.Error("paused on an unknown occupancy")
	}
}
