package audio

import (
	"errors"
	"log/slog"
	"os/exec"
	"testing"
	"time"

	"github.com/konradk/gotamusique/internal/config"
	"layeh.com/gumble/gumble"
)

// newTestPipeline builds a Pipeline whose audio output is a drained channel, so
// loop can run without a Mumble connection.
func newTestPipeline() (*Pipeline, chan gumble.AudioBuffer) {
	cfg := &config.Config{}
	cfg.Bot.Volume, cfg.Bot.MaxVolume = 0.8, 1.0
	p := New(nil, cfg, slog.New(slog.DiscardHandler))
	out := make(chan gumble.AudioBuffer, 64)
	go func() {
		for range out {
		}
	}()
	p.outgoing = func() chan<- gumble.AudioBuffer { return out }
	return p, out
}

// runLoop starts p.loop on `sh -c script` and returns the error passed to onEnd.
func runLoop(t *testing.T, script string) error {
	t.Helper()
	p, _ := newTestPipeline()
	cmd := exec.Command("sh", "-c", script)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	p.running.Store(true)
	res := make(chan error, 1)
	go p.loop(cmd, stdout, make(chan struct{}), func(e error) { res <- e })
	select {
	case e := <-res:
		return e
	case <-time.After(5 * time.Second):
		t.Fatal("loop did not finish")
		return nil
	}
}

// Review comment 1: ffmpeg that cannot open its input exits having written nothing.
// That must surface as ErrNoAudio, not as a clean end of track.
func TestLoop_NoOutput_ReportsErrNoAudio(t *testing.T) {
	err := runLoop(t, "exit 1")
	if !errors.Is(err, ErrNoAudio) {
		t.Errorf("onEnd err = %v, want ErrNoAudio", err)
	}
}

func TestLoop_NoOutputEvenWithExitZero_ReportsErrNoAudio(t *testing.T) {
	if err := runLoop(t, "exit 0"); !errors.Is(err, ErrNoAudio) {
		t.Errorf("onEnd err = %v, want ErrNoAudio", err)
	}
}

// A real track produces audio and then ends: that stays a clean end (nil).
func TestLoop_ProducesAudioThenEOF_IsCleanEnd(t *testing.T) {
	// 2 full frames of PCM (frameSize samples * 2 bytes each), then EOF.
	script := "head -c 1920 /dev/zero"
	if err := runLoop(t, script); err != nil {
		t.Errorf("onEnd err = %v, want nil for a stream that played", err)
	}
}

// A stream that dies mid-frame after producing audio is still a played track.
// A frame is frameSize*2 = 960 bytes, so 1000 bytes is one full frame plus 40.
func TestLoop_PartialLastFrame_IsCleanEnd(t *testing.T) {
	if err := runLoop(t, "head -c 1000 /dev/zero"); err != nil {
		t.Errorf("onEnd err = %v, want nil: audio was produced before the stream ended", err)
	}
}

// Fewer bytes than one frame means no audio was ever produced.
func TestLoop_LessThanOneFrame_ReportsErrNoAudio(t *testing.T) {
	if err := runLoop(t, "head -c 100 /dev/zero"); !errors.Is(err, ErrNoAudio) {
		t.Errorf("onEnd err = %v, want ErrNoAudio", err)
	}
}
