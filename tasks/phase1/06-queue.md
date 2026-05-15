# 1-06 — Simple Queue

**Status:** done  
**Depends on:** 1-04, 1-05  
**Unlocks:** 1-07

## Objective

A minimal thread-safe queue of `audio.MediaItem`s with a current-index pointer.
No playlist modes, no persistence, no DB — just a slice.

The queue holds the `audio.MediaItem` interface (defined in milestone 1-05) rather
than a concrete `*radio.RadioItem`, so Phase 2 items (files, yt-dlp) slot in
without touching the queue.

## Queue API (`internal/queue/queue.go`)

```go
type Queue struct {
    mu           sync.RWMutex
    items        []audio.MediaItem
    CurrentIndex int               // 0 on init; len(items) means "past end / idle"
}

func NewQueue() *Queue
func (q *Queue) Append(item audio.MediaItem)
func (q *Queue) Insert(index int, item audio.MediaItem) error  // error if out of range
func (q *Queue) Remove(index int) error                        // error if out of range
func (q *Queue) Current() audio.MediaItem                      // nil when empty or index >= len
func (q *Queue) Next() bool                                    // advance; false + index=len when at end
func (q *Queue) Clear()
func (q *Queue) Len() int
func (q *Queue) Items() []audio.MediaItem                      // copy of slice, safe to iterate
```

### Index semantics

- `CurrentIndex` starts at 0. `Current()` returns nil when `len(items) == 0 || CurrentIndex >= len(items)`.
- `Next()` at the last item sets `CurrentIndex = len(items)` and returns false. The loop treats `Current() == nil` as the idle signal; it does **not** re-launch the last track.
- `Stop()` resets `CurrentIndex = 0` (queue is also cleared, so the value is moot).

### `Remove` / `Insert` index adjustment

Both operations adjust `CurrentIndex` to keep pointing at the item that is actually playing:

- `Remove(i)`: if `i < CurrentIndex`, decrement `CurrentIndex` by 1.
- `Insert(i, item)`: if `i <= CurrentIndex`, increment `CurrentIndex` by 1.

Both return an error on an out-of-bounds index; the caller (command handler) surfaces this to the user.

## Bot playback controls (`internal/bot/controls.go`)

```go
func (b *Bot) Play(index int) error   // always Interrupt() then launch index; error if out of range
func (b *Bot) Mute()                  // SetTargetVolume(0) — ffmpeg keeps running
func (b *Bot) Unmute()                // restore previous volume via VolumeHelper
func (b *Bot) IsMuted() bool          // accessor for unexported isMuted field
func (b *Bot) Stop()                  // Interrupt() + queue.Clear() + CurrentIndex = 0
func (b *Bot) Skip()                  // Interrupt() + queue.Next() + wake loop
func (b *Bot) Clear()                 // Interrupt() + queue.Clear()
```

`Mute()`/`Unmute()` call `Pipeline.Volume().SetTargetVolume()`. ffmpeg is never killed.
`Play(index)` always calls `Interrupt()` before launching, even if nothing is playing.

## Main loop (`internal/bot/loop.go`)

The loop goroutine is started in the `Connect` event handler (not in `Run()`), because it
needs a live `*gumble.Client` to send channel messages. A per-connection cancel function is
stored as `cancelLoop func()` on `Bot` and called in the `Disconnect` handler before
signalling the reconnect channel.

```
loop(ctx, wakeCh):
  consecutiveFails = 0

  for:
    select:
      case <-ctx.Done():       return
      case <-wakeCh:           // fall through
      case <-time.After(100ms): // fall through

    if pipeline.IsRunning(): continue

    item = queue.Current()
    if item == nil: continue

    err = pipeline.Launch(item.StreamURL(), onTrackEnd)
    if err != nil:
      sendChannelMessage(formatAnnouncement(item) + ": failed to start")
      consecutiveFails++
      threshold = max(queue.Len(), 3)
      if consecutiveFails >= threshold:
        sendChannelMessage("too many consecutive failures, stopping")
        Stop()
        consecutiveFails = 0
        continue
      queue.Next()
      wakeLoop()    // non-blocking send on wakeCh
      continue

    consecutiveFails = 0
    if cfg.Bot.AnnounceCurrentMusic:
      sendChannelMessage(formatAnnouncement(item))

onTrackEnd(err):
  queue.Next()
  wakeLoop()        // non-blocking send on wakeCh
```

### Announcement helper

```go
// formatAnnouncement returns the plain-text channel message for a starting track.
// Exists as an extension point for a future HTML version.
func formatAnnouncement(item audio.MediaItem) string {
    return "Now playing: " + item.FormatTitle()
}
```

## Deliverables

- `internal/queue/queue.go`
- `internal/queue/queue_test.go` — append, remove, insert, next, concurrent safety, bounds errors, index adjustment
- `internal/bot/loop.go` — playback goroutine + `formatAnnouncement`
- `internal/bot/controls.go` — `Play`, `Mute`, `Unmute`, `IsMuted`, `Stop`, `Skip`, `Clear`

## Acceptance criteria

- `!radio jazz` adds the jazz preset and starts playing
- `!skip` moves to the next queued station
- `!clear` stops audio and empties the queue
- `!mute` silences audio without interrupting ffmpeg; `!unmute` restores volume
- Adding a second station while one is playing queues it without interrupting
- Queue exhausted → bot idles silently (no crash, no busy-loop)
- A queue of broken URLs stops after `max(queue.Len(), 3)` consecutive failures with a chat message
- `Remove` / `Insert` with an out-of-bounds index returns an error
- `Remove` / `Insert` adjust `CurrentIndex` to keep pointing at the playing item
