package audio

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/konradk/gotamusique/internal/config"
	"layeh.com/gumble/gumble"
)

// frameSize is the number of int16 samples per audio packet sent to gumble.
// Matches gumble.AudioDefaultFrameSize (480 samples = 10ms at 48kHz).
const frameSize = gumble.AudioDefaultFrameSize

// ErrAlreadyRunning is returned by Launch when a stream is already active.
var ErrAlreadyRunning = errors.New("audio pipeline already running")

// Pipeline manages a single ffmpeg subprocess and feeds its PCM output to Mumble.
type Pipeline struct {
	client *gumble.Client
	cfg    *config.Config
	log    *slog.Logger
	vol    VolumeHelper

	running atomic.Bool

	mu          sync.Mutex
	interruptCh chan struct{}
}

// New creates a Pipeline bound to client. Call this inside the gumble Connect
// handler — client must be non-nil and connected at the time of the call.
func New(client *gumble.Client, cfg *config.Config, log *slog.Logger) *Pipeline {
	return &Pipeline{
		client: client,
		cfg:    cfg,
		log:    log,
		vol: VolumeHelper{
			TargetVolume: cfg.Bot.Volume,
			RealVolume:   cfg.Bot.Volume,
			MaxVolume:    cfg.Bot.MaxVolume,
		},
	}
}

// Volume returns a pointer to the pipeline's VolumeHelper so callers can read
// RealVolume or call SetTargetVolume.
func (p *Pipeline) Volume() *VolumeHelper { return &p.vol }

// IsRunning reports whether the PCM goroutine is currently active.
func (p *Pipeline) IsRunning() bool { return p.running.Load() }

// Launch starts ffmpeg for url and begins streaming PCM frames to Mumble.
// onEnd is called exactly once when the goroutine exits: nil for a clean stop
// (stream ended or Interrupt), non-nil for unexpected errors.
// Returns ErrAlreadyRunning if a stream is already in progress.
func (p *Pipeline) Launch(url string, onEnd func(error)) error {
	if p.running.Swap(true) {
		return ErrAlreadyRunning
	}

	interruptCh := make(chan struct{})
	p.mu.Lock()
	p.interruptCh = interruptCh
	p.mu.Unlock()

	verbosity := "warning"
	if p.cfg.Debug.Ffmpeg {
		verbosity = "debug"
	}

	cmd := exec.Command("ffmpeg",
		"-v", verbosity,
		"-nostdin",
		"-i", url,
		"-ac", "1",
		"-f", "s16le",
		"-ar", "48000",
		"-",
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		p.running.Store(false)
		return fmt.Errorf("ffmpeg stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		p.running.Store(false)
		return fmt.Errorf("starting ffmpeg: %w", err)
	}

	p.log.Debug("ffmpeg started", slog.String("url", url))
	go p.loop(cmd, stdout, interruptCh, onEnd)
	return nil
}

// Interrupt signals the PCM goroutine to apply a fade-out and stop. Safe to
// call from any goroutine. No-op if the pipeline is not running.
func (p *Pipeline) Interrupt() {
	p.mu.Lock()
	ch := p.interruptCh
	p.mu.Unlock()

	if ch == nil {
		return
	}
	select {
	case <-ch: // already closed
	default:
		close(ch)
	}
}

func (p *Pipeline) loop(cmd *exec.Cmd, stdout io.ReadCloser, interruptCh <-chan struct{}, onEnd func(error)) {
	defer func() {
		stdout.Close()
		cmd.Wait() //nolint:errcheck
		p.running.Store(false)
	}()

	audioCh := p.client.AudioOutgoing()
	defer close(audioCh)

	buf := make([]byte, frameSize*2) // 2 bytes per int16 sample
	lastTime := time.Now()

	ticker := time.NewTicker(gumble.AudioDefaultInterval)
	defer ticker.Stop()

	for frameIdx := 0; ; frameIdx++ {
		// Block until the next 10 ms tick or an interrupt signal. Pacing here
		// prevents ffmpeg output bursts from flooding gumble with frames.
		select {
		case <-interruptCh:
			p.doFadeOut(cmd, stdout, audioCh, buf)
			onEnd(nil)
			return
		case <-ticker.C:
		}

		now := time.Now()
		delta := now.Sub(lastTime)
		lastTime = now

		p.vol.Cycle(delta)

		if _, err := io.ReadFull(stdout, buf); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				onEnd(nil)
			} else {
				onEnd(err)
			}
			return
		}

		samples := decodePCM(buf)

		scalar := p.vol.RealVolume * fadeInMultiplier(frameIdx)
		applyScalar(samples, scalar)

		audioCh <- samples
	}
}

// doFadeOut reads up to fadeDuration more frames from stdout, applies the
// fade-out envelope, sends them to audioCh, then kills the ffmpeg process.
func (p *Pipeline) doFadeOut(cmd *exec.Cmd, stdout io.ReadCloser, audioCh chan<- gumble.AudioBuffer, buf []byte) {
	for i := 0; i < fadeDuration; i++ {
		if _, err := io.ReadFull(stdout, buf); err != nil {
			break
		}
		samples := decodePCM(buf)
		applyScalar(samples, p.vol.RealVolume*fadeOutMultiplier(i))
		audioCh <- samples
	}
	cmd.Process.Kill() //nolint:errcheck
	// Closing stdout unblocks any pending write in ffmpeg, allowing cmd.Wait to complete.
	stdout.Close()
}

// decodePCM converts a buffer of int16 little-endian bytes into a gumble AudioBuffer.
func decodePCM(buf []byte) gumble.AudioBuffer {
	samples := make(gumble.AudioBuffer, len(buf)/2)
	for i := range samples {
		samples[i] = int16(binary.LittleEndian.Uint16(buf[i*2:]))
	}
	return samples
}
