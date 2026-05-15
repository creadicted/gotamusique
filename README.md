# gotamusique

A Go rewrite of [botamusique](https://github.com/azlux/botamusique), a Mumble music bot. Single statically-linked binary, no Python runtime required.

> **Work in progress.** The bot currently connects to Mumble and can stream audio via the internal pipeline, but has no chat command interface yet. Radio commands (`!radio`, `!skip`, etc.) land in milestone 1-06/1-07.

## What works today

- Connects to a Mumble server with TLS (certificate auth optional)
- Joins a configured channel on connect
- Reconnects automatically on disconnect with exponential backoff (up to 10 retries)
- Streams audio from a URL via ffmpeg → Opus pipeline:
  - Smooth volume control with exponential fade (600ms fade-in on track start, fade-out on stop)
  - Configurable volume and max-volume ceiling
- Reads configuration from an INI file with sane defaults embedded in the binary
- Logs to stderr (or a file) with structured key=value output
- Graceful shutdown on SIGINT / SIGTERM

## What is not implemented yet

- Chat command dispatcher (`!radio`, `!volume`, `!skip`, `!stop`, etc.)
- Play queue
- Radio-browser.info search (`!rbquery`)
- Docker image

## Requirements

- Go 1.21+
- `ffmpeg` on PATH (for audio streaming)

## Build

```sh
make build          # produces bin/gotamusique
make test           # run all tests
make run            # build and run (reads configuration.ini next to binary)
```

## Configuration

Copy `configuration.default.ini` (embedded in the binary, printed below) to `configuration.ini` next to the binary and edit what you need. Missing keys fall back to the embedded defaults.

```ini
[server]
host = 127.0.0.1
port = 64738
password =
channel =
tls_skip_verify = True

[bot]
username = gotamusique
volume = 0.8
max_volume = 1.0
admin =
logfile =

[radio]
jazz = http://jazz-wr04.ice.infomaniak.ch/jazz-wr04-128.mp3 "Jazz Yeah !"
```

Override individual values on the command line:

```sh
./bin/gotamusique --host my.mumble.server --username MusicBot
```

Run `./bin/gotamusique --help` for the full flag list.

## Debug options

```ini
[debug]
ffmpeg = True             # log ffmpeg stderr at debug level
mumble_connection = True  # verbose gumble connection logging
```

## Project structure

```
cmd/gotamusique/    main entrypoint
internal/
  config/           INI loading, embedded defaults, CLI flag overrides
  bot/              Bot struct, connect/reconnect loop, graceful shutdown
  audio/            ffmpeg subprocess, PCM pipeline, volume, fade
tasks/              milestone specs (see tasks/README.md)
source/             original Python bot (reference only, do not modify)
```

## Roadmap

See [`tasks/README.md`](tasks/README.md) for the full milestone list. Phase 1 delivers a deployable radio bot; Phase 2 adds local file playback, yt-dlp, a web UI, and full parity with the original Python bot.
