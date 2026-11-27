# 3-07 — Web REST API

**Status:** todo  
**Depends on:** 3-04, 3-05  
**Unlocks:** 3-08

## Objective

A clean, UI-agnostic JSON API under `/api/v1`, served by the Go binary, that lets users in the
Mumble channel control the bot from a browser: see what plays, steer playback, edit the queue,
add radio/URLs, browse the library and upload music.

The botamusique wire format (`/post`, `/playlist`, form-encoded actions) is **not** reproduced.
3-08 ships a new, small frontend written against this API (see that file).

## Decisions

| # | Decision |
|---|---|
| 1 | **Own API, no botamusique compatibility.** JSON bodies, resource-style routes, `/api/v1` prefix. |
| 2 | **Frontend (3-08) is new** and embedded with `go:embed`, no Node toolchain. The botamusique frontend is not adapted. |
| 3 | **Audience: users first.** The UI is a remote control plus upload for people in the channel. An admin view comes later (3-11) as a role mapping. |
| 4 | **Auth behind a session interface.** A session is `{subject, name, roles}`; handlers check roles, never the auth method. 3-07 ships `none` and `token`. `password` and IP-ban are dropped from this milestone. OIDC (roles from claims) is a later milestone behind the same interface. |
| 5 | **Polling, designed for SSE.** `GET /api/v1/state` returns one complete snapshot with an opaque `version`. SSE can push the same snapshot later without changing the contract. |
| 6 | **Slow operations: rescan is fire-and-poll.** `POST` returns `202` (or `409` if already running); progress/result appear in the state snapshot. Radio search stays synchronous with a timeout. No general job system. |
| 7 | **Web package depends on its own narrow `web.Bot` interface**, not `command.BotAPI`. Library DTOs move out of `internal/command` into a neutral package. |
| 8 | **Queue entries get stable IDs.** Edits address IDs, not indexes, so concurrent users cannot hit the wrong item. A missing ID is `404` ("already done, refresh"). No `If-Match`. |
| 9 | **Upload is in scope**; download, library edit/delete and thumbnails are not (see Out of scope). |

## Configuration

New keys (names indicative, finalise in PR 2). Defaults are safe: nothing listens after an upgrade.

| Key | Default | Notes |
|---|---|---|
| `web_enabled` | `false` | Off by default. |
| `web_bind` | `127.0.0.1:<port>` | Docker/compose binds `0.0.0.0` explicitly. |
| `web_base_url` | empty | Public URL used to build the `!web` link. The cookie `Secure` flag is derived from it being `https://` (never from `X-Forwarded-*`). |
| `auth_method` | `token` | `none` or `token`. `none` logs a startup warning when the bind address is not loopback. |
| `web_admins` | empty | Mumble names or cert hashes that get the `admin` role (used from 3-11). |
| `allow_upload_without_auth` | `false` | Upload is refused under `auth_method = none` unless set. |
| upload dir / max size | `uploads/`, set a limit | Under the music root. |

TLS is not terminated by the bot; the admin puts a reverse proxy in front.

## Auth

**`none`:** no auth; every request has role `user`.

**`token`:**

1. A user in the bot's channel types `!web`.
2. The bot creates a random single-use token with a short expiry and replies privately with
   `<web_base_url>/login?token=…`.
3. Opening the link consumes the token and sets a session cookie (HttpOnly, SameSite=Lax,
   Secure when `web_base_url` is https).
4. The session subject is the cert hash when the user is registered, otherwise the name.
   Everyone gets `user`; `admin` comes from `web_admins`.

Sessions live in memory with a sliding 12 h TTL and are not tied to Mumble presence. 3-11 can
revoke a user's sessions on ban. A restart logs everyone out.

## API (draft, finalise in PR 2/3/4)

| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/state` | Full snapshot: queue (with IDs), current item, playhead, paused, volume, mode, autoplay, library scan status, `version`. Supports `If-None-Match` / `?since=` → `304`. |
| POST | `/api/v1/playback/{pause,resume,stop,next}` | Playback control. |
| POST | `/api/v1/playback/seek` | Seek the current item (only if `Seekable`; otherwise `409`). |
| PUT | `/api/v1/volume` | Set volume. |
| PUT | `/api/v1/mode` | `one-shot`, `repeat`, `random`, autoplay. |
| POST | `/api/v1/queue` | Add an item (library file, radio, URL), at end / next / play now. |
| DELETE | `/api/v1/queue` | Clear. |
| DELETE | `/api/v1/queue/{id}` | Remove an entry. |
| POST | `/api/v1/queue/{id}/play` | Jump to an entry. |
| POST | `/api/v1/queue/{id}/move` | Move after another entry ID. |
| GET | `/api/v1/radio/search` | radio-browser.info query (synchronous, timeout). |
| GET | `/api/v1/library` | Browse/search files, tags, dirs. |
| POST | `/api/v1/library/rescan` | `202` / `409`. |
| POST | `/api/v1/library/upload` | Upload an audio file. |
| POST | `/api/v1/auth/logout` | End the session. |
| GET | `/login` | Consume a token, set the cookie, redirect to `/`. |

Every mutating call returns the new state snapshot. Errors are JSON `{error, code}`.

## Upload

- Destination `uploads/<username>/` under the music root. This is the attribution for now; no
  schema change.
- Extension allowlist, then ffprobe on the temp file to confirm it is real audio; failures are
  deleted.
- Filename sanitised (basename only, no separators, no leading dots); collisions are renamed,
  never overwritten. No path traversal.
- `http.MaxBytesReader` size limit. No per-user quota in this milestone.
- The file is indexed immediately via a new `Scanner.IndexFile(ctx, path)` (reuses `buildRecord`
  and the ffprobe step; the scan path is unchanged). The response returns the new library item.
- Permission: role `user`; under `auth_method = none` only with `allow_upload_without_auth`.

## Delivery: four PRs

1. **Prerequisites** (no HTTP): queue entry IDs, `Queue.Move`, `Queue` by-ID lookups,
   `Bot.Snapshot()` (consistent, taken under one lock), `Bot.Seek`, `Bot.RemoveByID`/`Move`,
   `Scanner.IndexFile`, library DTOs moved out of `internal/command`. New mutators go into the
   `TestVersion_EveryMutatorBumps` table. Decide whether `Next()` skips the version bump when the
   index does not change (today it always bumps); a rejected call may still bump, so a changed
   version always means "re-read".
2. **HTTP skeleton, auth, read-only state:** config, server, session store with roles, `!web`
   command, `GET /api/v1/state`, `/login`, logout.
3. **Control and queue endpoints:** playback, seek, volume, mode, queue edit by ID, add radio /
   URL, radio search, fire-and-poll rescan.
4. **Library read and upload:** browse/search, upload with validation, `IndexFile` wiring.

The milestone is done after PR 4. Each PR ships with tests and updated docs/CHANGELOG.

## Out of scope (follow-ups)

- `GET /download` (file / zip), library edit and delete: largest security surface; for the admin
  view later.
- Thumbnails: 3-05 stores no artwork. Decide in 3-08 (e.g. extract on demand with
  `file.ExtractThumb`, cache as files next to the DB, not in it).
- `password` auth and IP ban after `max_attempts`; OIDC.
- SSE (`/api/v1/events`), per-user upload quotas, general job system.

## Deliverables

- `internal/web/server.go`, `internal/web/auth.go`, `internal/web/handlers/` (one file per
  endpoint group), `web.Bot` interface in `internal/web`.
- Tests: every handler against a mock `web.Bot` with `httptest`; upload validation and
  path-traversal cases; session and token expiry/single-use; queue ID concurrency.
- Docs: configuration reference, `!web`, reverse-proxy note (TLS, timeouts).

## Acceptance criteria

- `GET /api/v1/state` returns a consistent snapshot while a track plays, and `304` when unchanged.
- `POST /api/v1/playback/pause` pauses playback and returns the new snapshot.
- A `!web` token works exactly once and expires; the session cookie persists across requests.
- Removing a queue entry that is already gone returns `404`, and no other entry is affected.
- `POST /api/v1/library/rescan` returns `202`, then `409` while running; the result appears in
  the state.
- An uploaded valid audio file appears in the library and can be queued immediately; invalid
  files and traversal attempts are rejected and leave nothing on disk.
- With `web_enabled = false` the bot opens no port.
