# 3-12 — Command leftovers

**Status:** todo (parked, not prioritised)  
**Depends on:** 3-06  
**Unlocks:** nothing

## Objective

Collects what was left over when 3-06 was closed. None of it is needed for the core bot; pick items
up when there is demand. Each item is independent and can ship on its own.

## Items

| Item | What it is | Size | Notes |
|---|---|---|---|
| `!remove <n>` | Remove one item from the queue | small | In the Python bot's `[commands]` but not in the Go bot. `queue.Queue.Remove(index)` already exists; needs a handler, `BotAPI` method, and handling of removing the playing item |
| `!ysearch <query>` | Search YouTube, show a results table and fill the shortlist | medium | Uses yt-dlp; `!sl 1` then queues the first result |
| `!yplay <query>` | Search YouTube, play the first result | small once `!ysearch` exists | |
| `!delete <n>` | Remove a file from the library | small, destructive | Needs the `delete_allowed` config flag; decide whether it deletes from disk or only from the DB |
| `!update` | Admin: print "not supported" | trivial | The bot has no update mechanism (`auto_check_update` was dropped) |
| In-memory item cache | `internal/media/cache`: `GetOrCreate`, `GetByID`, `FreeAndDelete` | internal | Holds `audio.MediaItem`s so the same track is not wrapped twice. Only worth it if tags need per-item state |
| i18n | `lang/<language>.json` plus `Tr(key, vars)` in every handler | large | Touches every handler, so do it last. `language = fr_FR` should give French replies. Acceptance below |
| "recent added" auto-tag | Tag items added in the last 24 h, remove it automatically | small | `ManageSpecialTags()` after a rescan |

## i18n sketch

```go
type Translator struct{ strings map[string]string }
func Load(dir, lang string) (*Translator, error)
func (t *Translator) Tr(key string, vars map[string]any) string
```

Deliverable: `internal/i18n/translator.go`.

## Acceptance criteria (per item)

- `!remove 2` removes the second queue item; removing the playing item moves on cleanly.
- `!ysearch beethoven` shows a table and sets the shortlist; `!sl 1` queues the first result.
- Language files load; `language = fr_FR` produces French responses.
- Each handler ships with tests (mocked bot) and updated docs and changelog.
