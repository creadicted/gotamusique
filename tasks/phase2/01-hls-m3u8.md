# 2-01 — HLS / M3U8 stream playback

**Status:** todo  
**Depends on:** Phase 1 complete  
**Unlocks:** nothing (standalone enhancement)

## Objective

Add a dedicated `!hls <url>` command that plays HLS/M3U8 streams with a distinct
`[HLS]` title prefix and ffmpeg reconnect flags for resilience against transient
stream interruptions.

## HLSItem

New package `internal/hls/`, file `item.go`:

```go
// HLSItem represents an HLS/M3U8 playlist stream.
type HLSItem struct {
    URL  string
    Name string
}

func (h *HLSItem) StreamURL() string   { return h.URL }
func (h *HLSItem) FormatTitle() string { return "[HLS] " + h.Name }
```

### Name resolution

`NewHLSItemFromURL(rawURL string) *HLSItem` derives `Name` as follows:

1. Parse the URL; on failure use the raw string as both `URL` and `Name`.
2. `name = strings.TrimSuffix(path.Base(u.Path), ".m3u8")` (case-insensitive strip).
3. If `name` is empty, `"/"`, or `"."`, fall back to `u.Host`.
4. If `u.Host` is also empty, fall back to the raw URL string.

Examples:
- `https://cdn.example.com/live/jazz-128k.m3u8` → `jazz-128k`
- `https://cdn.example.com/live/` → `cdn.example.com`
- `https://cdn.example.com` → `cdn.example.com`

## Command: `!hls`

New handler `handleHLS` in `internal/command/handlers.go`:

- **No argument:** reply `"Usage: !hls <url>"`.
- **Non-HTTP/HTTPS scheme:** reply `"Usage: !hls <url>"` (only `http://` and `https://` accepted).
- **Valid HTTP/HTTPS URL:** construct `*hls.HLSItem` via `NewHLSItemFromURL`, call
  `bot.Enqueue(item)`, reply `"Queued: <name>"`.

No validation of the URL beyond the scheme check — invalid URLs surface as ffmpeg
errors in the channel via the bot loop's existing error handling.

`!hls` is URL-only. No preset support.

`!radio` is unchanged — it always creates a `RadioItem` regardless of URL content.
There is no auto-detection of `.m3u8` in `handleRadio`.

Register in `internal/command/registry.go`:

```go
d.Register(a("play_hls"), handleHLS, false, "Play an HLS/M3U8 stream by URL")
```

Add default alias to `internal/config/configuration.default.ini`:

```ini
play_hls = hls
```

## ffmpeg reconnect flags

Extract a pure helper in `internal/audio/pipeline.go`:

```go
func buildFFmpegArgs(url, verbosity string) []string
```

For `http://` or `https://` URLs, prepend the following flags before `-i`:

```
-reconnect 1
-reconnect_streamed 1
-reconnect_delay_max 5
-reconnect_at_eof 1
-reconnect_on_http_error 4xx,5xx
```

For all other URLs (local file paths, etc.) these flags are omitted.

`Launch()` calls `buildFFmpegArgs` instead of inlining the args slice.

## Deliverables

- `internal/hls/item.go` — `HLSItem` struct and `NewHLSItemFromURL`
- `internal/hls/item_test.go` — constructor and method tests (see below)
- Edit `internal/command/handlers.go` — add `handleHLS` and `playHLS` helper
- Edit `internal/command/handlers_test.go` — handler tests
- Edit `internal/command/registry.go` — register `play_hls`
- Edit `internal/config/configuration.default.ini` — add `play_hls = hls`
- Edit `internal/audio/pipeline.go` — extract `buildFFmpegArgs`, inject reconnect flags
- `internal/audio/pipeline_args_test.go` — `buildFFmpegArgs` tests (see below)

## Tests

### `internal/hls/item_test.go`

| Test | Assertion |
|---|---|
| `TestNewHLSItemFromURL_filename` | `jazz-128k.m3u8` path → name `jazz-128k` |
| `TestNewHLSItemFromURL_uppercaseExtension` | `.M3U8` → name stripped correctly |
| `TestNewHLSItemFromURL_trailingSlash` | path `/live/` → hostname fallback |
| `TestNewHLSItemFromURL_noPath` | `https://cdn.example.com` → hostname |
| `TestNewHLSItemFromURL_malformed` | non-URL string → raw string as name |
| `TestHLSItem_FormatTitle` | `"[HLS] jazz-128k"` |
| `TestHLSItem_StreamURL` | returns URL unchanged |

### `internal/audio/pipeline_args_test.go`

| Test | Assertion |
|---|---|
| `TestBuildFFmpegArgs_httpURL` | reconnect flags present for `http://` URL |
| `TestBuildFFmpegArgs_httpsURL` | reconnect flags present for `https://` URL |
| `TestBuildFFmpegArgs_localPath` | reconnect flags absent for `/music/track.flac` |
| `TestBuildFFmpegArgs_verbosityPropagated` | `-v debug` appears when verbosity is `"debug"` |
| `TestBuildFFmpegArgs_inputFlag` | `-i` followed by the URL appears in all cases |

### `internal/command/handlers_test.go` additions

| Test | Assertion |
|---|---|
| `TestHandleHLS_noArg` | no enqueue, reply contains `"Usage"` |
| `TestHandleHLS_nonHTTP` | non-HTTP scheme rejected, no enqueue |
| `TestHandleHLS_validURL` | `bot.Enqueue` called with `*hls.HLSItem` |

## Acceptance criteria

- `!hls https://cdn.example.com/live/jazz-128k.m3u8` announces `[HLS] jazz-128k`
- `!radio https://cdn.example.com/stream.mp3` still announces `[Radio] cdn.example.com`
- `!radio https://cdn.example.com/live/stream.m3u8` announces `[Radio] cdn.example.com`
  (no special-casing in `handleRadio`)
- ffmpeg is invoked with all five reconnect flags for HTTP/HTTPS URLs
- ffmpeg is invoked without reconnect flags for local file paths
- `!hls` with no argument or a non-HTTP URL replies with a usage message
- `go test ./...` passes
