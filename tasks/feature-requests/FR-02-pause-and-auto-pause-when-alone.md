# FR-02 — Pause/Resume + auto-pause when the bot is alone in its channel

**Status:** open  
**Affects:** `internal/bot/`, `internal/command/`, `internal/config/`  
**Related milestone:** 1-05 (audio pipeline), 1-06 (bot loop), 1-07 (command dispatcher)

## Motivation

Two related asks:

1. **Pause/Resume** — a way to halt playback *without* resetting the queue, unlike
   `!stop` which rewinds the queue to the first item (`queue.Reset()`). After a pause,
   `!resume` should pick the current item back up.
2. **Auto-pause when alone** — when the last human leaves the bot's Mumble channel, the
   bot should stop feeding audio (no point streaming to an empty room, and it wastes
   ffmpeg CPU + upstream bandwidth). When someone rejoins, it should resume.

Requested UX:

```
!pause      ← halt playback, keep the queue position
!resume     ← continue from where !pause left off
(auto)      ← last user leaves the bot's channel → bot auto-pauses
(auto)      ← a user rejoins → bot auto-resumes (only if it auto-paused)
```

## Feasibility assessment

**Verdict: feasible, moderate effort, no architectural blockers.** Both pieces reuse
mechanisms already in the codebase. The one behaviour that needs to be called out and
documented — not worked around — is what "pause" means for a *live radio stream*.

### Finding 1 — "Pause" for live radio is really "detach + remember position"

Phase 1 plays live radio streams (`internal/radio/`). There is no seekable buffer: the
source keeps broadcasting whether or not we are listening. So a true tape-style
pause/resume (resume the exact same audio sample) is impossible. The honest, useful
semantics are:

- **Pause** = fade out and kill ffmpeg (same as `Interrupt()`), but keep
  `queue.currentIndex` pointing at the current item.
- **Resume** = re-launch the current queue item, i.e. rejoin the live stream.

