# 3-05 — Music Library (scan, DB wiring, autoplay)

**Status:** done  
**Depends on:** 3-01, 3-02, 3-04  
**Unlocks:** 3-06, 3-07

## Objective

Wire the music DB into the bot, scan `music_folder` into it with `!rescan`, and add the
`autoplay` playback mode that 3-04 deferred.

The in-memory item cache, tags, and the `!listfile` / `!search` commands moved to 3-06 (see
that spec). 3-05 is about the database and the filesystem.

## Design decisions

| # | Decision |
|---|---|
| 1 | Scope: directory scan, DB wiring, `!rescan`, `autoplay`. Cache, tags, `!listfile`, `!search` → 3-06. |
| 2 | The music DB opens **only when `[files] music_folder` is set**. A failed open logs an error and the bot runs without the library. |
| 3 | `!rescan` is manual and **incremental**; an optional startup scan is opt-in. |
| 4 | `autoplay` refills at the end of the queue; a failure streak stops it. |
| 5 | A refill **replaces** the queue (autoplay is the only mode that trims). |
| 6 | Audio-extension allowlist; exact-name ignore lists; no directory symlinks. |
| 7 | `!rescan` is open to everyone. |
| 8 | The scan extracts **no thumbnails**. |
| 9 | `database.MigrateMusic(path)`; compose/README get a data volume. |
| 10 | The scan never wipes the library because the folder is unavailable. |
| 11 | `!mode autoplay` starts playing immediately on an idle bot. |
| 12 | `keywords` = `"<title> <artist>"`, with a filename/folder fallback. |

## Config

```ini
[bot]
music_database_path = music.db     # music library database (created on first use)
refresh_cache_on_startup = False   # run a scan when the bot starts
autoplay_length = 5                # tracks added per autoplay refill

[files]
music_folder =
ignored_folders =                  # comma-separated exact folder names, any depth
ignored_files =                    # comma-separated exact file names
```

Python key names are kept for compatibility. `ignored_*` match exactly one path component,
case-sensitively; there are no globs or regexes in this milestone.

## DB wiring

- At startup, if `[files] music_folder` is set, call `database.MigrateMusic(music_database_path)`
  then `database.OpenMusic`. If that fails, log an error and continue **without a library**:
  `!rescan` and `autoplay` reply "library unavailable"; radio, HLS, URL and `!file` keep working.
- New `database.MigrateMusic(path)`. `Migrate(settingsPath, musicPath)` stays and calls it, so the
  settings DB is created only when something needs it (3-10).
- Close the DB on shutdown.

## Directory scan (`internal/library`)

```go
type Scanner struct { /* db, music folder, ignore lists, mutex */ }
func (s *Scanner) Scan(ctx context.Context) (Result, error) // Result{Added, Updated, Removed, Skipped}
```

**What is scanned**
- Only files with a known audio extension are probed: `mp3, flac, ogg, oga, opus, m4a, aac, wav,
  wma, ape, aiff`. Anything else is skipped without a probe.
- A file that fails ffprobe is logged and skipped; it does not abort the scan.
- Directory symlinks are not followed. A file symlink is kept only if it resolves inside
  `music_folder` (same rule as `!file`).
- `ignored_folders` / `ignored_files` skip matching components.
- IDs use the existing sha1-of-relative-path scheme, so a scanned track and a `!file` track
  share an ID.

**Incremental**
- Each row's `metadata` JSON stores the file's size and modified time. A rescan probes only new
  files and files whose size or mtime changed, and removes rows for files that are gone.
- Unverified: the Python bot's tolerance of these extra `metadata` keys (it matters only if one
  `.db` is shared between the two bots).

**Row contents**
- `type = "file"`, `path` = relative path, `title`, `metadata` (`artist`, `duration`, size, mtime).
- `keywords = "<title> <artist>"`, matching what the Python migration produced. With no title tag
  the title falls back to the filename without its extension and the folder path words are
  added, so untagged files stay searchable.
- **No thumbnails.** `file.ExtractMetadata` is split so the scan runs ffprobe only; `!file`
  still extracts artwork as today. Thumbnails are extracted on demand later (3-07/3-08).

