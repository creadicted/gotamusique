# 3-07 — Control API for Kallisti (web UI backend)

**Status:** todo  
**Priority:** next (top of the backlog)  
**Depends on:** 3-04, 3-05  
**Unlocks:** 3-08 (Kallisti panel)

## Objective

Give gotamusique a small, documented HTTP API so that a web UI can show live bot state and trigger
commands without users having to learn the `!commands`.

**Decision:** we do not build or embed our own web frontend. The UI lives in
[Kallisti](https://kallisti.chat) (the Mumble web client we already use) as a panel in its client.
This supersedes the old plan of serving the botamusique webpack frontend and its `/post`-style API.
Kallisti is only one client of the API, so the bot must stay UI-agnostic: no Kallisti-specific code
here, and chat commands keep working unchanged.

## Design principles

1. **One command path.** The API runs the same handlers as chat, through the same dispatcher, so
   aliases, argument parsing, admin checks and behaviour never drift between chat and web.
2. **Self-describing.** The bot publishes its command list with argument schemas. Kallisti renders
   buttons and forms from that, so a new command shows up in the UI with no Kallisti change.
3. **Structured results.** Commands return data, not only chat text.
4. **Trusted proxy, not public.** The API is meant to be called by the Kallisti server, not by browsers
   directly (no CORS, no browser-held secrets).

## Prerequisite refactor: handlers must return results

Today handlers have the signature
`func(bot BotAPI, user string, msg *gumble.TextMessage, cmd, arg string)` and reply by calling
`msg.Channels[0].Send(...)` (`dispatcher.go`) or `reply(cfg, msg, html, plain)` (`urlcmd.go`).
They cannot run without a Mumble message, and their output cannot be captured.

Introduce a reply sink and make handlers write to it instead of the message:

```go
// Replier receives a command's output. Chat sends it to the channel; the API collects it.
type Replier interface {
    Reply(html, plain string)
}

type HandlerFunc func(bot BotAPI, user string, r Replier, cmd, arg string)
```

- A chat `Replier` wraps the existing `msg` send logic (behaviour unchanged).
- An API `Replier` collects the plain text (and optionally HTML) into the response.
- Do this as a mechanical, behaviour-preserving first PR; existing handler tests keep passing with
  a fake `Replier`.
- `handlers_test.go` currently builds `gumble.TextMessage`s; move those to the fake.

## Command schema

Extend `Dispatcher.Register` with optional argument metadata, defaulting to free text so existing
registrations still compile:

```go
type Arg struct {
    Name     string   // "url", "volume"
    Kind     string   // "text" | "number" | "choice" | "flag"
    Required bool
    Choices  []string // for Kind == "choice", e.g. the playback modes
    Help     string
}
```

Fill it in for every command in `registry.go`. Aliases come from config (`aliasesFor`), and the API
reports the canonical name plus the configured aliases.

## Endpoints (all JSON, all under `/api/v1`)

| Method | Path | Description |
|---|---|---|
| GET | `/state` | Now playing, queue (with current index), volume, muted, mode, library available |
| GET | `/commands` | Command list: canonical name, aliases, description, admin-only, args schema |
| POST | `/commands/{name}` | Run a command: `{"args": "raw string"}` or `{"args": {"url": "..."}}`. Returns `{ok, output, state}` |
| GET | `/events` | Server-Sent Events: a `state` event whenever the queue, current item, volume, mute or mode changes |
| GET | `/presets` | Configured radio presets (name, comment) for one-click buttons |
| GET | `/library?q=&page=` | Paged library search (reuses `SearchLibrary` / `ListLibrary`) |

Notes:
- `/state` can use `queue.Version()` (already bumped by every mutator) as the change token for `/events`.
  A changed value means "re-read", not proof that something moved.
- Volume, mute, mode and current item have no change notification today. Add a small
  `Subscribe()`/notify hook in the bot rather than polling.
- Keep `/state` free of anything large; thumbnails stay out of scope (see below).
- Pause/resume (FR-02) is not implemented yet; add `paused` to `/state` when it lands.

## Who is the caller?

Commands take a `user` argument (used for admin checks and `!joinme`). The API caller supplies it:

- The Kallisti server forwards the authenticated Kallisti/Mumble username in a header
  (e.g. `X-Gotamusique-User`), and the bot applies the **same admin rules** as chat.
- `!joinme` needs a Mumble user to resolve the channel; resolve by that username, and return a clear
  error if the user is not connected.
- Do **not** allow `!kill` through the API unless the user is an admin. Consider hiding admin-only
  commands from `/commands` for non-admin callers.

## Auth and binding

Config section (new):

```ini
[api]
enabled = false
bind = 127.0.0.1:8181
token = change-me        ; shared secret, sent as "Authorization: Bearer <token>"
```

- Disabled by default.
- Reject all requests without the bearer token (constant-time compare). Refuse to start if
  `enabled = true` with an empty token.
- The token is held by the Kallisti server only; browsers never see it.
- TLS is the operator's concern; the intended deployment is loopback or a private Docker network.

## Out of scope (deferred)

- Uploads, downloads, library editing and tag editing over HTTP.
- Thumbnails (deferred from 3-05). If wanted later, extract on demand with `file.ExtractThumb` and cache
  as files next to the DB, not inside it.
- Per-user sessions, login pages, IP banning: Kallisti authenticates users, the bot only trusts Kallisti.
- Browser-facing static file serving (see 3-08).

## Deliverables

- `internal/command`: `Replier` refactor (separate PR, behaviour-preserving), `Arg` schema on `Register`
- `internal/api/server.go`, `internal/api/state.go`, `internal/api/events.go`, `internal/api/commands.go`
- `internal/config`: `[api]` section
- Bot change-notification hook feeding `/events`
- `README.md`: API section and `[api]` config; `CHANGELOG.md` entry
- Tests: each endpoint with a fake `BotAPI` via `httptest`; SSE test; auth tests

## Acceptance criteria

- With `[api] enabled = true`, `GET /api/v1/state` with the token returns correct JSON while a track plays;
  without the token it returns 401.
- `POST /api/v1/commands/volume {"args":"40"}` sets the volume, returns the confirmation text, and the same
  change is visible in chat behaviour and `/state`.
- `GET /api/v1/commands` lists every registered command with its aliases and args, and the list matches what `!help` shows.
- `/events` delivers a state event within a second of a chat-issued `!skip`.
- Admin-only commands are rejected for non-admin callers.
- All existing chat command tests still pass after the `Replier` refactor.
