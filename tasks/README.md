# Go Migration Tasks

**Version: 0.1.7** — Phase 1 complete

Rewrite of botamusique in Go, delivered in phases.

> The original Python codebase is not included in this repository snapshot.
> All new Go code lives at the repo root.

## Phase 1 — Online Radio (MVP)

**Goal:** A single Go binary that connects to a Mumble server and streams internet radio.  
No database, no file library, no web UI, no yt-dlp.

| #    | File                                                             | Status | Description                              |
|------|------------------------------------------------------------------|--------|------------------------------------------|
| 1-01 | [phase1/01-scaffold.md](phase1/01-scaffold.md)                   | done   | Go module, layout, Makefile              |
| 1-02 | [phase1/02-config.md](phase1/02-config.md)                       | done   | INI config (server + radio presets only) |
| 1-03 | [phase1/03-mumble-connection.md](phase1/03-mumble-connection.md) | done   | Connect, join channel, SIGINT shutdown   |
| 1-04 | [phase1/04-audio-pipeline.md](phase1/04-audio-pipeline.md)       | done   | ffmpeg → PCM → Mumble audio output       |
| 1-05 | [phase1/05-radio-media.md](phase1/05-radio-media.md)             | done   | HTTP stream item, radio-browser.info API |
| 1-06 | [phase1/06-queue.md](phase1/06-queue.md)                         | done   | Simple in-memory queue + play/stop/skip  |
| 1-07 | [phase1/07-commands.md](phase1/07-commands.md)                   | done   | Chat command dispatcher + radio commands |
| 1-08 | [phase1/08-docker.md](phase1/08-docker.md)                       | done   | Dockerfile + docker-compose              |
| 1-09 | [phase1/09-ghcr.md](phase1/09-ghcr.md)                          | done   | Publish image to GHCR on release tag     |

## Phase 2 — Incremental Enhancements

**Goal:** Targeted improvements that build on the Phase 1 binary without requiring the
full Phase 3 subsystems (database, library, web UI). Each milestone is independently
mergeable.

| #    | File                                          | Status | Description                        |
|------|-----------------------------------------------|--------|------------------------------------|
| 2-01 | [phase2/01-hls-m3u8.md](phase2/01-hls-m3u8.md) | todo   | Auto-detect HLS/M3U8 streams in `!radio` |

## Phase 3 — Full Bot

**Goal:** Parity with the original Python bot.  
Builds on the Phase 1 binary; each milestone is independently mergeable.

| #    | File                                                         | Status | Description                            |
|------|--------------------------------------------------------------|--------|----------------------------------------|
| 3-01 | [phase3/01-database.md](phase3/01-database.md)               | todo   | SQLite settings + music DB, migration  |
| 3-02 | [phase3/02-file-media.md](phase3/02-file-media.md)           | todo   | Local file playback (ffprobe metadata) |
| 3-03 | [phase3/03-url-media.md](phase3/03-url-media.md)             | todo   | YouTube / yt-dlp integration           |
| 3-04 | [phase3/04-playlist-modes.md](phase3/04-playlist-modes.md)   | todo   | repeat / random / autoplay modes       |
| 3-05 | [phase3/05-music-library.md](phase3/05-music-library.md)     | todo   | Dir scan, DB cache, tags               |
| 3-06 | [phase3/06-full-commands.md](phase3/06-full-commands.md)     | todo   | All remaining chat commands            |
| 3-07 | [phase3/07-web-api.md](phase3/07-web-api.md)                 | todo   | REST API for web remote control        |
| 3-08 | [phase3/08-web-frontend.md](phase3/08-web-frontend.md)       | todo   | Serve existing frontend from binary    |
| 3-09 | [phase3/09-ducking.md](phase3/09-ducking.md)                 | todo   | Auto volume-lower on voice activity    |
| 3-10 | [phase3/10-persistence.md](phase3/10-persistence.md)         | todo   | Save/restore playlist across restarts  |
| 3-11 | [phase3/11-admin.md](phase3/11-admin.md)                     | todo   | Ban/whitelist, admin-only commands     |

## Feature Requests

User-requested improvements that are scoped and ready to implement.  
Each FR lives in `feature-requests/` and must ship with updated docs and tests.

| #     | File | Status | Description |
|-------|------|--------|-------------|
| FR-01 | [feature-requests/FR-01-rbquery-index-and-limit.md](feature-requests/FR-01-rbquery-index-and-limit.md) | open | `!rbquery` numeric index for `!rbplay` + optional result count |
| FR-02 | [feature-requests/FR-02-pause-and-auto-pause-when-alone.md](feature-requests/FR-02-pause-and-auto-pause-when-alone.md) | open | `!pause`/`!resume` (keep queue position) + auto-pause when alone in channel |
| FR-03 | [feature-requests/FR-03-local-file-hls-random-folder-playback.md](feature-requests/FR-03-local-file-hls-random-folder-playback.md) | open | Local file, HLS/M3U8, and random-folder playback (pending clarifying questions) |

## Future

Ideas that are technically feasible but not prioritised — captured here to avoid re-investigating.

| Idea | Description |
|---|---|
| Spotify integration | Resolve `open.spotify.com` track/playlist/album URLs to track metadata via the Spotify Web API (Client Credentials flow, no Premium required), then hand off to yt-dlp for audio. Adds setup overhead (users must register a Spotify Developer App and supply credentials) for a benefit that is mostly limited to bulk-importing playlists. Low priority until there is clear user demand. |

## Key libraries

| Purpose               | Library                   |
|-----------------------|---------------------------|
| Mumble protocol       | `github.com/layeh/gumble` |
| INI config            | `gopkg.in/ini.v1`         |
| SQLite (phase 3)      | `modernc.org/sqlite`      |
| HTTP server (phase 3) | stdlib `net/http`         |

ffmpeg and yt-dlp (phase 3) are external binaries via `os/exec`.

## Status values

`todo` → `in-progress` → `done` → `blocked`
