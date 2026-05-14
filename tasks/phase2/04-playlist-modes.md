# 2-04 — Playlist Modes

**Status:** todo  
**Depends on:** Phase 1 (queue), 2-01  
**Unlocks:** 2-06

## Objective

Extend the Phase 1 simple queue into a full playlist with four playback modes, an item wrapper concept, and support for mixed media types (radio + file + URL).

## Playback modes

| Mode | `Next()` behaviour |
|---|---|
| `one-shot` | Advance index; stop when end is reached |
| `repeat` | Advance index; wrap around to 0 when end is reached |
| `random` | Pick a random index from the unplayed set |
| `autoplay` | Like random; when queue empties, pull random tracks from the music DB |

## ItemWrapper

Replaces the bare `*RadioItem` in the Phase 1 queue. Wraps any `MediaItem` (radio, file, URL) and tracks async download state.

```go
type ItemWrapper struct {
    mu     sync.Mutex
    item   MediaItem
    status ItemStatus  // pending | ready | failed
    Tags   []string
    Version int
}

func (w *ItemWrapper) IsReady() bool
func (w *ItemWrapper) IsFailed() bool
func (w *ItemWrapper) Item() MediaItem
func (w *ItemWrapper) AddTags(tags []string)
func (w *ItemWrapper) RemoveTags(tags []string)
```

## Playlist API extensions (on top of Phase 1 Queue)

```go
func (p *Playlist) Randomize()               // shuffle items after CurrentIndex
func (p *Playlist) SetMode(mode string)
func (p *Playlist) Version() int             // monotonic counter for web UI polling
```

## Async prepare

When `Next()` is called, launch `item.Prepare()` in a goroutine. The main loop waits until `IsReady()` before calling `pipeline.Launch()`.

## Deliverables

- Refactor `internal/queue` → `internal/playlist` (or extend in place)
- `internal/playlist/wrapper.go`
- `internal/playlist/modes.go` — mode-specific `Next()` implementations
- Tests: mode transitions, concurrent append/remove, autoplay DB integration

## Acceptance criteria

- `!mode repeat` loops the queue
- `!mode random` shuffles and picks randomly
- `!mode autoplay` adds library tracks when queue runs dry
- Mixed queue (radio + file + URL) plays correctly
