package bot

import (
	"testing"
)

// mockItem is a minimal audio.MediaItem for testing bot controls.
type mockItem struct{ id string }

func (m *mockItem) StreamURL() string   { return "http://example.com/" + m.id }
func (m *mockItem) FormatTitle() string { return "[Radio] " + m.id }

// --- Enqueue / QueueItems / QueueCurrentIndex / CurrentItem ---

func TestEnqueue_AddsToQueue(t *testing.T) {
	b, err := New(defaultCfg())
	if err != nil {
		t.Fatal(err)
	}
	item := &mockItem{"a"}
	b.Enqueue(item)
	items := b.QueueItems()
	if len(items) != 1 || items[0] != item {
		t.Errorf("QueueItems = %v, want [a]", items)
	}
}

func TestEnqueue_CurrentItemSet(t *testing.T) {
	b, err := New(defaultCfg())
	if err != nil {
		t.Fatal(err)
	}
	item := &mockItem{"a"}
	b.Enqueue(item)
	if got := b.CurrentItem(); got != item {
		t.Errorf("CurrentItem = %v, want %v", got, item)
	}
}

func TestQueueCurrentIndex_InitiallyZero(t *testing.T) {
	b, err := New(defaultCfg())
	if err != nil {
		t.Fatal(err)
	}
	if idx := b.QueueCurrentIndex(); idx != 0 {
		t.Errorf("QueueCurrentIndex = %d, want 0", idx)
	}
}

func TestCurrentItem_EmptyQueue_Nil(t *testing.T) {
	b, err := New(defaultCfg())
	if err != nil {
		t.Fatal(err)
	}
	if got := b.CurrentItem(); got != nil {
		t.Errorf("CurrentItem on empty queue = %v, want nil", got)
	}
}

// --- Config ---

func TestConfig_ReturnsCfg(t *testing.T) {
	cfg := defaultCfg()
	b, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if b.Config() != cfg {
		t.Error("Config() should return the same *Config passed to New")
	}
}

// --- TargetVolume ---

func TestTargetVolume_NoAudio_ReturnsCfgVolume(t *testing.T) {
	cfg := defaultCfg()
	cfg.Bot.Volume = 0.65
	b, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got := b.TargetVolume(); got != 0.65 {
		t.Errorf("TargetVolume (no audio) = %v, want 0.65", got)
	}
}

// --- SetVolume ---

func TestSetVolume_UpdatesCfgVolume(t *testing.T) {
	cfg := defaultCfg()
	cfg.Bot.MaxVolume = 1.0
	b, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	b.SetVolume(50)
	if got := b.TargetVolume(); got != 0.5 {
		t.Errorf("TargetVolume after SetVolume(50) = %v, want 0.5", got)
	}
}

func TestSetVolume_ClampsToMaxVolume(t *testing.T) {
	cfg := defaultCfg()
	cfg.Bot.MaxVolume = 0.8
	b, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	b.SetVolume(100) // 1.0 > MaxVolume 0.8
	if got := b.TargetVolume(); got > 0.8+1e-9 {
		t.Errorf("TargetVolume after SetVolume(100) with max=0.8: %v, want <= 0.8", got)
	}
}

func TestSetVolume_ClearsMutedState(t *testing.T) {
	b, err := New(defaultCfg())
	if err != nil {
		t.Fatal(err)
	}
	b.isMuted = true
	b.SetVolume(60)
	if b.IsMuted() {
		t.Error("SetVolume should clear the muted state")
	}
}

func TestSetVolume_Zero(t *testing.T) {
	b, err := New(defaultCfg())
	if err != nil {
		t.Fatal(err)
	}
	b.SetVolume(0)
	if got := b.TargetVolume(); got != 0.0 {
		t.Errorf("TargetVolume after SetVolume(0) = %v, want 0.0", got)
	}
}

// --- Mute / Unmute ---

func TestMute_SetsIsMuted(t *testing.T) {
	b, err := New(defaultCfg())
	if err != nil {
		t.Fatal(err)
	}
	b.Mute()
	if !b.IsMuted() {
		t.Error("IsMuted should be true after Mute()")
	}
}

func TestUnmute_ClearsIsMuted(t *testing.T) {
	b, err := New(defaultCfg())
	if err != nil {
		t.Fatal(err)
	}
	b.Mute()
	b.Unmute()
	if b.IsMuted() {
		t.Error("IsMuted should be false after Unmute()")
	}
}

func TestMute_Idempotent(t *testing.T) {
	b, err := New(defaultCfg())
	if err != nil {
		t.Fatal(err)
	}
	b.Mute()
	b.Mute() // second call should not panic or corrupt state
	if !b.IsMuted() {
		t.Error("should still be muted after double Mute()")
	}
}

func TestUnmute_WithoutMute_NoOp(t *testing.T) {
	b, err := New(defaultCfg())
	if err != nil {
		t.Fatal(err)
	}
	b.Unmute() // should not panic
	if b.IsMuted() {
		t.Error("IsMuted should be false when Unmute called without prior Mute")
	}
}

// --- Stop / Clear ---

func TestStop_ResetsQueue(t *testing.T) {
	b, err := New(defaultCfg())
	if err != nil {
		t.Fatal(err)
	}
	b.Enqueue(&mockItem{"a"})
	b.Enqueue(&mockItem{"b"})
	b.Stop()
	// Stop calls queue.Reset() (keeps items but resets index).
	if b.QueueCurrentIndex() != 0 {
		t.Errorf("QueueCurrentIndex after Stop = %d, want 0", b.QueueCurrentIndex())
	}
}

func TestClear_EmptiesQueue(t *testing.T) {
	b, err := New(defaultCfg())
	if err != nil {
		t.Fatal(err)
	}
	b.Enqueue(&mockItem{"a"})
	b.Clear()
	if n := len(b.QueueItems()); n != 0 {
		t.Errorf("QueueItems after Clear: len = %d, want 0", n)
	}
}

// --- Skip ---

func TestSkip_NoAudio_AdvancesQueue(t *testing.T) {
	b, err := New(defaultCfg())
	if err != nil {
		t.Fatal(err)
	}
	b.Enqueue(&mockItem{"a"})
	b.Enqueue(&mockItem{"b"})
	// No audio running, so Skip advances queue directly.
	b.Skip()
	if idx := b.QueueCurrentIndex(); idx != 1 {
		t.Errorf("QueueCurrentIndex after Skip = %d, want 1", idx)
	}
}
