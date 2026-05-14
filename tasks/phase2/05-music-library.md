# 2-05 — Music Library & Cache

**Status:** todo  
**Depends on:** 2-01, 2-02  
**Unlocks:** 2-06, 2-07

## Objective

Scan the music folder into the music DB, maintain an in-memory item cache (no duplicate wrappers for the same track), and implement tag management.

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
    items map[string]*playlist.ItemWrapper
    db    *database.MusicDB
}

func (c *Cache) GetOrCreate(item media.MediaItem) *playlist.ItemWrapper
func (c *Cache) GetByID(id string) (*playlist.ItemWrapper, bool)
func (c *Cache) FreeAndDelete(id string)   // remove from cache, delete tmp file
```

## Tag management

- Tags stored in DB as `"tag1,tag2,"` (trailing comma)
- "Recent added" auto-tag: applied to items added within last 24h, removed automatically
- `ManageSpecialTags()`: run the SQL update for "recent added" (called on rescan)

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
