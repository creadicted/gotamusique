# gotamusique

A Go rewrite of [botamusique](https://github.com/azlux/botamusique), a Mumble music bot. Single statically-linked
binary, no Python runtime required.

> **Work in progress.** The bot currently connects to Mumble and can stream audio via the internal pipeline, but has no
> chat command interface yet. Radio commands (`!radio`, `!skip`, etc.) land in milestone 1-06/1-07.

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

## Download

Pre-built static binaries are published automatically on every commit to `master`:

| Platform     | Binary                    |
|--------------|---------------------------|
| Linux x86-64 | `gotamusique-linux-amd64` |
| Linux ARM64  | `gotamusique-linux-arm64` |

Download from the [latest release](../../releases/tag/latest) — no Go toolchain or Python runtime needed.

> These are rolling pre-release builds. For stable versioned releases see below.

## Publishing a versioned release

1. Bump the version constant in `cmd/gotamusique/main.go`:
   ```go
   const version = "0.1.4"
   ```

2. Commit and push:
   ```sh
   git add cmd/gotamusique/main.go
   git commit -m "Release v0.1.4"
   git push origin master
   ```

3. Tag the commit and push the tag:
   ```sh
   git tag v0.1.4
   git push origin v0.1.4
   ```

CI picks up the tag, builds the binaries, and publishes a proper (non-pre-release) GitHub release at `releases/tag/v0.1.4` automatically.

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

Copy `configuration.default.ini` (embedded in the binary, printed below) to `configuration.ini` next to the binary and
edit what you need. Missing keys fall back to the embedded defaults.

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

See [`tasks/README.md`](tasks/README.md) for the full milestone list. Phase 1 delivers a deployable radio bot; Phase 2
adds local file playback, yt-dlp, a web UI, and full parity with the original Python bot.
