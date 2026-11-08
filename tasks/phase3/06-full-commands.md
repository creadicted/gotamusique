# 3-06 — Full Command Set

**Status:** in progress — `!listfile`, `!search`, tags and `!shortlist` shipped; cache, `!filematch`, `!delete`, URL search, mode extras, `!version` and i18n remain; the auto "recent added" tag is deferred  
**Depends on:** 3-02, 3-03, 3-04, 3-05  
**Unlocks:** nothing (feature-complete bot)

## Objective

Add all commands that require Phase 2 subsystems (file library, URL media, advanced playlist modes, tags, DB).

## Moved in from 3-05

3-05 ships only the directory scan, DB wiring, `!rescan` and `autoplay`. This milestone now also
owns what 3-05 dropped:

- **In-memory item cache** (`internal/media/cache`): `GetOrCreate`, `GetByID`, `FreeAndDelete`,
  holding `audio.MediaItem`s so the same track is not wrapped twice. Introduce a per-item wrapper
  only if tags need per-item state (3-04 dropped `ItemWrapper`).
- **Tag management:** tags are stored in the DB as `"tag1,tag2,"` (trailing comma). A "recent
  added" auto-tag is applied to items added within the last 24 h and removed automatically;
  `ManageSpecialTags()` runs the SQL update (called after a rescan).
- **`!listfile [regex]` and `!search <keywords>`** against the scanned library (3-05 populates it).
  Acceptance: `!listfile` shows scanned files with correct titles; `!search beethoven` returns
  matching tracks. For untagged files the stored title is the filename fallback (see 3-05).
- Deleting a file from disk is handled by the 3-05 scan, not here.

## Additional commands (beyond Phase 1)

### File commands
| Command | Description |
|---|---|
| `!file <path>` | Add local file or folder — single files **shipped in 3-02**; folders remain |
| `!filematch <regex>` | Regex match on filenames, add all matches |
| `!listfile [regex]` | List files (paginated, 50/page) |
| `!search <keywords>` | Keyword search in music library |
| `!delete <n>` | Remove file from library (needs `delete_allowed` config) |

### URL commands
| Command | Description |
|---|---|
| `!url <url>` | Add YouTube/SC/etc. URL — **shipped in 3-03** |
| `!playlist <url>` | Expand and queue a playlist — **shipped in 3-03** |
| `!yplay <query>` | Search YouTube, play first result |
| `!ysearch <query>` | Search YouTube, display results table |

### Tag commands
| Command | Description |
|---|---|
| `!addtag [n] <tags>` | Add tags to item at index n (or current) |
| `!untag [n] <tags\|*>` | Remove tags from item |
| `!tag <tags>` | Add all library items with given tags to queue |
| `!findtagged <tags>` | List library items with given tags |
| `!shortlist <n\|*>` | Pick from last search result by index |

### Mode commands
| Command | Description |
|---|---|
| `!mode [one-shot\|repeat\|random\|autoplay]` | Get or set playback mode — **shipped in 3-04** (`autoplay` in 3-05) |
| `!rescan` | Scan `music_folder` into the DB — **shipped in 3-05** |
| `!last` | Jump to last item in queue |
| `!repeat [n]` | Duplicate current item n times |
| `!random` | Shuffle queue |

### Misc
| Command | Description |
|---|---|
| `!version` | Report bot version |
| `!update` | Admin: run update (or just print "not supported") |

## i18n

Load `lang/<language>.json` files and replace hardcoded English strings with `Tr(key, vars)` calls throughout all command handlers (Phase 1 and Phase 2).

```go
type Translator struct{ strings map[string]string }
func Load(dir, lang string) (*Translator, error)
func (t *Translator) Tr(key string, vars map[string]any) string
```

## Deliverables

- `internal/command/handlers/file.go`
- `internal/command/handlers/url.go`
- `internal/command/handlers/tags.go`
- `internal/command/handlers/mode.go`
- `internal/i18n/translator.go`
- Update `internal/command/registry.go` to register new handlers
- Tests: each handler group with mocked bot dependencies

## Acceptance criteria

- All commands from `configuration.default.ini [commands]` respond correctly
- Language files load; `language = fr_FR` produces French responses
- `!ysearch beethoven` displays a table and sets shortlist; `!sl 1` queues the first result
