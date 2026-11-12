# Web interface

The bot can serve an HTTP API (and, in a later release, a web UI) so people in the Mumble
channel can follow and control playback from a browser. It is **off by default**: nothing
listens until you enable it.

!!! note "Work in progress"
    This release ships the login, the state and the control API (playback, volume, mode, queue,
    adding radio and URLs, library rescan). Library browsing and uploads follow in the next
    release, and the browser UI after that.

## Enable it

```ini
[webinterface]
enabled = True
listening_addr = 127.0.0.1
listening_port = 8181
access_address = https://music.example.com
auth_method = token
```

| Key              | Default                 | Description |
|------------------|-------------------------|-------------|
| `enabled`        | `False`                 | Start the web interface |
| `listening_addr` | `127.0.0.1`             | Address to listen on. Loopback only by default; use `0.0.0.0` in Docker and publish the port |
| `listening_port` | `8181`                  | Port to listen on. The bot exits at startup if it is already taken |
| `access_address` | `http://127.0.0.1:8181` | The public URL people open, an absolute `http://` or `https://` URL with a host (a path is fine, credentials and a query are not). The link from `!web` is built from it. An `https://` URL marks the session cookie *Secure*. The bot refuses to start with `enabled = True` and a missing or malformed value |
| `auth_method`    | `token`                 | `token` or `none` (see below) |

The section name and these keys are the same as in botamusique, so an existing file loads. Its
other keys (`user`, `password`, `max_attempts`, `flask_secret`, `is_web_proxified`,
`max_upload_file_size`, `upload_enabled`, `web_logfile`) are accepted and ignored.
`auth_method = password` is **not supported** and stops the bot at startup, rather than silently
serving without the login you asked for.

## Logging in (`auth_method = token`)

1. Type `!web` in the Mumble channel.
2. The bot sends you a **private message** with a login link. The link works once and expires after
   5 minutes. Asking again invalidates the previous link, and a link that could not be sent to you
   is cancelled at once.
3. Open it. You get a session cookie (HttpOnly, SameSite=Lax) that stays valid while you use it, up
   to 12 hours idle and 7 days in total. Restarting the bot logs everyone out.

Anyone in the bot's channel can request a link, so access to the web interface is as open as
access to the channel. A user with a certificate keeps the same identity if they rename
themselves; without one the identity is the name.

## No login (`auth_method = none`)

Everyone who can reach the address controls the bot. Only use it on a network you trust. The bot
logs a warning if it listens beyond loopback in this mode, and it refuses requests whose `Host`
header is not `access_address` or a loopback name (this defends against DNS rebinding, so set
`access_address` to the address you actually use).

## Reverse proxy and TLS

The bot speaks plain HTTP. For HTTPS put a reverse proxy in front, set `access_address` to its
public URL and keep `listening_addr` on loopback:

```nginx
server {
    listen 443 ssl;
    server_name music.example.com;
    location / {
        proxy_pass http://127.0.0.1:8181;
        proxy_set_header Host $host;
    }
}
```

Forward the original `Host` header (as above). The bot refuses state-changing requests whose
`Origin` is a different site than the `Host` it was sent to, or than `access_address`.

## Docker

Bind to all interfaces inside the container and publish the port:

```ini
[webinterface]
enabled = True
listening_addr = 0.0.0.0
access_address = http://your-host:8181
```

```yaml
services:
  gotamusique:
    ports:
      - "127.0.0.1:8181:8181"   # drop the 127.0.0.1: to expose it to the network
```

## API

All endpoints are under `/api/v1`, speak JSON and need a session (except in `none` mode). Errors
look like `{"error": "...", "code": "not_found"}`; the `code` is stable, the text is for humans.
Every call that changes something answers with the new state, so a client can apply the answer
instead of polling again. Some controls act asynchronously (a skip interrupts audio first); the
next poll shows the final state.

### State

`GET /api/v1/state` returns the whole state in one response: queue (each entry has a stable `id`),
position, volume, mode and scan status. Its `version` changes whenever the state changes (the
playhead position excluded), and it is also sent as an `ETag`, so polling with
`If-None-Match: "<version>"` (or `?since=<version>`) costs a `304` while nothing happened.

### Playback

| Request | Body | Effect |
|---------|------|--------|
| `POST /playback/pause`, `/resume`, `/stop`, `/next` | | Same as `!pause`, `!resume`, `!stop`, `!skip` |
| `POST /playback/seek` | `{"position_ms": 90000}` | Restart the current track at that position. Only local files seek (`409 not_seekable` otherwise) |
| `PUT /volume` | `{"volume": 0.4}` and/or `{"muted": true}` | Volume is 0 to 1 and ends a mute |
| `PUT /mode` | `{"mode": "repeat"}` | `one-shot`, `repeat`, `random` or `autoplay` |

### Queue

Entries are addressed by the `id` from the state, never by position, so two people editing at
once cannot hit the wrong track. An entry that is already gone answers `404 not_found`; treat that
as "already done" and refresh.

| Request | Body | Effect |
|---------|------|--------|
| `POST /queue` | see below | Add something; answers `201` with the state and `"added": {"id", "title"}` |
| `DELETE /queue` | | Clear the queue and stop |
| `POST /queue/shuffle` | | Shuffle the upcoming entries |
| `DELETE /queue/{id}` | | Remove an entry. Removing the playing one starts the next |
| `POST /queue/{id}/play` | | Jump to an entry and play it |
| `POST /queue/{id}/move` | `{"after": 7}` | Put the entry directly after entry 7 (`0` = the front) |

`POST /queue` takes a `type` plus its field, and an optional `position` (`end` by default, `next`
to play it after the current track, `now` to play it at once):

| `type` | Field | Meaning |
|--------|-------|---------|
| `library` | `path` | A file from the library, by its path |
| `radio` | `preset` | A name from `[radio]` |
| `radio` | `url` | A direct stream URL (checked to be reachable first) |
| `radio` | `uuid` | A radio-browser.info station, from the search below |
| `url` | `url` | One track resolved through yt-dlp. Playlists are refused |

A source that cannot be queued answers `422 invalid_source` with the reason; all yt-dlp slots busy
answers `503 busy`. The checks take up to 25 seconds.

### Radio search and library

| Request | Effect |
|---------|--------|
| `GET /radio/search?q=jazz&limit=10` | Search radio-browser.info (limit 1 to 50); add a result with its `uuid` |
| `POST /library/rescan` | Start an incremental scan. Answers `202` at once, or `409 scan_running`. The state's `scan` field shows `running`, then `last_result` or `last_error`. There is no `force` option here |
