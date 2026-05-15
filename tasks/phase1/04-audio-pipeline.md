# 1-04 — Audio Pipeline

**Status:** todo  
**Depends on:** 1-03  
**Unlocks:** 1-06

## Objective

Pipe audio from an ffmpeg subprocess to Mumble's Opus output with correct PCM buffering, volume control, and fade in/out on track transitions.

## ffmpeg command

```
ffmpeg -v {level} -nostdin -i {url} -ac 1 -f s16le -ar 48000 -
```

- `-ac 1` always (mono). `gumble` hardcodes `AudioChannels = 1`; stereo PCM fed into it produces corrupted audio. `config.Bot.Stereo` is preserved in the INI schema for future use but is ignored by the pipeline.
- `-v warning` normally; `-v debug` when `debug.ffmpeg = true`
- No `-ss` seek argument (radio streams are not seekable)

## PCM loop

Runs in its own goroutine. Gumble's `AudioOutgoing()` returns an **unbuffered** `chan<- gumble.AudioBuffer` — blocking sends provide natural backpressure. No sleep/threshold loop is needed.

```
frame_size = gumble.AudioDefaultFrameSize   // 480 samples = 10ms at 48kHz
                                            // matches gumble's AudioDefaultInterval

loop (frameIdx = 0, 1, 2, ...):
  if interruptCh closed: doFadeOut(); return
  vol.Cycle(elapsed since last frame)        // smooth RealVolume toward TargetVolume
  raw = io.ReadFull(ffmpeg.stdout, frame_size*2 bytes)
  if err: call onEnd(err or nil); return
  samples = decode int16-LE bytes → []int16
  scalar = vol.RealVolume * fadeInMultiplier(frameIdx)  // fadeIn: first 60 frames only
  applyScalar(samples, scalar)
  audioCh <- samples                         // blocking send to gumble
```

## Volume

```go
type VolumeHelper struct {
    TargetVolume float64   // set via SetTargetVolume (0.0–MaxVolume)
    RealVolume   float64   // smoothed toward TargetVolume each frame
    MaxVolume    float64   // ceiling from config; enforced in setter
}

// SetTargetVolume clamps vol to [0, MaxVolume] before storing.
func (v *VolumeHelper) SetTargetVolume(vol float64)

// Cycle advances RealVolume one step toward TargetVolume using exact exponential
// smoothing with τ = 0.5s:  RealVolume += (Target - Real) * (1 - exp(-δ/0.5))
func (v *VolumeHelper) Cycle(delta time.Duration)
```

`MaxVolume` is enforced in `SetTargetVolume`, not in `Cycle`. `Cycle` is a pure smoothing function.

## Fade in/out

Both fades run for exactly **60 frames** (600ms at 10ms/frame).

```
fadeInMultiplier(x)  = exp(-(60-x) / 60.0)   // x: 0..59; starts ≈0.37, ends ≈0.98
fadeOutMultiplier(x) = exp(-x / 60.0)         // x: 0..59; starts 1.0, ends ≈0.37
```

**Fade-in:** automatically applied to the first 60 frames of every `Launch`. No flag needed.

**Fade-out:** triggered by `Interrupt()`. The PCM goroutine detects the interrupt signal, reads 60 more frames from ffmpeg stdout (applying `fadeOutMultiplier`), then calls `cmd.Process.Kill()` and exits.

**Combined scalar per frame:** `sample * RealVolume * fadeMultiplier` — both applied as a single multiplication, never separately.

**Clamping:** all scalar applications clamp results to `[math.MinInt16, math.MaxInt16]` before casting to int16.

## Pipeline API

```go
type Pipeline struct { ... }

// New creates a Pipeline bound to the given gumble client.
// Call in the Connect handler — client must be non-nil and connected.
func New(client *gumble.Client, cfg *config.Config, log *slog.Logger) *Pipeline

// Launch starts ffmpeg and the PCM goroutine. onEnd is called exactly once
// when the goroutine exits (nil = clean end or Interrupt, non-nil = error).
// Returns ErrAlreadyRunning if a stream is in progress.
func (p *Pipeline) Launch(url string, onEnd func(error)) error

// Interrupt signals the PCM goroutine to begin fade-out and stop.
// Safe to call from any goroutine. No-op if not running.
func (p *Pipeline) Interrupt()

// IsRunning returns true while the PCM goroutine is active.
// Backed by atomic.Bool flipped by the goroutine itself.
func (p *Pipeline) IsRunning() bool

// Volume returns a pointer to the VolumeHelper for the active stream.
func (p *Pipeline) Volume() *VolumeHelper

var ErrAlreadyRunning = errors.New("audio pipeline already running")
```

## Bot wiring

`Pipeline` is created inside the `Connect` event handler (not in `bot.New`) because `*gumble.Client` is only available after a successful dial. On each reconnect the handler creates a fresh `Pipeline` and assigns it to `b.audio`.

Replace the `// TODO(1-04)` stubs in:
- `internal/bot/connect.go` — construct `audio.New(b.client, b.cfg, b.log)` in the `Connect` handler
- `internal/bot/bot.go` — change `audio interface{}` to `audio *audio.Pipeline`; call `b.audio.Interrupt()` in `Shutdown()`

## Deliverables

- `internal/audio/pipeline.go`
- `internal/audio/volume.go`
- `internal/audio/fade.go`
- `internal/audio/pipeline_test.go` — unit tests for `VolumeHelper.Cycle` convergence, `SetTargetVolume` clamping, fade envelope values, and `applyScalar` clamping

## Acceptance criteria

- Bot streams a radio URL continuously without audio stuttering
- `Interrupt()` produces an audible fade-out before the next track starts
- Volume 0.5 produces perceptibly quieter audio than volume 1.0
