# 1-06 — Simple Queue

**Status:** todo  
**Depends on:** 1-04, 1-05  
**Unlocks:** 1-07

## Objective

A minimal thread-safe queue of `audio.MediaItem`s with a current-index pointer.
No playlist modes, no persistence, no DB — just a slice.

The queue holds the `audio.MediaItem` interface (defined in milestone 1-05) rather
than a concrete `*radio.RadioItem`, so Phase 2 items (files, yt-dlp) slot in
without touching the queue.

## API

```go
type Queue struct {
    mu           sync.RWMutex
    items        []audio.MediaItem
    CurrentIndex int
}

func NewQueue() *Queue
func (q *Queue) Append(item audio.MediaItem)
func (q *Queue) Insert(index int, item audio.MediaItem)
func (q *Queue) Remove(index int)
func (q *Queue) Current() audio.MediaItem   // nil if empty
func (q *Queue) Next() bool                 // advance index; false if nothing next
func (q *Queue) Clear()
func (q *Queue) Len() int
func (q *Queue) Items() []audio.MediaItem   // snapshot for display
```

`Next()` behaviour: advance `CurrentIndex` by 1; return `false` if already at end (one-shot — stop when queue exhausted).

## Bot playback state (on `Bot`)

```go
type Bot struct {
    // ...
    IsPaused bool
}

func (b *Bot) Play(index int) error   // jump to index and start
func (b *Bot) Pause()
func (b *Bot) Resume()
func (b *Bot) Stop()                  // stop + reset queue index to -1
func (b *Bot) Skip()                  // interrupt + advance queue
func (b *Bot) Clear()                 // stop + empty queue
```

## Main loop (runs in a goroutine)

```
loop:
  if not paused and pipeline not running:
    item = queue.Current()
    if item == nil: sleep(100ms); continue
    err = pipeline.Launch(item.StreamURL(), onTrackEnd)
    // onTrackEnd: calls queue.Next() then wakes the loop
    if err: send channel message; queue.Next(); continue
    announce current station (if config.Bot.AnnounceCurrentMusic)
  sleep(100ms)
```

## Deliverables

- `internal/queue/queue.go`
- `internal/bot/loop.go` — the main playback goroutine
- `internal/bot/controls.go` — `Play`, `Pause`, `Resume`, `Stop`, `Skip`, `Clear`
- Unit tests for `Queue`: append, remove, next, concurrent safety

## Acceptance criteria

- `!radio jazz` adds the jazz preset and starts playing
- `!skip` moves to the next queued station
- `!clear` stops audio and empties the queue
- Adding a second station while one is playing queues it (doesn't interrupt)
- Queue is empty → bot is silent (no crash, no busy-loop)
