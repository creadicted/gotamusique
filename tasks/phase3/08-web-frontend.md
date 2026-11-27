# 3-08 — Web Frontend

**Status:** done  
**Depends on:** 3-07  
**Unlocks:** nothing

## Objective

A new, small frontend for the `/api/v1` API from 3-07, embedded in the binary. It is a remote
control for people in the Mumble channel: see what plays, steer playback, edit the queue, add
radio/URLs, browse the library and upload music. The botamusique frontend is **not** reused or
adapted.

## Approach

- Plain HTML/JS (or something light such as htmx/Preact), **no Node toolchain** and no build
  step, so the single-binary goal holds. Sources live in the repo (e.g. `web/static/`).
- Embedded with `//go:embed` and served at `/`; API stays under `/api/v1`.
- State comes from polling `GET /api/v1/state` (using its `version` / `If-None-Match` so
  unchanged polls are cheap). Every mutating call returns the new snapshot; apply it directly.
- A `404` on a queue entry ID means "already done": refresh, no error toast.
- Login is the `!web` link (`/login?token=…`) from 3-07; show a "type `!web` in Mumble" hint
  when unauthenticated.
- UI text is English first; i18n is not part of this milestone.

## As built

- Plain ES modules and one stylesheet under `internal/web/ui/` (see its README), served from an
  explicit file table at `/` and `/ui/...`. The page is public; it asks the API and shows the login
  steps on a 401. The Archivo font is embedded (OFL).
- Design: light timetable paper with one dark board for what is playing, amber only on dark
  surfaces, condensed bold for the board and queue; one animation (the title decoding on a track
  change), skipped for reduced motion.
- Polling uses `?since=<version>` (a `304` costs no body); the playhead advances locally between
  polls; mutations apply the returned state at once. A `404` on an entry refreshes silently.
- Added `GET /api/v1/radio/presets`, because the interface needs the configured stations.
- Searching in the Library tab covers the whole library even when a folder is open: the folder is
  not shown while searching, so limiting it would hide results.
- `cmd/uidemo` (`make ui-demo`) serves the interface with a fake bot, for development and for
  trying it without Mumble. It is not part of the release.

## Thumbnails (deferred from 3-05)

**Decision: not built.** 3-05 stores no artwork, and serving it means extracting art on demand
(`file.ExtractThumb`), caching it as files next to the DB, and a new endpoint, for something the
remote-control job does not need. The interface works without it. If wanted later, it fits as an
endpoint plus an `<img>` in the queue and library rows.

## Deliverables

- `internal/web/ui/` sources and the embed wiring in `internal/web/ui.go`
- Tests for static serving (index at `/`, correct content types, no directory listing)
- Docs: how to enable the UI and put it behind a reverse proxy

## Acceptance criteria

- With `web_enabled = true`, `http://<bind>/` shows the interface after logging in via `!web`
- Now-playing, playback controls, volume, mode, queue edit, add radio/URL, library browse and
  upload all work
- Two browsers editing the queue concurrently never act on the wrong item
- No JS console errors; the binary has no runtime dependency on Node or external assets
