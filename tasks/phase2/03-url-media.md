# 2-03 — URL Media (yt-dlp)

**Status:** todo  
**Depends on:** 2-01  
**Unlocks:** 2-06

## Objective

Implement the `url` media item type using yt-dlp to resolve YouTube/SoundCloud/etc. URLs to playable streams.

## URLItem

```go
type URLItem struct {
    ID       string
    URL      string
    Title    string
    Duration time.Duration
    Thumb    string
}

func (u *URLItem) Validate() error  // yt-dlp --dump-json
func (u *URLItem) Prepare() error   // yt-dlp download to tmp/ if under max_track_duration
func (u *URLItem) StreamURL() string
```

- If duration ≤ `config.Bot.MaxTrackDuration`: pre-download to `tmp/<id>.mp3`, `StreamURL()` returns the file path
- If duration > limit: stream directly via the URL from yt-dlp JSON

## yt-dlp wrapper

```go
func GetInfo(url string, cfg *config.Config) (*YTInfo, error)
func Download(url, outPath string, cfg *config.Config) error
```

Passes `cookie_file`, `source_address`, `user_agent` from `[youtube_dl]` config section.

## Playlist URLs

`!playlist <url>` calls `yt-dlp --flat-playlist --dump-json` and returns N individual `URLItem`s (capped at `config.Bot.MaxTrackPlaylist`).

## Deliverables

- `internal/media/url/item.go`
- `internal/media/url/ytdlp.go`
- `internal/media/url/playlist.go`
- Tests: mock yt-dlp with pre-written JSON fixture (no network in unit tests)

## Acceptance criteria

- `!url https://www.youtube.com/watch?v=...` queues and plays the track
- `!playlist <yt-playlist>` expands and queues tracks
- Invalid URL shows error in chat, item is not queued
- Downloaded tmp files are deleted when the item leaves the queue
