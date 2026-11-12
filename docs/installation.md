# Installation

## Requirements

- `ffmpeg` on PATH (`ffprobe` too, for `!file` metadata)
- `yt-dlp` on PATH, only for `!url` / `!playlist`. Optional, and best effort: see [URL playback](commands/index.md#url-playback)
- A JavaScript runtime for yt-dlp's YouTube support: [`deno`](https://deno.com) >= 2.3.0 (enabled by default),
  or node/QuickJS/bun via yt-dlp's `--js-runtimes`. The Docker image bundles yt-dlp and deno.

!!! warning "Keep yt-dlp up to date"
    Sites, YouTube above all, change often and old yt-dlp versions stop working. With the binary run
    `yt-dlp -U` now and then. The Docker image contains one fixed version (`YTDLP_VERSION` in the
    `Dockerfile`): rebuild with a newer one when `!url` starts failing.

No Go toolchain and no Python runtime are needed.

!!! note "Native libraries"
    The downloadable Linux binaries are built with CGO and link dynamically, so the host needs the
    Opus library (`libopus`, e.g. `apt install libopus0`). The Docker image is statically linked
    and needs nothing extra.

## Binary

Pre-built binaries are published on every commit to `master` on the
[releases page](https://github.com/creadicted/gotamusique/releases).

```ini
# configuration.ini, next to the binary
[server]
host = mumble.example.com
port = 64738
channel = Music

[bot]
username = gotamusique
admin = YourMumbleUsername

[radio]
jazz = http://jazz-wr04.ice.infomaniak.ch/jazz-wr04-128.mp3 "Jazz Yeah!"
```

```sh
chmod +x gotamusique-linux-amd64
./gotamusique-linux-amd64
```

## Docker

Images are published to GHCR on every `v*` release tag: `ghcr.io/creadicted/gotamusique:<version>`,
`:<major>.<minor>` and `:latest` (pre-releases do not move `:latest`).

```yaml
services:
  gotamusique:
    image: ghcr.io/creadicted/gotamusique:latest
    volumes:
      - ./configuration.ini:/app/configuration.ini:ro
      # keeps the music library database across container recreation
      - ./data:/app/data
    restart: unless-stopped
```

Only put keys you want to override in `configuration.ini`; defaults are baked into the image.
If you use the [music library](configuration/library.md), set `music_database_path = data/music.db`
so the database lands on the mounted volume.
To use another config path:

```sh
docker run -v ./my.ini:/app/my.ini ghcr.io/creadicted/gotamusique:latest --config my.ini
```

## Build from source

```sh
git clone https://github.com/creadicted/gotamusique
cd gotamusique
make build   # produces bin/gotamusique
make test
```
