# 3-05 — Music Library & Cache

**Status:** todo  
**Depends on:** 3-01, 3-02, 3-04  
**Unlocks:** 3-06, 3-07

## Objective

Scan the music folder into the music DB, maintain an in-memory item cache, and implement tag management.

> Note: 3-04 dropped `ItemWrapper` (modes work on `[]audio.MediaItem`). The cache below holds
> `audio.MediaItem`s; introduce a wrapper here only if tags/status need per-item state.

## Dir scan

```go
func (c *Cache) BuildDirCache(musicFolder string, cfg *config.BotConfig) error
```

Walk `musicFolder` recursively, skip paths in `ignored_files`/`ignored_folders` config lists, extract metadata via ffprobe, upsert to `MusicDB`. Files no longer on disk are deleted from the DB.

Protected by a `sync.Mutex` (`DirLock`) so the web API waits for the scan to finish.

## In-memory cache

```go
type Cache struct {
    mu    sync.RWMutex
    items map[string]audio.MediaItem
    db    *database.MusicDB
}

func (c *Cache) GetOrCreate(item audio.MediaItem) audio.MediaItem
func (c *Cache) GetByID(id string) (audio.MediaItem, bool)
func (c *Cache) FreeAndDelete(id string)   // remove from cache
```

## Tag management

- Tags stored in DB as `"tag1,tag2,"` (trailing comma)
- "Recent added" auto-tag: applied to items added within last 24h, removed automatically
- `ManageSpecialTags()`: run the SQL update for "recent added" (called on rescan)

## Autoplay mode (moved from 3-04)

3-04 ships `one-shot`, `repeat` and `random` and refuses `!mode autoplay` because nothing
populates the music DB yet. Once the scan exists, this milestone adds the mode:

- Accept `autoplay` in `queue.ParseMode`, `!mode` and `[bot] playback_mode`.
- When the queue runs dry, pull random tracks with `MusicDB.QueryRandom` and append them
  (count from `autoplay_length`, default 5), then continue playing.
- With an empty DB the queue just stops, as in `one-shot`.

## Deliverables

- `internal/media/cache/cache.go`
- `internal/media/cache/scan.go`
- `internal/media/cache/tags.go`
- Tests: scan a temp dir of fixture files, verify DB upsert

## Acceptance criteria

- `!rescan` triggers a full scan and updates the DB
- `!listfile` shows scanned files with correct titles
- `!search beethoven` returns matching tracks from the DB
- Files deleted from disk are removed from DB on next scan
- `!mode autoplay` adds random library tracks when the queue runs dry
