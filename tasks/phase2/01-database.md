# 2-01 — Database Layer

**Status:** todo  
**Depends on:** Phase 1 complete  
**Unlocks:** 2-02, 2-03, 2-05, 2-10, 2-11

## Objective

Add SQLite persistence for settings and the music library. Schema is identical to the Python version so existing `.db` files remain compatible.

## Settings DB (`settings-<username>.db`)

Table: `botamusique (section TEXT, option TEXT, value TEXT, UNIQUE(section, option))`

Wraps runtime state that should survive restarts: volume, ducking settings, playback mode, user bans, URL bans, web tokens.

```go
type SettingsDB interface {
    Get(section, option string) (string, error)
    GetOrDefault(section, option, fallback string) string
    Set(section, option string, value any) error
    Has(section, option string) bool
    Remove(section, option string) error
    Items(section string) ([][2]string, error)
}
```

## Music DB (`music.db`)

Table: `music (id TEXT PRIMARY KEY, type TEXT, title TEXT, keywords TEXT, metadata TEXT, tags TEXT, path TEXT, create_at DATETIME DEFAULT CURRENT_TIMESTAMP)`

```go
type MusicDB interface {
    Insert(m MusicRecord) error
    Query(cond Condition) ([]MusicRecord, error)
    QueryByID(id string) (*MusicRecord, error)
    QueryByKeywords(keywords []string) ([]MusicRecord, error)
    QueryByTags(tags []string) ([]MusicRecord, error)
    QueryRandom(count int, cond Condition) ([]MusicRecord, error)
    Delete(cond Condition) error
    AllPaths() ([]string, error)
    AllTags() ([]string, error)
}
```

## Migration
Dont replicate migration logic. We start with a clean slate.

## Deliverables

- `internal/database/settings.go`
- `internal/database/music.go`
- `internal/database/condition.go` — query builder mirroring Python `Condition` class
- `internal/database/migration.go`
- Tests: round-trip insert/query/delete, migration from each prior schema version

## Acceptance criteria

- Existing Python-generated `.db` files open and query correctly
- Migration runs without data loss from each prior schema version
- `SettingsDB.Set` → `SettingsDB.Get` round-trip works
