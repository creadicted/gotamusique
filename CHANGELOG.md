# Changelog

All notable changes to this project will be documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

---

## [0.2.1] — pre-release

### Added

- **`!url <url>` and `!playlist <url>` commands** — play audio from YouTube, SoundCloud and
  other yt-dlp sites. Audio is always streamed (no downloads); the direct stream URL is
  resolved just before a track plays. `!url` queues exactly one track; `!playlist` queues
  up to `max_track_playlist` entries and checks each when it plays. Tracks longer than
  `max_track_duration` are rejected or skipped. Only `http`/`https` URLs are accepted.
  New `[youtube_dl]` config section; the Docker image bundles a pinned yt-dlp binary and deno
  (base image bumped to Alpine 3.22 for deno >= 2.3.0).
  The HTTP headers and cookies yt-dlp reports for a stream (Referer, User-Agent, session
  cookies) are passed to ffmpeg, so sites that require them play correctly.
  Finite URL tracks play to the end instead of using ffmpeg's reconnect-at-EOF; skip/clear/stop
  during a pending resolve no longer launches or skips the wrong track.
- **`!hls <url>` command** — play HLS/M3U8 streams directly. The stream is announced
  as `[HLS] <name>` where name is derived from the M3U8 filename in the URL path,
  falling back to the hostname.
- **ffmpeg reconnect flags** for all HTTP/HTTPS streams — transient network
  interruptions now recover automatically instead of dropping the stream.
  Flags applied: `-reconnect`, `-reconnect_streamed`, `-reconnect_delay_max 5`,
  `-reconnect_at_eof`, `-reconnect_on_http_error 4xx,5xx`.

### Changed

- `!radio` is unchanged and always produces a `[Radio]` item — there is no
  auto-detection of `.m3u8` URLs in `!radio`. Use `!hls` explicitly for HLS playlists.
- Upgraded all GitHub Actions to Node 24 runtime (`actions/checkout@v7`,
  `actions/setup-go@v7`, `actions/upload-artifact@v7`, `actions/download-artifact@v8`,
  `docker/*@v4–v7`, `softprops/action-gh-release@v3`).

### Internal

- Extracted `buildFFmpegArgs(url, verbosity string) []string` from `pipeline.go`
  for testability; covered by `pipeline_args_test.go`.
- New package `internal/hls` with `HLSItem` implementing `audio.MediaItem`.

---

## [0.2.0] — Phase 1 complete

Radio streaming MVP. Single statically-linked binary connecting to a Mumble server
and streaming internet radio via ffmpeg.

### Added

- Mumble connection with SIGINT shutdown
- ffmpeg audio pipeline (PCM → gumble, volume control, fade in/out)
- HTTP radio stream playback (`!radio`)
- radio-browser.info integration (`!rbquery`, `!rbplay`)
- In-memory play queue with skip, stop, clear
- Chat command dispatcher with configurable aliases and symbol prefix
- INI config with two-file merge (defaults + user overlay)
- Dockerfile + docker-compose
- GHCR image published on release tags
