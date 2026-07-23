# 2-01 — HLS / M3U8 stream detection

**Status:** todo  
**Depends on:** Phase 1 complete  
**Unlocks:** nothing (standalone enhancement)

## Objective

Auto-detect HLS `.m3u8` playlist URLs inside `handleRadio` and represent them as a
distinct `HLSItem` type so they appear with a `[HLS]` title prefix in announcements
and `!queue` output. Add ffmpeg reconnect flags to improve resilience against transient
stream interruptions.

## HLSItem

Create `internal/radio/hls.go` (co-located with `RadioItem` — same package, same
pipeline concerns):

```go
// HLSItem represents an HLS/M3U8 playlist stream.
type HLSItem struct {
    URL  string
    Name string
}

func (h *HLSItem) StreamURL() string   { return h.URL }
func (h *HLSItem) FormatTitle() string { return "[HLS] " + h.Name }
```

`Name` defaults to the URL hostname (same logic as `NewRadioItemFromURL`).

## Detection in handleRadio

In `internal/command/handlers.go`, inside the `http`/`https` branch of `handleRadio`,
check whether the URL path ends with `.m3u8` (case-insensitive). If so, construct an
`HLSItem`; otherwise fall through to the existing `RadioItem` path. No separate command
needed.

```go
if strings.HasSuffix(strings.ToLower(u.Path), ".m3u8") {
    playHLS(bot, cfg, msg, arg)
} else {
    playURL(bot, cfg, msg, arg)
}
```

`playHLS` mirrors `playURL`: construct `HLSItem`, skip the `Validate()` step (HEAD
requests to HLS manifests often return `405`), enqueue, reply.

## ffmpeg reconnect flags

In `internal/audio/pipeline.go`, extend `Launch` to accept the URL so reconnect flags
can be injected for HLS streams. The cleanest approach: detect `http`/`https` prefix and
add `-reconnect 1 -reconnect_streamed 1 -reconnect_delay_max 5` before `-i` for all
HTTP sources (radio and HLS alike — these flags are no-ops for non-live sources).

Alternatively, add the flags unconditionally since they are harmless for local files and
standard radio streams too.

## Deliverables

- `internal/radio/hls.go` — `HLSItem` struct
- Edit `internal/command/handlers.go` — `.m3u8` detection branch + `playHLS`
- Edit `internal/audio/pipeline.go` — add ffmpeg reconnect flags for HTTP sources
- Tests: `TestHLSItemFormatTitle`, `TestDetectM3U8` (unit-test the URL detection logic)

## Acceptance criteria

- `!radio https://example.com/stream.m3u8` announces `[HLS] example.com`
- `!radio https://example.com/stream.mp3` still announces `[Radio] example.com`
- ffmpeg is invoked with `-reconnect 1 -reconnect_streamed 1 -reconnect_delay_max 5`
  for HTTP/HTTPS URLs
- No new top-level command is introduced
