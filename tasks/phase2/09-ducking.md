# 2-09 — Volume Ducking

**Status:** todo  
**Depends on:** Phase 1 audio pipeline, Phase 1 Mumble connection  
**Unlocks:** nothing (standalone feature)

## Objective

Reduce playback volume automatically when users speak in the channel.

## Behaviour

- Enable/disable via `config.Bot.Ducking` or `!duck on/off`
- Register gumble audio receive callback
- Compute RMS of incoming PCM frame
- If `rms > ducking_threshold`: set `on_ducking = true`, schedule release 1s in the future
- `volume_cycle()` (every audio frame):
  - Ducking active: `real_volume → ducking_volume` with τ = 0.2s
  - Normal: `real_volume → target_volume` with τ = 0.5s

## RMS

```go
func rms(samples []int16) float64 {
    var sum float64
    for _, s := range samples { sum += float64(s) * float64(s) }
    return math.Sqrt(sum / float64(len(samples)))
}
```

## Commands

| Command | Description |
|---|---|
| `!duck [on\|off]` | Enable or disable ducking |
| `!duckthres <n>` | Set RMS threshold |
| `!duckv <n>` | Set ducking volume (0–100) |

Settings persist in `SettingsDB`.

## Deliverables

- `internal/audio/ducking.go` — `DuckingDetector`
- Integrate into `VolumeHelper.Cycle` in the audio pipeline
- Tests: RMS above/below threshold transitions, volume convergence

## Acceptance criteria

- Bot volume drops when another user speaks
- Volume restores ~1s after they stop
- `!duck off` keeps volume constant
- Settings persist across restarts
