# 1-04 — Audio Pipeline

**Status:** todo  
**Depends on:** 1-03  
**Unlocks:** 1-06

## Objective

Pipe audio from an ffmpeg subprocess to Mumble's Opus output with correct PCM buffering, volume control, and fade in/out on track transitions.

## ffmpeg command

```
ffmpeg -v {level} -nostdin -i {url} -ac {channels} -f s16le -ar 48000 -
```

- `-ac 2` (stereo) or `-ac 1` (mono) from `config.Bot.Stereo`
- `-v warning` normally; `-v debug` when `debug.ffmpeg = True`
- For radio streams, no `-ss` seek argument (streams have no seekable position)

## PCM loop

```
frame_size = 960 * channels          // one Opus frame = 20ms at 48kHz
buffer_threshold = 0.5 seconds

loop:
  while mumble_buffer > buffer_threshold: sleep(10ms)
  raw = ffmpeg.stdout.read(frame_size)
  if raw == "":
      // stream ended or error
      break
  pcm = apply_volume(raw, real_volume)
  mumble.sound_output.add(pcm)
```

## Volume

```go
type VolumeHelper struct {
    TargetVolume float64   // set by !volume command (0.0–1.0)
    RealVolume   float64   // smoothed toward TargetVolume each frame
    MaxVolume    float64   // ceiling from config
}

// Called every frame:
// RealVolume approaches TargetVolume exponentially (τ = 0.5s)
func (v *VolumeHelper) Cycle(delta time.Duration)

// Apply scalar to raw PCM bytes (int16 little-endian)
func (v *VolumeHelper) ApplyToFrame(raw []byte) []byte
```

## Fade in/out

On track start: apply `exp(-x/60)` envelope in reverse (fade in).  
On `Interrupt()`: apply `exp(-x/60)` envelope forward (fade out) to the current frame, then kill ffmpeg.

## Pipeline API

```go
type Pipeline struct { ... }

func (p *Pipeline) Launch(url string) error
func (p *Pipeline) Interrupt()             // fade-out then kill ffmpeg
func (p *Pipeline) IsRunning() bool
func (p *Pipeline) Volume() *VolumeHelper
```

## Deliverables

- `internal/audio/pipeline.go`
- `internal/audio/volume.go`
- `internal/audio/fade.go`
- Unit tests for `VolumeHelper.Cycle` convergence and fade envelope math

## Acceptance criteria

- Bot streams a radio URL continuously without audio stuttering
- `Interrupt()` produces an audible fade-out before the next track starts
- Volume 0.5 produces perceptibly quieter audio than volume 1.0
