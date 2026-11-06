# 3-04 — Playlist Modes

**Status:** done  
**Depends on:** Phase 1 (queue), 3-03 (`audio.Resolver`)  
**Unlocks:** 3-06, 3-10

## Objective

Extend the queue with playback modes (`one-shot`, `repeat`, `random`) and a `!mode` command.
Works on mixed queues (radio + HLS + file + URL).

## Design decisions

| # | Decision |
|---|---|
| 1 | No `ItemWrapper`. Modes work on `[]audio.MediaItem`. Tags and ready/failed state wait for the milestone that needs them (3-05/3-06). |
| 2 | `random` **shuffles the list** after the current item; new items are inserted at a random position after the current one. `Next()` stays sequential in every mode. |
| 3 | `autoplay` is **deferred to 3-05** (nothing populates the music DB yet). `!mode autoplay` is refused with a clear message. |
| 4 | Extend `internal/queue` in place; keep the type name `Queue`. No package rename. |
| 5 | Ships `!mode` only. `!random`, `!repeat [n]`, `!last` stay in 3-06. Mode is in-memory; `[bot] playback_mode` sets the startup value. |
| 6 | At the end of the list `random` reshuffles and keeps going. The first track of the new lap is never the one that just played. |
| 7 | The spec's "async prepare" is dropped: `audio.Resolver` (3-03) already resolves items just before they play. See Known limitations. |
| 8 | A queue `Version()` counter ships here after all: the failure guard needs it to detect a changed queue atomically (see the failure-guard bullet below). The web UI (3-07) reuses it. |
| 9 | Anyone can read and set `!mode`. A bad `playback_mode` in config logs a warning and falls back to `one-shot`. |

## Modes

| Mode | `Next()` behaviour |
|---|---|
| `one-shot` | Advance the index; past the last item, go idle (current behaviour). |
| `repeat` | Advance the index; past the last item, wrap to 0. |
| `random` | Advance the index over a shuffled list; past the last item, reshuffle and wrap to 0. |
| `autoplay` | Not available in 3-04 (see 3-05). |

## Queue API (`internal/queue`)

```go
type Mode string

const (
    ModeOneShot Mode = "one-shot"
    ModeRepeat  Mode = "repeat"
    ModeRandom  Mode = "random"
)

func ParseMode(s string) (Mode, error)  // accepts the three modes; "autoplay" returns ErrAutoplayUnavailable
func (q *Queue) Mode() Mode
func (q *Queue) SetMode(m Mode)         // entering random shuffles the items after the current one
func (q *Queue) Randomize()             // shuffle items after the current index (current item stays put)
```

- `Next()` becomes mode-aware; its signature is unchanged. All three callers (the loop's
  failure path, the end-of-track callback, `Skip()`) pick up the new behaviour.
- In `random` mode `Append` places the new item at a random index after the current one.
  `Insert` keeps its explicit index (it is a positional operation; nothing calls it today).
- Switching back to `one-shot` does **not** restore the original order.
- Mode and shuffle live under the queue's existing mutex.

## `!mode` command

- `!mode` — show the current mode and the available ones.
- `!mode repeat` / `!mode random` / `!mode one-shot` — set it.
- `!mode autoplay` — "autoplay needs the music library (not available yet)".
- Anything else — usage message.
- Open to all users (`adminOnly=false`), like the other playback commands.
- Alias key: `[commands] mode = mode`.

## Config

```ini
[bot]
playback_mode = one-shot   # one-shot | repeat | random
```

An invalid value (including `autoplay`) logs a warning and falls back to `one-shot`; the bot
still starts. The mode is **not** persisted across restarts (3-10 handles that).

## Interaction with the play loop

- `Randomize()`/`SetMode(random)` can change which item is current's successor, not the current
  item itself, so an in-flight `Resolve` is unaffected. Anything that changes the current index
  must still go through the `playEpoch` guard (`invalidatePlayback`).
- **Failure guard (reworked after review):**
  - *Giving up.* The loop used to call `Stop()` after "too many consecutive failures". `Stop()`
    resets the queue to index 0, so an all-failing queue was retried forever (~35 attempts per
    700 ms with 5 items, in every mode). It now goes idle at the end of the queue with the items
    kept, via `Queue.FinishIfUnchanged`. Playback resumes when something new is queued or played.
  - *What counts.* The guard tracks **distinct failed entries** since the last played track and
    gives up only when every entry currently in the queue has failed. Counting attempts would
    stop a queue that still has playable tracks: in `random` mode a reshuffle can retry the same
    failing entries before a playable one comes up. A played track, or any user control, clears
    the streak.
  - *Stale results.* The loop takes `Queue.Snapshot()` (items, index and `Version`) before it
    resolves/launches. The failure path only moves the queue (`FinishIfUnchanged`,
    `NextIfUnchanged`) if the version is unchanged. Comparing the current item is not enough:
    `!play` can re-select the same item, a skip can wrap a one-item repeat/random queue back onto
    it, and an item appended during the failing resolve leaves the current item unchanged.
  - *Unplayable files.* If ffmpeg starts but produces no audio at all (e.g. a file deleted after
    it was queued) the pipeline reports `audio.ErrNoAudio`, and that counts as a failure. Before,
    process creation counted as success and an unplayable `repeat`/`random` queue spun forever.
    A stream that produced at least one frame and then ended is still a normal end of track.
- A one-item queue in `repeat` or `random` replays that item forever.

## Known limitations

- A URL track only starts resolving when the previous track ends, so there can be a 2–10 s gap
  between consecutive yt-dlp tracks. Prefetching the next item is a possible follow-up
  milestone; it needs invalidation on every queue mutation.
- Mode is lost on restart (until 3-10).

## Deliverables

- `internal/queue/queue.go` — mode, `SetMode`, `Randomize`, mode-aware `Next`/`Append`/`Insert`
- `internal/queue/mode.go` — `Mode`, `ParseMode`
- `!mode` handler in `internal/command/` and its registration
- `[bot] playback_mode` in `internal/config/` and `configuration.default.ini`; wire into `bot.New`
- README (command table, `[bot]` key) and CHANGELOG
- Tests: each mode's `Next()` transitions; random reshuffle never repeats the last-played item
  first; concurrent `Append`/`Next`/`SetMode` under `-race`; `ParseMode`; `!mode` handler;
  config default and invalid-value fallback

## Acceptance criteria

- `!mode repeat` loops the queue; `!mode one-shot` stops at the end again
- `!mode random` shuffles the upcoming items and keeps playing after the last one
- New items queued in `random` mode land at a random position after the current track
- `!mode autoplay` is refused with a clear message
- `!mode` with no argument shows the current mode
- A mixed queue (radio + file + URL) plays correctly in every mode
- Invalid `playback_mode` in config: warning logged, bot starts in `one-shot`
