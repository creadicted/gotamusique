# 2-06 — Full Command Set

**Status:** todo  
**Depends on:** 2-02, 2-03, 2-04, 2-05  
**Unlocks:** nothing (feature-complete bot)

## Objective

Add all commands that require Phase 2 subsystems (file library, URL media, advanced playlist modes, tags, DB).

## Additional commands (beyond Phase 1)

### File commands
| Command | Description |
|---|---|
| `!file <path>` | Add local file or folder |
| `!filematch <regex>` | Regex match on filenames, add all matches |
| `!listfile [regex]` | List files (paginated, 50/page) |
| `!search <keywords>` | Keyword search in music library |
| `!delete <n>` | Remove file from library (needs `delete_allowed` config) |

### URL commands
| Command | Description |
|---|---|
| `!url <url>` | Add YouTube/SC/etc. URL |
| `!playlist <url>` | Expand and queue a playlist |
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
| `!mode [one-shot\|repeat\|random\|autoplay]` | Get or set playback mode |
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
