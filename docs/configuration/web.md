# Web interface

The bot can serve an HTTP API (and, in a later release, a web UI) so people in the Mumble
channel can follow and control playback from a browser. It is **off by default**: nothing
listens until you enable it.

!!! note "Work in progress"
    This release ships the login and a read-only `GET /api/v1/state`. Playback control, the
    library and uploads follow in the next releases, and the browser UI after that.

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
| `access_address` | `http://127.0.0.1:8181` | The public URL people open. The link from `!web` is built from it. An `https://` URL marks the session cookie *Secure* |
| `auth_method`    | `token`                 | `token` or `none` (see below) |

The section name and these keys are the same as in botamusique, so an existing file loads. Its
other keys (`user`, `password`, `max_attempts`, `flask_secret`, `is_web_proxified`,
`max_upload_file_size`, `upload_enabled`, `web_logfile`) are accepted and ignored.
`auth_method = password` is **not supported** and stops the bot at startup, rather than silently
serving without the login you asked for.

## Logging in (`auth_method = token`)

1. Type `!web` in the Mumble channel.
2. The bot sends you a **private message** with a login link. The link works once and expires after
   5 minutes. Asking again invalidates the previous link.
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

`GET /api/v1/state` returns the whole state in one response: queue (each entry has a stable `id`),
position, volume, mode and scan status. Its `version` changes whenever the state changes (the
playhead position excluded), and it is also sent as an `ETag`, so polling with
`If-None-Match: "<version>"` (or `?since=<version>`) costs a `304` while nothing happened.
Errors are JSON: `{"error": "...", "code": "unauthenticated"}`.