This is the natural behaviour for radio anyway and matches what users expect ("it went
quiet, then it came back"). It must be documented so nobody files a bug that resume
"skipped ahead" — for live radio that is unavoidable. (In Phase 2, local-file playback
could support true position-preserving pause; out of scope here.)

### Finding 2 — the loop will fight a naive pause; two guards already exist to win

The loop (`internal/bot/loop.go:26-34`) relaunches `queue.Current()` whenever the
pipeline is idle:

```go
if pipeline == nil || pipeline.IsRunning() { continue }
item := b.queue.Current()
if item == nil { continue }
... pipeline.Launch(...)
```

So simply calling `Interrupt()` would immediately be undone — the loop would relaunch the
current item on its next tick. Two existing mechanisms solve this cleanly:

- **A paused gate** — add `paused atomic.Bool` to `Bot`; extend the loop guard to
  `... || b.paused.Load()`. While paused the loop idles and never relaunches.
- **`launchVersion` to suppress auto-advance** — `Interrupt()` runs the pipeline's
  `onEnd(nil)` callback, and that callback calls `queue.Next()` (that's how `Skip()`
  advances — see `loop.go:42-48`). If Pause just interrupted, the queue would advance to
  the *next* track, not stay on the current one. `Play()` already dodges this exact
  problem by bumping `launchVersion` before interrupting (`controls.go:19-26`); the
  callback's `b.launchVersion.Load() == launchVer` guard then fails and `Next()` is
  skipped. **Pause reuses the same trick.**

Net: Pause = `paused=true` + `launchVersion++` + `Interrupt()`. Resume = `paused=false` +
`wakeLoop()`. No new pipeline machinery.

### Finding 3 — channel occupancy is available via a `UserChange` event handler

gumble exposes everything needed; we're just not listening yet:

- `client.Self.Channel` — the bot's current channel.
- `channel.Users` — `gumble.Users` (`map[uint32]*User`), everyone in that channel
  (includes the bot itself; exclude `client.Self.Session`).
- `gumble.UserChangeEvent{Client, Type, User, Actor}` fires on connect / disconnect /
  channel move / mute / etc. (confirmed in `gumble/event.go:92`, `handlers.go`).

We attach a `UserChange` listener in `buildGumbleConfig` (`connect.go:101-131`, alongside
the existing `Connect`/`Disconnect`/`TextMessage` handlers) and, on every event,
**recompute** the occupancy of the bot's own channel rather than trying to parse the
`Type` bitmask. Recompute is robust to every case (user joins/leaves/moves, the bot
itself being moved via `!joinme`, disconnects). Counting "other users" = users in
`Self.Channel` whose `Session != Self.Session`.

### Finding 4 — auto-pause and manual pause must not clobber each other

If a user runs `!pause` and then someone walks into the channel, the bot should **not**
auto-resume — the human explicitly paused. So the pause state needs a *reason*:

- `pauseReasonNone` — playing (or idle).
- `pauseReasonManual` — `!pause` / (future) an admin action.
- `pauseReasonAuto` — auto-paused because the channel emptied.

Auto-resume only fires when the reason is `Auto`. A manual `!resume` clears any reason.

### Effort estimate

| Area | Work |
|---|---|
| `internal/bot/` | `Pause`/`Resume`/`IsPaused`, paused gate in loop, occupancy tracker + `UserChange` handler | 
| `internal/command/` | `!pause` / `!resume` handlers, `BotAPI` additions | 
| `internal/config/` | `auto_pause_when_alone`, `auto_resume_on_join` keys + allowlist + default ini | 
| Tests | queue-position-after-pause, paused-gate, occupancy transitions, reason precedence | 

## Design decisions

| Topic | Decision |
|---|---|
| Pause semantics (radio) | Detach + remember queue index; resume rejoins the live stream. Documented, not worked around. |
| Pause implementation | `paused atomic.Bool` gate in loop + `launchVersion++` before `Interrupt()` to suppress auto-advance (mirrors `Play()`). |
| Pause vs Stop | `!pause` keeps `currentIndex`; `!stop` keeps its current behaviour (`queue.Reset()` → index 0). |
| Pause vs Mute | Distinct. `!mute` keeps ffmpeg running at volume 0 (still burns CPU/bandwidth); `!pause` kills ffmpeg. Auto-pause uses pause precisely to free those resources. |
| Occupancy detection | `UserChange` listener recomputes `Self.Channel` occupancy each event; "others" excludes `Self.Session`. |
| Pause reason | `pauseReason` enum (none/manual/auto); auto-resume only undoes `auto`. |
| Auto-pause default | **Off** (`auto_pause_when_alone = False`) — opt-in, since silent auto-stop is surprising by default. |
| Auto-resume default | **On** when auto-pause is enabled (`auto_resume_on_join = True`). |
| Commands | New canonical commands `pause` and `resume` (defaults `!pause`, `!resume`). Not admin-only. |
| `BotAPI` change | Add `Pause()`, `Resume()`, `IsPaused()`. |

## Proposed changes

### 1 — Pause/Resume controls (`internal/bot/`)

Add to `Bot` (`bot.go`):

```go
paused      atomic.Bool
pauseReason atomic.Int32 // pauseReasonNone / Manual / Auto
```

`controls.go`:

```go
// Pause halts playback but keeps the queue position, unlike Stop which rewinds
// to the first item. For live radio, Resume rejoins the stream (no seek).
func (b *Bot) Pause() { b.pauseWithReason(pauseReasonManual) }

func (b *Bot) pauseWithReason(reason int32) {
    if b.paused.Swap... // set paused, store reason
    b.launchVersion.Add(1) // suppress onTrackEnd's queue.Next() (mirrors Play)
    b.mu.Lock()
    if b.audio != nil { b.audio.Interrupt() }
    b.mu.Unlock()
}

func (b *Bot) Resume() {
    // clear paused + reason, then wake the loop to relaunch queue.Current()
    b.paused.Store(false)
    b.wakeLoop()
}

func (b *Bot) IsPaused() bool { return b.paused.Load() }
```

`loop.go` — extend the idle guard (`loop.go:26`):

```go
if pipeline == nil || pipeline.IsRunning() || b.paused.Load() {
    continue
}
```

### 2 — Occupancy tracking + auto-pause (`internal/bot/`)

New handler in `buildGumbleConfig` (`connect.go`):

```go
UserChange: func(e *gumble.UserChangeEvent) {
    b.onOccupancyChange()
},
```

New method (e.g. `internal/bot/presence.go`):

```go
// onOccupancyChange auto-pauses when the bot is the only user left in its
// channel and auto-resumes (if it auto-paused) when someone returns.
func (b *Bot) onOccupancyChange() {
    if !b.cfg.Bot.AutoPauseWhenAlone { return }
    others := b.othersInChannel()
    switch {
    case others == 0 && !b.paused.Load():
        b.pauseWithReason(pauseReasonAuto)
        b.sendChannelMessage("Nobody here — pausing.")
    case others > 0 && b.paused.Load() && b.pauseReason == Auto && b.cfg.Bot.AutoResumeOnJoin:
        b.Resume()
        b.sendChannelMessage("Welcome back — resuming.")
    }
}

func (b *Bot) othersInChannel() int {
    // count Self.Channel.Users excluding Self.Session; 0 if not connected / no channel
}
```

Notes:
- Auto-pause should only *announce* when something is actually playing/queued, to avoid
  chatter on an idle bot — recompute is cheap but the message should be gated on
  `queue.Current() != nil` (or on `paused` transitions actually changing state).
- The bot moving channel (via `!joinme`) also triggers `UserChange`; recompute handles it.

### 3 — Commands (`internal/command/`)

- `BotAPI` (in `dispatcher.go`): add `Pause()`, `Resume()`, `IsPaused()`.
- `handlers.go`: `handlePause` → `bot.Pause()` + reply "Paused."; `handleResume` →
  `bot.Resume()` + reply "Resumed." (or "Nothing to resume." when `queue.Current()==nil`).
- `registry.go`: register `a("pause")`/`a("resume")` with descriptions:
  - pause — "Pause playback, keeping the queue position"
  - resume — "Resume playback after a pause"

### 4 — Config (`internal/config/`)

- `BotConfig`: add `AutoPauseWhenAlone bool`, `AutoResumeOnJoin bool`.
- `build()`: `AutoPauseWhenAlone: b.Key("auto_pause_when_alone").MustBool(false)`,
  `AutoResumeOnJoin: b.Key("auto_resume_on_join").MustBool(true)`.
- `sectionAllowlists["bot"]`: add `"auto_pause_when_alone": true`,
  `"auto_resume_on_join": true`.
- `configuration.default.ini` `[bot]`: `auto_pause_when_alone = False`,
  `auto_resume_on_join = True`. `[commands]`: `pause = pause`, `resume = resume`.

(The `[commands]` section is not allowlist-validated, so alias keys need no allowlist
change — only the two `[bot]` keys do.)

## Deliverables

| File | Change |
|---|---|
| `internal/bot/bot.go` | Add `paused atomic.Bool`, `pauseReason` state |
| `internal/bot/controls.go` | `Pause`, `pauseWithReason`, `Resume`, `IsPaused` |
| `internal/bot/loop.go` | Extend idle guard with `b.paused.Load()` |
| `internal/bot/connect.go` | Attach `UserChange` listener |
| `internal/bot/presence.go` (new) | `onOccupancyChange`, `othersInChannel` |
| `internal/command/dispatcher.go` | Add `Pause`/`Resume`/`IsPaused` to `BotAPI` |
| `internal/command/handlers.go` | `handlePause`, `handleResume` |
| `internal/command/registry.go` | Register `pause` / `resume` |
| `internal/config/config.go` | Add two bot keys + allowlist entries |
| `internal/config/configuration.default.ini` | Add config keys + command aliases |
| `tasks/phase1/07-commands.md` | Document `!pause` / `!resume` |
| `tasks/phase1/*` (loop/audio milestone) | Note the paused gate + auto-pause behaviour |

## Documentation requirements

- Command table gains `!pause` and `!resume` rows.
- A short note that **resume rejoins a live stream** — no seek-back for radio.
- `[bot]` config docs describe `auto_pause_when_alone` (default off) and
  `auto_resume_on_join` (default on), including that auto-resume only undoes an
  auto-pause, never a manual `!pause`.

## Test requirements

### `internal/bot` (queue + control-level, using the existing test fakes)

- `TestPause_keepsQueueIndex` — enqueue 3, play, `Pause()`; `queue.Index()` unchanged
  (contrast with `Stop()` which resets to 0).
- `TestPause_suppressesAutoAdvance` — pausing does not advance to the next item
  (`launchVersion` guard holds).
- `TestResume_relaunchesCurrent` — after Pause then Resume, the loop relaunches
  `queue.Current()`.
- `TestLoop_pausedGateIdles` — while `paused`, an idle pipeline is not relaunched.
- `TestOccupancy_autoPauseWhenAlone` — others→0 with feature on auto-pauses (reason=auto).
- `TestOccupancy_autoResumeOnJoin` — others 0→1 resumes only when reason=auto.
- `TestOccupancy_manualPauseNotAutoResumed` — `!pause` then user joins → stays paused.
- `TestOccupancy_featureOff_noop` — with `auto_pause_when_alone=false`, no auto-pause.

### `internal/command/handlers_test.go`

- `TestHandlePause` — calls `bot.Pause()`, replies "Paused."
- `TestHandleResume` — calls `bot.Resume()`, replies appropriately.
- Extend the `BotAPI` test fake with `Pause`/`Resume`/`IsPaused`.

### `internal/config`

- Parse test: the two new keys load with correct defaults; unknown-key validation still
  passes with them present.

## Acceptance criteria

- `!pause` halts audio and **keeps** the queue position; `!queue` shows the same current
  marker; `!resume` continues from that item.
- `!stop` behaviour is unchanged (still rewinds to the first item).
- With `auto_pause_when_alone = True`: when the last other user leaves the bot's channel,
  playback pauses automatically; when a user rejoins (and `auto_resume_on_join = True`)
  it resumes.
- A manual `!pause` is **not** undone by someone joining the channel.
- With `auto_pause_when_alone = False` (default), occupancy changes have no effect.
- `go test ./...` and `go vet ./...` pass.

## Open questions

1. **Announce auto-pause in channel?** Proposed: yes, a brief "pausing/resuming" line,
   but only when something is actually playing. Could be made configurable if noisy.
2. **Toggle vs. separate commands?** Proposed two commands (`!pause`/`!resume`) for
   clarity; a single toggling `!pause` is an alternative.
3. **Grace period?** A user bouncing (leave→rejoin within seconds) would cause a
   pause/resume flap. A small debounce (e.g. 2–3 s before auto-pause) could smooth this;
   left out of the initial scope to keep the loop simple — revisit if it's annoying in
   practice.
