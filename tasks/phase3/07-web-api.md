# 3-07 — Web REST API

**Status:** done — PRs 1 (#41), 2 (#42), 3 (#43) and 4 (library browse and upload)  
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

The `[webinterface]` section and key names are botamusique's, so existing files load; its other
keys are accepted and ignored. Defaults are safe: nothing listens after an upgrade.

| Key | Default | Notes |
|---|---|---|
| `enabled` | `False` | Off by default. |
| `listening_addr` / `listening_port` | `127.0.0.1` / `8181` | Docker binds `0.0.0.0` explicitly. A busy port stops the start. |
| `access_address` | `http://127.0.0.1:8181` | Public URL; the `!web` link is built from it. The cookie `Secure` flag is derived from it being `https://` (never from `X-Forwarded-*`). |
| `auth_method` | `token` | `token` or `none`. Anything else, including botamusique's `password`, is a startup error. `none` logs a warning when listening beyond loopback and enforces the `Host` check (DNS rebinding). |

Admins reuse `[bot] admin`; the web `admin` role additionally requires a registered Mumble
account, because the list matches names only. Upload keys (`upload_enabled`,
`max_upload_file_size`, upload dir) arrive with PR 4; `allow_upload_without_auth` is decided there.

TLS is not terminated by the bot; the admin puts a reverse proxy in front.

## Auth

**`none`:** no auth; every request has role `user`.

**`token`:**

1. A user in the bot's channel types `!web`.
2. The bot creates a random single-use token with a short expiry and replies privately with
   `<access_address>/login?token=…`.
3. Opening the link consumes the token and sets a session cookie (HttpOnly, SameSite=Lax,
   Secure when `access_address` is https).
4. The session subject is the cert hash when the user is registered, otherwise the name.
   Everyone gets `user`; `admin` comes from `[bot] admin` (registered accounts only).

Sessions live in memory with a sliding 12 h TTL and are not tied to Mumble presence. 3-11 can
revoke a user's sessions on ban. A restart logs everyone out.

## API (final)

| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/state` | Full snapshot: queue (with IDs), current item, playhead, paused, volume, mode, autoplay, library scan status, `version`. Supports `If-None-Match` / `?since=` → `304`. |
| POST | `/api/v1/playback/{pause,resume,stop,next}` | Playback control. |
| POST | `/api/v1/playback/seek` | Seek the current item (only if `Seekable`; otherwise `409`). |
| PUT | `/api/v1/volume` | `{"volume": 0..1}` and/or `{"muted": bool}`. |
| PUT | `/api/v1/mode` | `one-shot`, `repeat`, `random`, autoplay. |
| POST | `/api/v1/queue` | Add an item (`type`: `library` path, `radio` preset/url/uuid, `url` via yt-dlp), `position`: `end` / `next` / `now`. Answers `201` with the state and the new entry. |
| DELETE | `/api/v1/queue` | Clear. |
| POST | `/api/v1/queue/shuffle` | Shuffle the upcoming entries. |
| DELETE | `/api/v1/queue/{id}` | Remove an entry. |
| POST | `/api/v1/queue/{id}/play` | Jump to an entry. |
| POST | `/api/v1/queue/{id}/move` | Move after another entry ID. |
| GET | `/api/v1/radio/search` | radio-browser.info query (synchronous, 10 s timeout); add a hit by `uuid`. |
| GET | `/api/v1/library` | Browse a folder (`dir`: sub-folders with counts, files directly in it) or search (`q` words, `tag`s, below `dir`); paged (`limit` 1–500, `offset`); lists all tags. |
| POST | `/api/v1/library/rescan` | `202` / `409`; no `force`. The state's `scan` shows `running`, then `last_result` / `last_error`. |
| POST | `/api/v1/library/upload` | One audio file as multipart `file`; `201` with the new entry. |
| POST | `/api/v1/auth/logout` | End the session. |
| GET | `/login` | Consume a token, set the cookie, redirect to `/`. |

Every mutating call returns the new state snapshot. Errors are JSON `{error, code}`.

State-changing requests (anything but GET/HEAD/OPTIONS) are refused with `403 cross_origin` when the
browser marks them cross-site (`Sec-Fetch-Site`) or their `Origin` host differs from the `Host`
and from `access_address`. Without a login there is no cookie to rely on, so this is what stops
another website from driving a bot on the user's own network.

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
- Added while building it: the destination is resolved and must lie inside the music folder
  *before* anything is written (a symlinked `uploads/` cannot redirect it); the final name is
  reserved with `O_EXCL`; the body goes to a hidden temp file and is renamed into place; a file
  without an audio stream is refused; ffprobe runs with a 60 s timeout; errors shown to the user
  are generic (ffprobe's text contains server paths and only goes to the log). Keys reuse
  botamusique's `upload_enabled` and `max_upload_file_size`.

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

- `internal/web/` (flat package): `server.go`, `auth.go`, `login.go`, one file per endpoint
  group (`state.go`, ...), and the narrow `web.Bot` interface. Handlers are not split into a
  sub-package because they share the session and error helpers.
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
- With `enabled = False` the bot opens no port.
