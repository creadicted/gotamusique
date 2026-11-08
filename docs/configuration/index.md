# Configuration

Copy `configuration.default.ini` to `configuration.ini` next to the binary and edit what you need.
Missing keys fall back to the embedded defaults.

## `[server]`

| Key               | Default     | Description |
|-------------------|-------------|-------------|
| `host`            | `127.0.0.1` | Mumble server hostname or IP |
| `port`            | `64738`     | Mumble server port |
| `password`        | *(empty)*   | Server password |
| `channel`         | *(empty)*   | Channel to join on connect (root if empty) |
| `certificate`     | *(empty)*   | PEM client certificate for cert-auth |
| `tokens`          | *(empty)*   | Comma-separated access tokens |
| `tls_skip_verify` | `True`      | Skip TLS certificate verification |

## `[bot]`

| Key                        | Default           | Description |
|----------------------------|-------------------|-------------|
| `username`                 | `gotamusique`     | Display name in Mumble |
| `volume`                   | `0.8`             | Initial volume (0.0–1.0) |
| `max_volume`               | `1.0`             | Upper ceiling for `!volume` |
| `admin`                    | *(empty)*         | Comma-separated admin usernames |
| `comment`                  | `"I play radio."` | Bot comment |
| `announce_current_music`   | `True`            | Announce when a track starts |
| `logfile`                  | *(empty)*         | Log to this path instead of stderr |
| `max_track_duration`       | `60`              | Max `!url` track length in minutes (`0` = no limit) |
| `max_track_playlist`       | `20`              | Max tracks added by one `!playlist` |
| `playback_mode`            | `one-shot`        | `one-shot`, `repeat`, `random` or `autoplay` |
| `music_database_path`      | `music.db`        | Library database; opened only when `music_folder` is set |
| `refresh_cache_on_startup` | `False`           | Scan the music folder at startup |
| `autoplay_length`          | `5`               | Tracks added per autoplay refill |

## `[youtube_dl]`

| Key              | Default   | Description |
|------------------|-----------|-------------|
| `binary`         | *(empty)* | Path to yt-dlp; empty = from `PATH` |
| `cookiefile`     | *(empty)* | Cookies file |
| `source_address` | *(empty)* | Local IP to bind to |
| `user_agent`     | *(empty)* | Override the HTTP user agent |

## `[files]`

| Key               | Default   | Description |
|-------------------|-----------|-------------|
| `music_folder`    | *(empty)* | Root for `!file` and the library scan; paths cannot escape it, including via symlinks |
| `ignored_folders` | *(empty)* | Folder names the scan skips (exact, case-sensitive, any depth) |
| `ignored_files`   | *(empty)* | File names the scan skips (exact, case-sensitive) |

See [Music library](library.md) for how scanning works.

## `[radio]` presets

Each key is an alias; the value is a URL, optionally followed by a quoted display name.

```ini
[radio]
jazz = http://jazz-wr04.ice.infomaniak.ch/jazz-wr04-128.mp3 "Jazz Yeah!"
soma = http://ice1.somafm.com/groovesalad-128-mp3 "SomaFM Groove Salad"
```

## `[commands]` aliases

```ini
[commands]
command_symbol = !
volume = vol, v
play_radio = radio, r
```

## `[debug]`

```ini
[debug]
ffmpeg = True             # log ffmpeg stderr
mumble_connection = True  # verbose gumble logging
```

## Command-line overrides

```sh
./gotamusique --server my.mumble.server --username MusicBot --channel Radio
./gotamusique --help
```
