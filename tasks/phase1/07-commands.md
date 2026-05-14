# 1-07 — Command Dispatcher & Radio Commands

**Status:** todo  
**Depends on:** 1-03, 1-05, 1-06  
**Unlocks:** 1-08 (feature complete for Phase 1)

## Objective

Parse incoming Mumble text messages, route them to handlers, and implement all radio-relevant commands.

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
| `!rbquery <name>` | Search radio-browser.info, display table of results |
| `!rbplay <uuid>` | Play station by radio-browser UUID |
| `!stop` | Stop playback, reset queue index |
| `!pause` | Pause (keep queue position) |
| `!play` / `!p` | Resume if paused; otherwise show current station |
| `!skip` | Skip to next queued station |
| `!clear` | Stop and empty the queue |
| `!queue` | List queued stations |
| `!np` / `!now` | Show currently playing station |
| `!volume [0-100]` | Get or set volume |
| `!joinme` | Bot moves to caller's channel |
| `!kill` | Disconnect and exit (admin only) |
| `!help` | List available commands |

## `!rbquery` response format

```
Radio-Browser results:
| rbplay ID | Station Name | Genre | Codec/Bitrate | Country |
| --------- | ------------ | ----- | ------------- | ------- |
| <uuid>    | <name>       | ...   | ...           | ...     |
```

Truncate to 5000 chars (Mumble message limit). If still too long, drop codec/country columns.

## Deliverables

- `internal/command/dispatcher.go`
- `internal/command/handlers.go` — all handler functions
- `internal/command/registry.go` — `RegisterAll(bot *Bot, d *Dispatcher)`
- Unit tests: exact match, prefix match, ambiguous prefix, admin guard, unknown command

## i18n

Phase 1 uses hardcoded English strings. No JSON lang file loading yet (that moves to Phase 2).

## Acceptance criteria

- `!radio jazz` plays the jazz preset
- `!radio http://stream.somafm.com/groovesalad-128-mp3` plays the direct URL
- `!rbquery soma` returns a table of SomaFM stations
- `!rbplay <uuid>` plays the station
- `!queue` shows all queued stations with their index
- `!volume 60` sets volume to 60%
- `!hel` (partial match) dispatches to `!help`
- `!kill` from a non-admin gets "not admin" response