**Safety**
- Abort without touching the DB if `music_folder` is not a readable directory.
- Delete only rows of `type = "file"`; never the `info` row or other media types.
- If the walk finds **zero** audio files but the DB has file rows, remove nothing and reply "no
  audio files found, is the music folder mounted? nothing removed".
- A folder that is only partly unavailable still prunes that subfolder's rows (not detected).
- One scan at a time (mutex). A second request gets "scan already running".

## `!rescan`

- Open to everyone (restricting commands is 3-11).
- Replies "Scanning…", runs the scan in a goroutine (like `!url`, so it never blocks the Mumble
  event goroutine), then "Done: N added, M updated, K removed".
- `[bot] refresh_cache_on_startup = True` runs one scan after startup, in the background.
- Alias key: `[commands] rescan = rescan`.

## Autoplay mode (moved from 3-04)

- Accept `autoplay` in `queue.ParseMode`, `!mode` and `[bot] playback_mode`. With no library,
  `[bot] playback_mode = autoplay` logs a warning and falls back to `one-shot`.
- **Refill:** when the queue runs past its end in autoplay mode, query `autoplay_length` random
  tracks with `MusicDB.QueryRandom` (rows of type `file` only) and **replace** the queue with
  them. Played history is dropped so the queue, and `!queue`, stay short. `one-shot`, `repeat`
  and `random` are unchanged.
- The swap is atomic and happens only if the queue is unchanged since the refill began (same
  `Queue.Version` pattern as the failure guard). If something was queued during the DB query,
  the refill is skipped and the loop re-checks.
- Records whose file no longer exists are skipped at refill time.
- **Failure cap:** the existing `failGuard` streak also covers autoplay. When `autoplay_length`
  consecutive tracks fail with no successful one in between (e.g. the folder was unmounted),
  the bot goes idle, says "autoplay stopped: tracks keep failing", and stays in autoplay mode.
  Without this, each refill adds fresh untried entries and an unmounted folder would loop forever.
- **`!mode autoplay` on an idle bot** (empty queue or past its end) refills and starts playing
  immediately. A track already playing is not interrupted; the refill happens when it ends.
  Replies: empty library → "library is empty, run !rescan"; no library → "library unavailable".
  Running `!mode autoplay` again after the failure cap restarts it. Document this asymmetry with
  `repeat`/`random` in the `!mode` help text.

## Docker

- `docker-compose.yml` mounts `./data:/app/data`; README documents
  `music_database_path = data/music.db`.
- README warns that `docker run` without a volume loses the library when the container is
  recreated (a rescan restores it, but re-probes every file).

## Known limitations

- The first scan probes every file with ffprobe; its duration on a large library is unmeasured.
- Autoplay can leave a short gap between the last track and the first new one (a DB query).
- A refill drops tracks you queued by hand once they have played.
- No cache, tags, `!listfile` or `!search` yet (3-06).

## Deliverables

- `internal/library/scan.go`, `internal/library/scan_test.go`
- `database.MigrateMusic` in `internal/database/migration.go`
- Split of thumbnail extraction out of `file.ExtractMetadata`
- DB open/close wiring in `internal/bot` (and `cmd/`) with the "library unavailable" fallback
- `!rescan` handler; `autoplay` in `queue.ParseMode`, `!mode`, config and the bot loop
- Config keys, defaults and README/CHANGELOG; `docker-compose.yml` volume
- Tests: scan a temp dir of fixture files (needs ffprobe, skipped when absent), incremental
  rescan, removal of deleted files, the unavailable-folder guard, ignore lists and symlinks,
  the autoplay refill, the failure cap, the stale-refill guard, and the `!mode autoplay` replies

## Acceptance criteria

- `!rescan` scans the folder and reports added/updated/removed counts
- A second `!rescan` with no changes probes no files
- Files deleted from disk are removed from the DB on the next scan; non-`file` rows are untouched
- An unmounted/empty music folder removes nothing and says so
- `!mode autoplay` on an idle bot starts playing random library tracks
- Autoplay keeps refilling at the end of the queue and keeps `!queue` short
- If every track fails (e.g. folder unmounted) autoplay stops and reports it
- With no `music_folder` the bot starts normally; `!rescan` and autoplay say "library unavailable"
- A queued-while-refilling track is never dropped by the refill
