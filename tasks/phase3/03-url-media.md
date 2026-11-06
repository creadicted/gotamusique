# 3-03 — URL Media (yt-dlp)

**Status:** todo  
**Depends on:** 3-01  
**Unlocks:** 3-06

## Objective

Implement the `url` media item type using yt-dlp to resolve YouTube/SoundCloud/etc. URLs to
playable streams. Audio is **always streamed** — nothing is downloaded to disk.

## Design decisions

| # | Decision |
|---|---|
| 1 | Always stream. No tmp downloads, no `Download()`, no cleanup. |
| 2 | `yt-dlp -g -f bestaudio` returns a direct URL that ffmpeg reads via `-i`. No stdout pipe. |
| 3 | Python-compatible config keys (see Config). Missing yt-dlp binary → chat error; bot still starts. |
| 4 | Playlists are lazy: flat entries are queued immediately and resolved just before play. |
| 5 | New optional `audio.Resolver` interface, called by the play loop before `Launch`. |
| 6 | Docker image ships the standalone musllinux yt-dlp binary, pinned by `ARG YTDLP_VERSION`. |
| 7 | Only `http`/`https` URLs. URL passed after `--`, `exec.Command` arg list, no shell. No host filtering. |
| 8 | `!url` uses `--no-playlist` (always one track). `!playlist` expands up to `max_track_playlist`. |
| 9 | URL handlers reply "Resolving…" and work in a goroutine (30 s timeout); semaphore of ~3 concurrent yt-dlp processes. |
| 10 | `URLItem` is in-memory only. Keeps the thumbnail URL string; no DB writes, no thumbnail download. |

## Config

Mirrors the Python bot so existing config files keep working.

```ini
[bot]
max_track_duration = 60     # minutes; hard limit, 0 = no limit
max_track_playlist = 20     # max entries added by !playlist

[youtube_dl]
binary =                    # default: "yt-dlp" from PATH (new key; tests point it at a fake script)
cookiefile =
source_address =
user_agent =
```

## audio.Resolver

```go
// Optional. Implemented by items that must resolve their stream URL just before playback.
type Resolver interface {
    Resolve(ctx context.Context) error
}
```

In `internal/bot/loop.go`, before `pipeline.Launch`: if the item implements `Resolver`, call
`Resolve` with a 30 s timeout. On error: send a chat notice (e.g. "Skipped <title>: longer than
60 min"), call `queue.Next()`, and count it toward the consecutive-failure threshold. Radio, HLS
and file items are unaffected.

## URLItem

```go
type URLItem struct {
    URL      string
    Title    string
    Uploader string
    Duration time.Duration
    Thumb    string // thumbnail URL only; not downloaded

    streamURL string // set by Resolve
}

func (u *URLItem) Validate() error                 // scheme check, yt-dlp --dump-json, duration limit
func (u *URLItem) Resolve(ctx context.Context) error // yt-dlp -g -f bestaudio; re-checks duration limit
func (u *URLItem) StreamURL() string                // resolved direct URL
func (u *URLItem) FormatTitle() string
```

- Single `!url` items are validated at command time (immediate error to the user).
- Playlist entries come from `--flat-playlist` and may lack title/duration; the duration limit
  is enforced in `Resolve` at play time.

## yt-dlp wrapper

```go
func GetInfo(ctx context.Context, url string, cfg *config.Config) (*YTInfo, error)       // --dump-json --no-playlist
func ResolveStream(ctx context.Context, url string, cfg *config.Config) (string, error)  // -g -f bestaudio
func FlatPlaylist(ctx context.Context, url string, cfg *config.Config) ([]YTInfo, error) // --flat-playlist --dump-json
```

- Uses `binary`, `cookiefile`, `source_address`, `user_agent` from `[youtube_dl]`.
- Always passes `--` before the URL; rejects non-`http(s)` schemes before invoking yt-dlp.
- A missing binary returns a distinct error surfaced as a clear chat message.

## Commands

- `!url <url>` — queue exactly one track (`--no-playlist`). A bare playlist link replies
  "that's a playlist, use !playlist".
- `!playlist <url>` — expand up to `max_track_playlist` entries; a single video queues one track.
- Both reply "Resolving…" immediately, run in a goroutine with a 30 s timeout, then post
  "Queued: …" or the error. A semaphore (~3) limits concurrent yt-dlp processes.
- Verify the queue is safe for concurrent `Enqueue` before relying on this.

## Docker

Replace nothing in the existing ffmpeg install; add the standalone binary:

```dockerfile
ARG YTDLP_VERSION=<pinned>
ADD https://github.com/yt-dlp/yt-dlp/releases/download/${YTDLP_VERSION}/yt-dlp_musllinux /usr/local/bin/yt-dlp
RUN chmod +x /usr/local/bin/yt-dlp
```

No Python, no runtime self-update. Updating = bump the ARG and rebuild.

## Deliverables

- `internal/media/url/item.go`
- `internal/media/url/ytdlp.go`
- `internal/media/url/playlist.go`
- `audio.Resolver` + loop hook in `internal/bot/loop.go`
- `!url` / `!playlist` handlers in `internal/command/`
- Config keys + defaults in `configuration.default.ini` and `internal/config/`
- Dockerfile update
- Tests using a fake script (set via `binary`) that prints a JSON fixture — no network

## Acceptance criteria

- `!url https://www.youtube.com/watch?v=...` queues and plays the track
- `!url` on `watch?v=X&list=Y` queues only X
- `!playlist <yt-playlist>` queues up to `max_track_playlist` tracks, resolved lazily
- A non-`http(s)` URL or a value starting with `-` is rejected; nothing is queued
- A track over `max_track_duration` is rejected (`!url`) or skipped with a chat notice (playlist)
- Missing yt-dlp binary gives a chat error; radio/file playback still work
- A slow yt-dlp call does not block the Mumble event goroutine
- Resolve failure in the loop skips the track and counts toward the failure threshold
