# 3-08 — Web Frontend

**Status:** todo  
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

## Thumbnails (deferred from 3-05)

3-05 stores no artwork. Decide here whether to serve any, e.g. extract on demand with
`file.ExtractThumb` and cache the result as files next to the DB (not inside it), exposed via an
endpoint added to the API. The UI must work without thumbnails.

## Deliverables

- `web/static/` sources and the embed wiring in `internal/web/server.go`
- Tests for static serving (index at `/`, correct content types, no directory listing)
- Docs: how to enable the UI and put it behind a reverse proxy

## Acceptance criteria

- With `web_enabled = true`, `http://<bind>/` shows the interface after logging in via `!web`
- Now-playing, playback controls, volume, mode, queue edit, add radio/URL, library browse and
  upload all work
- Two browsers editing the queue concurrently never act on the wrong item
- No JS console errors; the binary has no runtime dependency on Node or external assets
