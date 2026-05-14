# 2-07 — Web REST API

**Status:** todo  
**Depends on:** 2-04, 2-05  
**Unlocks:** 2-08

## Objective

HTTP server with all REST endpoints consumed by the existing frontend.

## Endpoints

| Method | Path | Description |
|---|---|---|
| GET | `/` | Serve index HTML |
| GET | `/playlist` | Current playlist slice + metadata |
| POST | `/post` | All playback control actions |
| GET | `/library/info` | Tags, dirs, upload config |
| POST | `/library` | Query / add / delete / edit library |
| POST | `/upload` | Upload audio file |
| GET | `/download` | Download file or zip |

## Auth methods

| `auth_method` | Behaviour |
|---|---|
| `none` | No auth |
| `password` | HTTP Basic Auth (config user/pass or DB users) |
| `token` | One-time token from `!web` command; session cookie after first use |

IP ban after `max_attempts` failed logins.

## POST /post actions

`pause`, `resume`, `stop`, `next`, `clear`, `random`, `one-shot`, `repeat`, `autoplay`, `rescan`, `volume_up`, `volume_down`, `volume_set_value`, `play_music`, `delete_music`, `move_playhead`, `add_url`, `add_radio`, `add_item_at_once`, `add_item_bottom`, `add_item_next`, `delete_item_from_library`, `add_tag`

All POST /post responses return the current status JSON.

## Deliverables

- `internal/web/server.go`
- `internal/web/auth.go`
- `internal/web/handlers/` — one file per endpoint group
- Tests: each handler with a mock `Bot` interface using `httptest`

## Acceptance criteria

- `GET /playlist` returns correct JSON while a track plays
- `POST /post {"action":"pause"}` pauses playback
- Basic auth rejects wrong credentials; IP banned after `max_attempts`
- Token auth session persists across requests
