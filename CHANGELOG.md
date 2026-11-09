# Changelog

All notable changes to this project will be documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

---

## [Unreleased]

### Fixed

- **radio-browser.info servers are now discovered through DNS** (`all.api.radio-browser.info`),
  shuffled, and tried before the built-in list. `nl1` and `at1` no longer resolve, so with the old
  fixed list only `de1` still worked. The built-in list stays as a fallback if the lookup fails.

---

## [0.6.0] — 2026-11-09

### Added

- **`!resume` after a restart** — the bot remembers the last item it played (`last_played_path`,
  default `last_played.json`, empty = off) and `!resume` plays it again when nothing is paused and
  the queue is empty: a local file from the saved position, a radio or HLS stream live, a yt-dlp
  track from the start. Once per start, never over a queue you built. Saved when a track starts,
  on pause and on clean shutdown; a missing file or unset `music_folder` restores nothing.

### Changed

- **`!resume` continues a local file from where it was paused** instead of restarting it. The bot
  keeps the position (audio sent to Mumble) and restarts ffmpeg at that offset (`-ss`); the
  announcement says `(resumed at m:ss)`. Auto-pause resumes the same way. Live streams still rejoin
  live, and yt-dlp tracks still restart from the beginning. `!skip`, `!clear`, `!stop` and playing
  something new drop the saved position.

---

## [0.5.1] — 2026-11-08

### Added

- **`!version`** — reports the running version; the startup log shows it too.

### Fixed

- Release binaries and Docker images reported a hard-coded `0.1.9` as their version. The version
  is now injected at build time from the release tag (`make build` uses `git describe`); a plain
  `go build` reports `dev (<revision>)`.

---

## [0.5.0] — 2026-11-08

### Added

- **`!pause` and `!resume`** — halt playback without losing the queue position (`!stop` still rewinds
  to the first item). Resuming a live stream rejoins it; there is nothing to seek back to. A paused
  bot runs no ffmpeg. `!skip`, `!clear`, `!stop` and starting a new track end a pause.
- **Auto-pause when alone** — `auto_pause_when_alone` (default `False`) pauses when the last other
  user leaves the bot's channel; `auto_resume_on_join` (default `True`) resumes when someone is
  back. Only an automatic pause is undone, never one from `!pause`.

---

## [0.4.0] — 2026-11-08

### Added

- **Documentation site** — MkDocs Material site (installation, configuration, commands,
  migrating) published to GitHub Pages; `make docs` previews it and `make docs-build` checks it.
  The README now points to it.
- **Queue and file commands** — `!file <path|folder|part of a path>` (alias `!f`) now searches
  the library: an exact path, else a folder (queues every track below it), else a
  case-insensitive partial match (one hit is queued, several are listed for `!shortlist`);
  without a library, or when nothing matches, it still plays the path from `music_folder`.
  `!filematch <regex>` (alias `!fm`) queues every library track whose path or title matches.
  `!last` jumps to the last queued item, `!random` shuffles the upcoming items without
  interrupting the current track, and `!repeat [n]` plays the current item `n` more times
  (1-200). Bulk commands (`!file` folder, `!filematch`, `!tag`, `!shortlist *`) queue at most
  200 tracks at once and say so when more matched.
  When `!file` lists several partial matches, `!shortlist -p <page>` shows the other pages.
- **`!listfile` (alias `!list`) `[regex] [-p page]` and `!search <keywords> [-p page]`** — browse the scanned
  music library from chat. `!listfile` lists tracks ordered by path, optionally filtered by a
  case-insensitive regex on path and title; `!search` returns tracks matching every keyword
  (`%` and `_` match literally). Pages are sized to stay under Mumble's message limit. Paths
  shown can be played with `!file`.
- **Tags and `!shortlist`** — `!listfile`, `!search` and `!findtagged <tag,tag>` (alias `!ft`)
  number their results (across pages) and remember them per channel; `!shortlist <n> [n ...]|*`
  (alias `!sl`) queues entries from that list, and queues nothing if any number is bad.
  `!addtag [n|*] <tag,tag>` and `!untag [n|*] <tag,tag|*>` tag queued library files (current
  item by default, `n` = queue position, `*` = all); radio, URL and stream items are skipped.
  `!tag <tag,tag>` queues every library track carrying all the tags. Tags match whole words
  case-insensitively and are kept when a changed file is re-probed by `!rescan`.

---

## [0.3.0] — 2026-11-07

### Added

- **Music library and `!rescan`** — with `[files] music_folder` set, `!rescan` scans the folder
  into a SQLite database (`[bot] music_database_path`, default `music.db`). The scan is
  incremental (size and modified time are stored, only new or changed files are probed),
  skips non-audio files, honours `ignored_folders` / `ignored_files`, never follows directory
  symlinks, and runs ffprobe only (no artwork). It never wipes the library because the folder is
  unavailable (`!rescan force` clears the library on purpose), and only ever deletes rows of type `file`. `[bot] refresh_cache_on_startup`
  runs one in the background at startup. The database opens only when `music_folder` is set; if
  it cannot be opened the bot logs an error and runs without a library.
- **`!mode autoplay`** — plays random tracks from the library. It starts immediately on an
  idle bot, refills the queue when it runs dry (replacing played tracks so the queue stays
  short; `autoplay_length` tracks per refill), and stops itself after `autoplay_length`
  consecutive failures. Skips rows whose file no longer exists. Running `!mode autoplay` again
  restarts it.
- **`!mode [one-shot|repeat|random]` command and `[bot] playback_mode` setting** — `repeat`
  loops the queue. `random` shuffles the upcoming items, inserts newly queued tracks at a random
  position after the current one, and reshuffles when the list ends (the track that just played
  never starts the next lap). An invalid
  `playback_mode` in the config logs a warning and falls back to `one-shot`. The mode is not
  saved across restarts.
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
- **`!file <relative/path>` command** — play a local audio file from the new `[files]
  `music_folder`. The path is validated lexically and after resolving symlinks, so it cannot
  escape the folder. Title, artist, duration and embedded artwork (re-encoded to PNG) are read
  with ffprobe/ffmpeg. Requires `ffprobe` on `PATH`.
- **`!hls <url>` command** — play HLS/M3U8 streams directly. The stream is announced
  as `[HLS] <name>` where name is derived from the M3U8 filename in the URL path,
  falling back to the hostname.
- **ffmpeg reconnect flags** for all HTTP/HTTPS streams — transient network
  interruptions now recover automatically instead of dropping the stream.
  Flags applied: `-reconnect`, `-reconnect_streamed`, `-reconnect_delay_max 5`,
  `-reconnect_at_eof`, `-reconnect_on_http_error 4xx,5xx`.

### Fixed

- A queue in which every track fails to start no longer retries forever. After every entry has
  failed the bot goes idle (queue kept) instead of resetting to the first track and starting
  over, which spammed the channel in every mode. Only distinct failed entries count, so a
  `random` reshuffle retrying the same broken tracks no longer stops a queue that still has
  playable ones.
- A file that ffmpeg cannot open or decode (e.g. deleted after it was queued) is now treated as
  a failed track. Before, it counted as a track that played, so `repeat` and `random` looped on
  it forever.
- A failing track can no longer skip a track you just selected (`!play`, `!skip`) or one that was
  queued while it was failing.

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
