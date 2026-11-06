package audio

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"sort"
	"strings"
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

// ErrNoAudio is passed to the Launch callback when ffmpeg ended before
// producing a single frame of audio (it could not open or decode the input, e.g.
// a file deleted after it was queued). It is a failure, not a normal end of track.
var ErrNoAudio = errors.New("ffmpeg produced no audio")

// Pipeline manages a single ffmpeg subprocess and feeds its PCM output to Mumble.
type Pipeline struct {
	client *gumble.Client
	cfg    *config.Config
	log    *slog.Logger
	vol    VolumeHelper

	running atomic.Bool

	// outgoing opens the gumble audio channel for one stream. Replaced in tests.
	outgoing func() chan<- gumble.AudioBuffer

	mu          sync.Mutex
	interruptCh chan struct{}
}

// New creates a Pipeline bound to client. Call this inside the gumble Connect
// handler — client must be non-nil and connected at the time of the call.
func New(client *gumble.Client, cfg *config.Config, log *slog.Logger) *Pipeline {
	return &Pipeline{
		client:   client,
		cfg:      cfg,
		log:      log,
		outgoing: func() chan<- gumble.AudioBuffer { return client.AudioOutgoing() },
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
// opts tunes the ffmpeg input (finite tracks, HTTP headers, cookies).
// onEnd is called exactly once when the goroutine exits: nil for a clean stop
// (stream ended or Interrupt), ErrNoAudio when ffmpeg produced no audio at all,
// and another non-nil error for unexpected read errors.
// Returns ErrAlreadyRunning if a stream is already in progress.
func (p *Pipeline) Launch(url string, opts StreamOptions, onEnd func(error)) error {
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

	cmd := exec.Command("ffmpeg", buildFFmpegArgs(url, verbosity, opts)...)

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

	audioCh := p.outgoing()
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
			if frameIdx == 0 {
				// Not one frame of audio: the input failed to open or decode. Report
				// it so the caller counts a failure instead of a finished track.
				onEnd(fmt.Errorf("%w (%v)", ErrNoAudio, err))
				return
			}
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

// buildFFmpegArgs returns the argument list for an ffmpeg invocation that decodes
// url to mono 16-bit LE PCM at 48 kHz. HTTP/HTTPS URLs get reconnect flags so
// live streams survive transient interruptions. For finite tracks
// -reconnect_at_eof is omitted so a completed download ends instead of being
// restarted from the beginning; transient-error reconnection is kept. Headers
// and cookies from opts are passed before -i (http(s) only).
func buildFFmpegArgs(url, verbosity string, opts StreamOptions) []string {
	args := []string{"-v", verbosity, "-nostdin"}

	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		args = append(args,
			"-reconnect", "1",
			"-reconnect_streamed", "1",
			"-reconnect_delay_max", "5",
		)
		if !opts.Finite {
			args = append(args, "-reconnect_at_eof", "1")
		}
		args = append(args, "-reconnect_on_http_error", "4xx,5xx")
		args = append(args, httpInputArgs(opts)...)
	}

	args = append(args, "-i", url, "-ac", "1", "-f", "s16le", "-ar", "48000", "-")
	return args
}

// httpInputArgs renders opts.Headers/Cookies as ffmpeg http-protocol options.
// User-Agent goes through -user_agent; everything else through -headers, which
// requires each line to end in CRLF. Values containing CR/LF are dropped so a
// header can never inject another one.
func httpInputArgs(opts StreamOptions) []string {
	var args []string

	keys := make([]string, 0, len(opts.Headers))
	for k := range opts.Headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var hdr strings.Builder
	for _, k := range keys {
		v := opts.Headers[k]
		if k == "" || strings.ContainsAny(k, ":\r\n") || strings.ContainsAny(v, "\r\n") {
			continue // malformed or injection attempt
		}
		if strings.EqualFold(k, "User-Agent") {
			args = append(args, "-user_agent", v)
			continue
		}
		if strings.EqualFold(k, "Cookie") {
			continue // cookies are passed scoped, via opts.Cookies
		}
		hdr.WriteString(k + ": " + v + "\r\n")
	}
	if hdr.Len() > 0 {
		args = append(args, "-headers", hdr.String())
	}

	var cookies []string
	for _, c := range opts.Cookies {
		if c != "" && !strings.ContainsAny(c, "\r\n") {
			cookies = append(cookies, c)
		}
	}
	if len(cookies) > 0 {
		args = append(args, "-cookies", strings.Join(cookies, "\n"))
	}
	return args
}

// decodePCM converts a buffer of int16 little-endian bytes into a gumble AudioBuffer.
func decodePCM(buf []byte) gumble.AudioBuffer {
	samples := make(gumble.AudioBuffer, len(buf)/2)
	for i := range samples {
		samples[i] = int16(binary.LittleEndian.Uint16(buf[i*2:]))
	}
	return samples
}
