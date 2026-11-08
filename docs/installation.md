# Installation

## Requirements

- `ffmpeg` on PATH (`ffprobe` too, for `!file` metadata)
- `yt-dlp` on PATH, only for `!url` / `!playlist`
- A JavaScript runtime for yt-dlp's YouTube support: [`deno`](https://deno.com) >= 2.3.0 (enabled by default),
  or node/QuickJS/bun via yt-dlp's `--js-runtimes`. The Docker image bundles yt-dlp and deno.

No Go toolchain, no Python runtime, no system libraries: the binary is fully static.

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
    restart: unless-stopped
```

Only put keys you want to override in `configuration.ini`; defaults are baked into the image.
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
