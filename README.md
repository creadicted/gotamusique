# gotamusique

A Go rewrite of [botamusique](https://github.com/azlux/botamusique), a Mumble music bot. Single statically-linked
binary, no Python runtime required.

> **Work in progress — Phase 1 (radio-only) nearing completion.**
> Milestones 1-01 through 1-08 are done. GHCR publishing (1-09) is the remaining step.

## Download

Pre-built static binaries are published automatically on every commit to `master`:

| Platform     | Binary                    |
|--------------|---------------------------|
| Linux x86-64 | `gotamusique-linux-amd64` |

Download from the [latest release](../../releases/tag/latest) — no Go toolchain or Python runtime needed.

> These are rolling pre-release builds. For stable versioned releases see the [Releases](../../releases) page.

## Requirements

- `ffmpeg` on PATH

No Go toolchain, no Python runtime, no system libraries — the binary is fully static.

## Quick start

```sh
# 1. Download the binary for your platform and make it executable
chmod +x gotamusique-linux-amd64

# 2. Create a config file next to it
cat > configuration.ini <<'EOF'
[server]
host = mumble.example.com
port = 64738
channel = Music

[bot]
username = gotamusique
admin = YourMumbleUsername

[radio]
jazz  = http://jazz-wr04.ice.infomaniak.ch/jazz-wr04-128.mp3 "Jazz Yeah!"
soma  = http://ice1.somafm.com/groovesalad-128-mp3 "SomaFM Groove Salad"
EOF

# 3. Run
./gotamusique-linux-amd64
```

The bot connects, joins the configured channel, and waits for commands in Mumble chat.

## Docker

The image is published to GHCR on every release. No Go toolchain or `ffmpeg` installation needed on the host — `ffmpeg` is bundled in the image.

```sh
# 1. Create a configuration.ini with your overrides (see Configuration below)
cat > configuration.ini <<'EOF'
[server]
host = mumble.example.com
port = 64738

[bot]
username = gotamusique
admin = YourMumbleUsername
EOF

# 2. Create docker-compose.yml
cat > docker-compose.yml <<'EOF'
services:
  gotamusique:
    image: ghcr.io/konradk/gotamusique:latest
    volumes:
      - ./configuration.ini:/app/configuration.ini:ro
    restart: unless-stopped
EOF

# 3. Run
docker compose up -d
```

Only put keys you want to override in `configuration.ini` — the image ships with `configuration.default.ini` baked in, so missing keys fall back to their defaults automatically.

**Using a custom config path:**

```sh
docker run -v /path/to/myconfig.ini:/app/myconfig.ini ghcr.io/konradk/gotamusique:latest --config myconfig.ini
```

**Building locally:**

```sh
make docker-build   # docker build -t gotamusique .
make docker-run     # docker compose up
```

## Configuration

Copy `configuration.default.ini` to `configuration.ini` next to the binary and edit what you need.
Missing keys fall back to the embedded defaults — you only have to set what you want to change.

### `[server]`

| Key               | Default     | Description                                    |
|-------------------|-------------|------------------------------------------------|
| `host`            | `127.0.0.1` | Mumble server hostname or IP                   |
| `port`            | `64738`     | Mumble server port                             |
| `password`        | *(empty)*   | Server password                                |
| `channel`         | *(empty)*   | Channel to join on connect (root if empty)     |
| `certificate`     | *(empty)*   | Path to a PEM client certificate for cert-auth |
| `tokens`          | *(empty)*   | Comma-separated access tokens                  |
| `tls_skip_verify` | `True`      | Skip TLS certificate verification              |

### `[bot]`

| Key                      | Default           | Description                                        |
|--------------------------|-------------------|----------------------------------------------------|
| `username`               | `gotamusique`     | Display name in Mumble                             |
| `volume`                 | `0.8`             | Initial volume (0.0–1.0)                           |
| `max_volume`             | `1.0`             | Upper ceiling for `!volume`                        |
| `admin`                  | *(empty)*         | Comma-separated Mumble usernames with admin access |
| `comment`                | `"I play radio."` | Bot comment shown on hover                         |
| `announce_current_music` | `True`            | Post station name to channel when a track starts   |
| `logfile`                | *(empty)*         | Write logs to this path instead of stderr          |

### `[radio]` — presets

Each key is a short alias; the value is a URL, optionally followed by a quoted display name.

```ini
[radio]
jazz = http://jazz-wr04.ice.infomaniak.ch/jazz-wr04-128.mp3 "Jazz Yeah!"
soma = http://ice1.somafm.com/groovesalad-128-mp3 "SomaFM Groove Salad"
lofi = http://stream.lofi.cafe/lofi "Lo-fi Hip-Hop"
defiance = http://stream.wknc.org:8000/wknc128
```

`!radio jazz` plays the `jazz` preset. `!radio` with no argument lists all presets.

### `[commands]` — aliases

Override any command's trigger words:

```ini
[commands]
command_symbol = !       # prefix for all commands (multiple: !:！)
volume = vol, v  # !vol and !v now both work
play_radio = radio, r
```

### Command-line overrides

Any config value can be overridden at runtime:

```sh
./gotamusique --host my.mumble.server --username MusicBot --channel Radio
./gotamusique --help   # full flag list
```

## Commands

All commands use `!` by default (configurable via `command_symbol`). Commands can be sent in any
channel the bot is in or moved to. Private messages are ignored.

Partial command names work: `!hel` dispatches to `!help` as long as it's unambiguous.

### Playback

| Command          | Description                                                              |
|------------------|--------------------------------------------------------------------------|
| `!radio`         | List configured radio presets                                            |
| `!radio <alias>` | Play a preset by its alias, e.g. `!radio jazz`                           |
| `!radio <url>`   | Play any direct stream URL, e.g. `!radio http://stream.example.com/live` |
| `!stop`          | Stop playback and reset the queue position                               |
| `!mute`          | Set volume to 0 — stream stays connected, no reconnect delay             |
| `!unmute`        | Restore volume after `!mute`                                             |
| `!skip`          | Stop current station and start the next one in queue                     |
| `!clear`         | Stop playback and empty the queue                                        |

### Queue

| Command         | Description                                     |
|-----------------|-------------------------------------------------|
| `!queue`        | List all stations in the queue with their index |
| `!np` or `!now` | Show the currently playing station              |

Adding a station while one is already playing appends it to the queue — it does not interrupt the current station.

Example session in Mumble chat:

```
You:  !radio soma
Bot:  Now playing: [Radio] SomaFM Groove Salad
You:  !radio jazz
Bot:  Queued: [Radio] Jazz Yeah! (position 2)
You:  !queue
Bot:  Queue:
      1. [Radio] SomaFM Groove Salad  ← current
      2. [Radio] Jazz Yeah!
You:  !skip
Bot:  Now playing: [Radio] Jazz Yeah!
```

### Radio Browser

Search [radio-browser.info](https://www.radio-browser.info) for stations by name:

```
You:  !rbquery soma
Bot:  Radio-Browser results:
      | rbplay ID                            | Station Name          | Genre       | Codec/Bitrate | Country |
      | ------------------------------------ | --------------------- | ----------- | ------------- | ------- |
      | 9cf19f37-35f5-11e8-a303-52543be04c81 | SomaFM: Groove Salad  | ambient     | MP3/128       | US      |
      | 6bc2f454-f66c-11e8-b42b-52543be04c81 | SomaFM: Lush          | dream pop   | MP3/128       | US      |
      | ...

You:  !rbplay 9cf19f37-35f5-11e8-a303-52543be04c81
Bot:  Now playing: [Radio] SomaFM: Groove Salad
```

### Volume

```
You:  !volume        → Bot: Volume: 80%
You:  !volume 60     → Bot: Volume set to 60%
You:  !volume 0      → Bot: Volume set to 0% (muted)
```

Volume is a percentage of `max_volume`. Setting it above 100 is not allowed.

### Navigation

| Command   | Description                       |
|-----------|-----------------------------------|
| `!joinme` | Bot moves to your current channel |
| `!help`   | List all available commands       |

### Admin commands

These require your Mumble username to be in `config.Bot.Admin`.

| Command | Description                 |
|---------|-----------------------------|
| `!kill` | Disconnect the bot and exit |

## Diverging from botamusique

gotamusique is a clean rewrite, not a drop-in replacement. If you are migrating an
existing `configuration.ini` from the original Python bot, note the following intentional
differences.

### Changed command keys (`[commands]`)

| botamusique key  | gotamusique key   | Reason                                                                                                                         |
|------------------|-------------------|--------------------------------------------------------------------------------------------------------------------------------|
| `pause = pause`  | `mute = mute`     | Radio streams are live and cannot be seeked. `!mute` sets volume to 0 without killing ffmpeg, avoiding reconnection on unmute. |
| `play = p, play` | `unmute = unmute` | Counterpart to `!mute`. `!play` no longer exists as a resume command; use `!np` to see what is currently playing.              |

### Removed config keys (Phase 2 or permanently dropped)

The following `[bot]` keys from botamusique are **not recognised** in Phase 1 and will be
silently ignored if present in your config file. Most are planned for Phase 2; a few are
permanently removed because they don't apply to a Go binary.

| Key                                                      | Status   | Notes                                                |
|----------------------------------------------------------|----------|------------------------------------------------------|
| `allow_private_message`                                  | deferred | Private messages are always ignored in Phase 1       |
| `allow_other_channel_message`                            | deferred | Phase 2                                              |
| `auto_check_update`                                      | removed  | No update mechanism; use the GitHub release workflow |
| `autoplay_length`                                        | deferred | Phase 2 playlist modes                               |
| `clear_when_stop_in_oneshot`                             | deferred | Phase 2                                              |
| `database_path` / `music_database_path`                  | deferred | Phase 2 (SQLite)                                     |
| `delete_allowed`                                         | deferred | Phase 2                                              |
| `download_attempts`                                      | deferred | Phase 2 (yt-dlp)                                     |
| `ducking` / `ducking_threshold` / `ducking_volume`       | deferred | Phase 2                                              |
| `language`                                               | deferred | Hardcoded English in Phase 1                         |
| `max_track_duration` / `max_track_playlist`              | deferred | Phase 2                                              |
| `music_folder` / `ignored_files` / `ignored_folders`     | deferred | Phase 2 (file library)                               |
| `pip3_path` / `target_version`                           | removed  | Not applicable to a Go binary                        |
| `playback_mode` / `save_playlist` / `save_music_library` | deferred | Phase 2                                              |
| `refresh_cache_on_startup`                               | deferred | Phase 2                                              |
| `tmp_folder` / `tmp_folder_max_size`                     | deferred | Phase 2 (yt-dlp downloads)                           |
| `when_nobody_in_channel`                                 | deferred | Phase 2                                              |

The entire `[webinterface]` and `[youtube_dl]` sections are Phase 2 and ignored for now.

### New keys (not in botamusique)

| Key               | Section    | Description                                                                                                |
|-------------------|------------|------------------------------------------------------------------------------------------------------------|
| `tls_skip_verify` | `[server]` | Skip TLS certificate verification (default `True`). Many self-hosted Murmur servers use self-signed certs. |

## Debug options

```ini
[debug]
ffmpeg = True  # log ffmpeg stderr at debug level
mumble_connection = True  # verbose gumble connection logging
```

## Build from source

```sh
git clone https://github.com/konradk/gotamusique
cd gotamusique
make build   # produces bin/gotamusique
make test    # run all tests
```

## Publishing a versioned release

1. Bump the version constant in `cmd/gotamusique/main.go`.
2. Commit, push, then tag:
   ```sh
   git tag v0.1.5
   git push origin v0.1.5
   ```

CI builds the binaries and publishes a GitHub release automatically.

## Project structure

```
cmd/gotamusique/    main entrypoint
internal/
  config/           INI loading, embedded defaults, CLI flag overrides
  bot/              Bot struct, connect/reconnect loop, graceful shutdown
  audio/            ffmpeg subprocess, PCM pipeline, volume, fade; MediaItem interface
  radio/            RadioItem, RadioBrowser (radio-browser.info client)
  queue/            thread-safe play queue  [milestone 1-06]
  command/          dispatcher and chat command handlers  [milestone 1-07]
tasks/              milestone specs (see tasks/README.md)
ref/                original Python bot Dockerfiles (reference only, do not modify)
```

## Roadmap

See [`tasks/README.md`](tasks/README.md). Phase 1 delivers a deployable radio bot (milestones 1-01 through 1-09).
Phase 2 adds local file playback, yt-dlp, a web UI, and full parity with the original Python bot.
