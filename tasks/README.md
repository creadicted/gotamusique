# Go Migration Tasks

**Version: 0.1.7** — Phase 1 in progress (milestones 1-01 through 1-07 complete)

Rewrite of botamusique in Go, delivered in two phases.

> The original Python codebase (botamusique) has been preserved in [`ref/`](../ref/) for reference.
> All new Go code lives at the repo root. Do not modify files under `ref/`.

## Phase 1 — Online Radio (MVP)

**Goal:** A single Go binary that connects to a Mumble server and streams internet radio.  
No database, no file library, no web UI, no yt-dlp.  
After milestone 1-09 the bot is deployable and useful.

| #    | File                                                             | Status | Description                              |
|------|------------------------------------------------------------------|--------|------------------------------------------|
| 1-01 | [phase1/01-scaffold.md](phase1/01-scaffold.md)                   | done   | Go module, layout, Makefile              |
| 1-02 | [phase1/02-config.md](phase1/02-config.md)                       | done   | INI config (server + radio presets only) |
| 1-03 | [phase1/03-mumble-connection.md](phase1/03-mumble-connection.md) | done   | Connect, join channel, SIGINT shutdown   |
| 1-04 | [phase1/04-audio-pipeline.md](phase1/04-audio-pipeline.md)       | done   | ffmpeg → PCM → Mumble audio output       |
| 1-05 | [phase1/05-radio-media.md](phase1/05-radio-media.md)             | done   | HTTP stream item, radio-browser.info API |
| 1-06 | [phase1/06-queue.md](phase1/06-queue.md)                         | done   | Simple in-memory queue + play/stop/skip  |
| 1-07 | [phase1/07-commands.md](phase1/07-commands.md)                   | done   | Chat command dispatcher + radio commands |
| 1-08 | [phase1/08-docker.md](phase1/08-docker.md)                       | todo   | Dockerfile + docker-compose              |
| 1-09 | [phase1/09-ghcr.md](phase1/09-ghcr.md)                          | todo   | Publish image to GHCR on release tag     |

## Phase 2 — Full Bot

**Goal:** Parity with the original Python bot.  
Builds on the Phase 1 binary; each milestone is independently mergeable.

| #    | File                                                       | Status | Description                            |
|------|------------------------------------------------------------|--------|----------------------------------------|
| 2-01 | [phase2/01-database.md](phase2/01-database.md)             | todo   | SQLite settings + music DB, migration  |
| 2-02 | [phase2/02-file-media.md](phase2/02-file-media.md)         | todo   | Local file playback (ffprobe metadata) |
| 2-03 | [phase2/03-url-media.md](phase2/03-url-media.md)           | todo   | YouTube / yt-dlp integration           |
| 2-04 | [phase2/04-playlist-modes.md](phase2/04-playlist-modes.md) | todo   | repeat / random / autoplay modes       |
| 2-05 | [phase2/05-music-library.md](phase2/05-music-library.md)   | todo   | Dir scan, DB cache, tags               |
| 2-06 | [phase2/06-full-commands.md](phase2/06-full-commands.md)   | todo   | All remaining chat commands            |
| 2-07 | [phase2/07-web-api.md](phase2/07-web-api.md)               | todo   | REST API for web remote control        |
| 2-08 | [phase2/08-web-frontend.md](phase2/08-web-frontend.md)     | todo   | Serve existing frontend from binary    |
| 2-09 | [phase2/09-ducking.md](phase2/09-ducking.md)               | todo   | Auto volume-lower on voice activity    |
| 2-10 | [phase2/10-persistence.md](phase2/10-persistence.md)       | todo   | Save/restore playlist across restarts  |
| 2-11 | [phase2/11-admin.md](phase2/11-admin.md)                   | todo   | Ban/whitelist, admin-only commands     |

## Future

Ideas that are technically feasible but not prioritised — captured here to avoid re-investigating.

| Idea | Description |
|---|---|
| Spotify integration | Resolve `open.spotify.com` track/playlist/album URLs to track metadata via the Spotify Web API (Client Credentials flow, no Premium required), then hand off to yt-dlp for audio. Adds setup overhead (users must register a Spotify Developer App and supply credentials) for a benefit that is mostly limited to bulk-importing playlists. Low priority until there is clear user demand. |

## Key libraries

| Purpose               | Library                   |
|-----------------------|---------------------------|
| Mumble protocol       | `github.com/layeh/gumble` |
| SQLite (phase 2)      | `modernc.org/sqlite`      |
| INI config            | `gopkg.in/ini.v1`         |
| HTTP server (phase 2) | stdlib `net/http`         |

ffmpeg and yt-dlp (phase 2) are external binaries via `os/exec`.

## Status values

`todo` → `in-progress` → `done` → `blocked`
