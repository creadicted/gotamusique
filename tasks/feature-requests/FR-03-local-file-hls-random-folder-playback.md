# FR-03 — Local file, HLS/M3U8, and random-folder playback modes

**Status:** open — pending answers to clarifying questions below  
**Affects:** `internal/audio/`, `internal/command/`, `internal/config/`, `internal/bot/`  
**Related milestone:** Phase 2

## Motivation

The bot currently supports only live HTTP radio streams and radio-browser.info stations.
Users want three additional playback modes:

1. **Local file playback** — play a single audio file by path or selection
2. **M3U8/HLS stream playback** — play HLS playlists (ffmpeg already handles these natively)
3. **Random-folder playback** — shuffle-play all audio files from a local directory

## Codebase context (discovered during analysis)

- `internal/audio/media.go` defines the `MediaItem` interface (`StreamURL() string`, `FormatTitle() string`). Any new item type only needs those two methods — the pipeline, queue, and bot loop are fully agnostic.
- `internal/audio/pipeline.go` launches ffmpeg with `-i <url>`. ffmpeg accepts local file paths, `http://` URLs, and `.m3u8` HLS URLs identically — **the pipeline needs zero changes**.
- `internal/radio/item.go` (`RadioItem`) is the direct template for new item types.
- New commands go in `internal/command/handlers.go` + `registry.go`; new config sections follow the `[radio]` pattern in `config.go` and `configuration.default.ini`.
- The bot loop (`internal/bot/loop.go`) already has a `consecutiveFails` counter that stops playback after N consecutive launch failures.

## Proposed new types

| Type | Package | `StreamURL()` returns | `FormatTitle()` prefix |
|---|---|---|---|
| `FileItem` | `internal/file` (new) or inline | absolute filesystem path | `[File]` |
| `HLSItem` | `internal/file` or inline | the `.m3u8` URL | `[HLS]` |
| Random-folder | constructs `FileItem` entries | — (queue-construction only) | — |

## Edge cases to handle

- **Subfolder recursion**: configurable — default TBD (see Q4 below)
- **Mixed file types / unsupported formats**: skip silently or warn — configurable (see Q5)
- **Stream interruptions for HLS**: retry behavior or fail-fast (see Q6)

---

## Open questions — need answers before architecture design

### Q1 — Command interface for local file playback

**Option A** — New `!play <path>` command (separate from `!radio`)  
**Option B** — Extend `!radio` to accept local paths too

And for path resolution:  
- **a)** Arbitrary absolute paths (any file the bot process can read — simpler, less safe)  
- **b)** Paths relative to a configured `music_dir` in the INI (recommended for safety)  
- **c)** Both: absolute if arg starts with `/`, otherwise relative to `music_dir`

### Q2 — Command interface for M3U8/HLS

M3U8 URLs are `http://`/`https://` and already pass through `!radio` today (ffmpeg handles them). Options:

- **Option A** — No separate command; `!radio <m3u8-url>` just works (title says `[Radio]` not `[HLS]`)  
- **Option B** — Dedicated `!hls <url>` command with `HLSItem` type and `[HLS]` title  
- **Option C** — Auto-detect `.m3u8` extension inside `handleRadio` and create `HLSItem` instead of `RadioItem`

### Q3 — Command interface for random-folder playback

- What is the command name? (`!folder`, `!random`, `!shuffle`, …?)
- Behaviour on invocation:
  - **A)** Enqueue the full shuffled directory into the queue at once (all tracks visible in `!queue`)
  - **B)** Pick one random track per track-end (continuous shuffle mode; queue stays length 1)
- Can the argument be a configured preset name (like radio presets), an inline path, or both?

### Q4 — Subfolder recursion: what default?

Configurable is agreed. Should the default be **recurse** or **flat only**?

### Q5 — Unsupported file type handling

ffmpeg can decode almost anything, so extension filtering is optional. Options:
- Filter by an extension allowlist (e.g. `.mp3 .flac .ogg .wav .opus .m4a .aac`) and skip/warn on others
- Attempt all files and let ffmpeg fail (simplest; falls through existing fail-counter)

If filtering: warn in channel or skip silently by default?

### Q6 — HLS stream interruption / retry

ffmpeg supports reconnect flags (`-reconnect 1 -reconnect_streamed 1 -reconnect_delay_max 5`).
The bot loop already stops after N consecutive failures via `consecutiveFails`.

- **Option A** — Add ffmpeg reconnect flags to `HLSItem` launches only (transparent internal retry)
- **Option B** — Existing fail-counter behaviour is sufficient; no extra retry logic
- **Option C** — Configurable per-item retry count on top of the fail counter

### Q7 — Path security

If users can specify file paths via Mumble chat, should the bot restrict paths to `music_dir` to prevent reading arbitrary files? Or is this a trusted-admin channel where that is not a concern?

---

## Suggested defaults (pending confirmation)

| Decision | Suggested default | Reason |
|---|---|---|
| Q1 command | New `!play` command, paths relative to `music_dir` | Clean separation; `music_dir` prevents path traversal |
| Q2 HLS | Option C (auto-detect `.m3u8` in `handleRadio`) | Zero new commands; existing radio flow reused |
| Q3 random command | `!folder <name-or-path>`, full shuffle into queue | Users can see and skip tracks; consistent with queue UX |
| Q4 recursion default | Flat only | Safer default; opt-in recursion via config |
| Q5 file types | Extension allowlist, skip silently | Avoids noisy failures for non-audio files (cover art, etc.) |
| Q6 retry | Option A (ffmpeg reconnect flags for HLS) | Transparent; no bot-level complexity |
| Q7 security | Restrict to `music_dir` | Least-surprise for a network-facing bot |
