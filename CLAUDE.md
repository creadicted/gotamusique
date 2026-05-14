# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project overview

**gotamusique** is a Go rewrite of [botamusique](https://github.com/azlux/botamusique), an archived Python Mumble music bot. The goal is a single statically-linked binary with fewer runtime dependencies.

- Original Python code lives in `source/` — **do not modify it**; it exists as reference only.
- All new Go code goes at the repo root.
- The `tasks/` folder contains detailed milestone specs; `tasks/README.md` is the index.

## Build & test

Once the scaffold is in place (milestone 1-01), standard Go commands apply:

```sh
make build        # go build ./...
make test         # go test ./...
make run          # build and run with a config file
go vet ./...      # must be clean before any commit
go test ./...     # run all tests
go test ./internal/audio/...   # run a single package's tests
```

## Planned directory layout

```
cmd/gotamusique/      # main entrypoint
internal/
  config/             # INI loading (gopkg.in/ini.v1), two-file merge
  bot/                # Bot struct, main event loop
  audio/              # ffmpeg subprocess pipeline, volume, fade
  radio/              # RadioItem, radio-browser.info API client
  queue/              # in-memory play queue
  command/            # dispatcher + all chat command handlers
```

## Key libraries

| Purpose | Package |
|---|---|
| Mumble protocol | `github.com/layeh/gumble` |
| INI config | `gopkg.in/ini.v1` |
| SQLite (Phase 2) | `modernc.org/sqlite` (pure Go, no CGO) |
| HTTP server (Phase 2) | stdlib `net/http` |

`ffmpeg` and `yt-dlp` (Phase 2) are external binaries called via `os/exec` — they are not Go dependencies.

## Architecture notes

**Audio pipeline** (`internal/audio/`): ffmpeg is launched as a subprocess with `-f s16le -ar 48000` output piped to stdout. The Go side reads 960-sample frames (20 ms at 48 kHz), applies a `VolumeHelper` scalar to the raw int16-LE PCM, and feeds frames to `gumble`'s sound output. `Interrupt()` fades out then kills ffmpeg before the next track starts.

**Command dispatcher** (`internal/command/`): strips HTML from incoming Mumble text messages, matches the command symbol prefix (default `!`), then does exact-match → prefix-match → ambiguous reply. `HandlerFunc` signature: `func(bot *Bot, user string, msg *gumble.TextMessage, cmd, arg string)`.

**Config** (`internal/config/`): loads `configuration.default.ini` first, then overlays `configuration.ini` (user file). Schema mirrors the original Python bot so existing config files remain compatible.

## Development phases

**Phase 1 (MVP):** Radio streaming only — no database, no file library, no yt-dlp, no web UI. After milestone 1-08 the bot is deployable. Milestones live in `tasks/phase1/`.

**Phase 2 (full bot):** SQLite, local file playback, yt-dlp, playlist modes, music library, web API + frontend, ducking, persistence, admin features. Milestones live in `tasks/phase2/`.

Each milestone file specifies its own acceptance criteria and is independently mergeable.
