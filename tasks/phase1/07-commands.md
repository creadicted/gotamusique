# 1-07 — Command Dispatcher & Radio Commands

**Status:** done  
**Depends on:** 1-03, 1-05, 1-06  
**Unlocks:** 1-08 (feature complete for Phase 1)

## Objective

Parse incoming Mumble text messages, route them to handlers, and implement all radio-relevant commands.

## Architecture decisions

### Circular-import break
`command` defines `BotAPI` — an interface covering the methods handlers need. `*bot.Bot`
satisfies it without the `bot` package knowing about `command`. `bot` imports `command`
(to create `Dispatcher` and call `RegisterAll`); `command` does NOT import `bot`.

### Admin check
Case-sensitive string comparison against `cfg.Bot.Admin`. Mumble usernames are
case-sensitive at the protocol level.

### URL vs preset detection (`!radio <arg>`)
`url.Parse` + scheme check: `http`/`https` → treat as direct stream URL (validate first);
anything else → look up in `cfg.Radio` by exact key.

### Stream validation
Direct URLs (`!radio <url>`) are validated with `RadioItem.Validate()` before enqueuing.
Config presets and radio-browser results (`!rbplay`) are trusted and not validated —
`rbplay` URLs come from a structured API and many streams use the ICY/SHOUTcast protocol
which the Go HTTP client cannot parse, so validation produces false negatives.

### Private message guard
Hardcoded reject (no config option in Phase 1). Private messages have no `Channels`.

## Dispatcher

### Message parsing

Strip HTML tags from raw message, then match:
```
^[{command_symbols}](?P<command>\S+)(?:\s(?P<argument>.*))?
```

`command_symbols` from `config.Commands.Symbol` (default `!`).

### Command lookup

1. Exact match in registered handlers
2. Prefix match: if exactly one registered command starts with the typed prefix → dispatch it
3. Multiple prefix matches → reply "did you mean: X, Y, Z?"
4. No match → reply "unknown command"

### Guards

- `allow_private_message = False`: reject commands sent as private messages (not in a channel)
- Admin-only commands: check sender name against `config.Bot.Admin` list
- Actor `== 0` (server message): ignore silently

### Dispatcher API

```go
type HandlerFunc func(bot *Bot, user string, msg *gumble.TextMessage, cmd, arg string)

type Dispatcher struct { ... }

func (d *Dispatcher) Register(aliases []string, fn HandlerFunc, adminOnly bool)
func (d *Dispatcher) Dispatch(bot *Bot, msg *gumble.TextMessage)
```

## Radio commands

| Command (default alias) | Description |
|---|---|
| `!radio [name\|url]` | No arg: list presets. Name: play preset. URL: play stream URL. |
| `!rbquery <name> [-n N]` | Search radio-browser.info; N results (default 10, max 50) |
| `!rbplay <uuid\|N>` | Play station by UUID or by index from last `!rbquery` |
| `!stop` | Stop playback and reset queue |
| `!mute` | Set volume to 0 — ffmpeg keeps running, stream stays connected |
| `!unmute` | Restore volume after `!mute` |
| `!skip` | Skip to next queued station |
| `!clear` | Stop and empty the queue |
| `!queue` | List queued stations with their index |
| `!np` / `!now` | Show currently playing station |
| `!volume [0-100]` | Get or set volume percentage |
| `!joinme` | Bot moves to caller's channel |
| `!kill` | Disconnect and exit (admin only) |
| `!help` | List available commands |

> **Note:** there is no `!pause` / `!resume`. Radio streams are live and cannot be seeked.
> `!mute` / `!unmute` are the equivalent operations — they silence the bot without
> reconnecting to the stream.

## Command output formats

### `!radio` (no arg)
Presets sorted alphabetically by key; each line: `key — comment` (falls back to URL
hostname if comment is empty). HTML mode: `<b>key</b> — name<br>` wrapped in a header.

### `!queue`
```
Queue (N items):
> 1. [Radio] Jazz Yeah !   ← > marks current index (1-based)
  2. SomaFM Groove Salad
```
`Queue is empty.` when nothing is queued. HTML mode: `<pre>`-wrapped.

### `!np`
`Now playing: <title>` (HTML: `<b>`-wrapped title); `Nothing is currently playing.` when idle.

### `!volume`
No arg: `Volume: 80` (current `TargetVolume * 100`, rounded). With arg: `Volume set to 80`.
Setting volume also clears the muted state.

### `!help`
All registered commands sorted alphabetically by primary alias, one per line:
`!alias1, !alias2 — description`. HTML mode: `<pre>`-wrapped. Uses actual configured
aliases, not hardcoded names.

## `!rbquery` response format

```
Radio-Browser results for "soma":
| #  | rbplay ID                            | Station Name             | Genre | Codec/Bitrate | Country |
| -- | ------------------------------------ | ------------------------ | ----- | ------------- | ------- |
|  1 | <uuid>                               | <name>                   | ...   | ...           | ...     |
|  2 | <uuid>                               | <name>                   | ...   | ...           | ...     |
```

Pipe table, `<pre>`-wrapped in HTML mode. Default 10 results; user-controllable via
`-n N` / `--limit N` flag (max 50). Truncate to 5000 chars; if still too long, rebuild
without Codec/Country columns; if still too long, hard-truncate to 5000.

`!rbplay N` after `!rbquery` plays result N from the most recent search in the channel.
Cache miss reply: `"No recent !rbquery results for this channel."`
Out-of-range reply: `"Index out of range — use !rbquery to see available stations."`

## Deliverables

- `internal/command/dispatcher.go`
- `internal/command/handlers.go` — all handler functions
- `internal/command/registry.go` — `RegisterAll(bot *Bot, d *Dispatcher)`
- Unit tests: exact match, prefix match, ambiguous prefix, admin guard, unknown command

## Bot wiring (required)

Milestone 1-03 left a `// TODO(1-07): register TextMessageEvent handler` comment in
`internal/bot/connect.go`. This milestone must replace that stub — registering
`Dispatcher.Dispatch` as the gumble `TextMessageEvent` handler and calling
`command.RegisterAll` during bot initialisation.

## Reply format

Controlled by `cfg.Bot.FormattedReplies` (default `True`).

- `true` — replies use HTML: `<b>`, `<br>`, `<pre>` for tables, etc.
- `false` — replies are plain text with newlines only.

All handler helpers that build reply strings must check this flag. A single
`format(cfg, template, args...)` helper (or equivalent) in `handlers.go`
centralises the switch.

## i18n

Phase 1 uses hardcoded English strings. No JSON lang file loading yet (that moves to Phase 2).

## Acceptance criteria

- `!radio jazz` plays the jazz preset
- `!radio http://stream.somafm.com/groovesalad-128-mp3` plays a direct URL
- `!rbquery soma` returns a table of SomaFM stations
- `!rbplay <uuid>` plays the station
- `!mute` silences the bot; `!unmute` restores volume; ffmpeg never restarts
- `!queue` shows all queued stations with their index
- `!volume 60` sets volume to 60%
- `!hel` (partial match) dispatches to `!help`
- `!kill` from a non-admin gets "not admin" response
