# FR-01 — rbquery: numeric index for rbplay + configurable result count

**Status:** open  
**Affects:** `internal/radio/`, `internal/command/`  
**Related milestone:** 1-07 (command dispatcher & radio commands)

## Motivation

`!rbplay` currently requires pasting the full UUID from `!rbquery` output.  
That is awkward in a chat interface where copying a UUID is friction.  
Users also can't control how many results `!rbquery` returns — the hardcoded cap of 10
is too low for common station names (e.g. "soma" returns far more than 10 results on
radio-browser.info).

Requested UX:

```
!rbquery soma -n 20     ← show up to 20 results (default stays 10)
!rbquery soma --limit 20  ← same, long form
!rbplay 2               ← play result #2 from the last rbquery in this channel
!rbplay <uuid>          ← existing UUID flow still works
```

## Design decisions

| Topic | Decision |
|---|---|
| Argument syntax | `-n N` / `--limit N` flag, end-only: `!rbquery <name> [-n N]` |
| Invalid flag value | Reply with usage error and bail, do not silently fall back to default |
| Cache location | Closure in `command` package; `BotAPI` interface unchanged |
| Cache key | Channel ID (`uint32` from `msg.Channels[0].ID`), not channel name |
| Cache concurrency | Protected by `rbCache.mu sync.Mutex` inside the command package |
| Table signature | `buildRBTable(name string, stations []radio.Station)` — name shown in header |

## Proposed changes

### 1 — Numbered index column in `!rbquery` output

`rbTableFull` and `rbTableShort` gain a leading `#` column (width 2, 1-based).  
The UUID column is kept — UUIDs remain the authoritative identifier for `!rbplay <uuid>`.  
The header now includes the search term: `Radio-Browser results for "soma":`.

```
Radio-Browser results for "soma":
| #  | rbplay ID                            | Station Name             | ...
|----|--------------------------------------|--------------------------|----
|  1 | <uuid>                               | SomaFM Groove Salad      | ...
|  2 | <uuid>                               | SomaFM Drone Zone        | ...
```

### 2 — Optional result-count flag: `!rbquery <name> [-n N]`

Argument parsing in `parseRBQueryArg`:

- Check whether the last two tokens are a recognised flag (`-n` or `--limit`) followed
  by a value.
- If yes: parse value as positive integer, cap at `maxSearchLimit` (50), treat remaining
  tokens as the station name. If the value is not a valid positive integer, reply with a
  usage error and bail.
- If the last token is `-n` or `--limit` with no following value, reply with usage error.
- If the resulting name is empty after stripping the flag, reply with usage error.
- Otherwise the whole arg is the name and `limit` defaults to `defaultSearchLimit` (10).

`radio.RadioBrowser.Search` signature changes to accept a limit:

```go
func (rb *RadioBrowser) Search(name string, limit int) ([]Station, error)
```

The `searchLimit` constant in `browser.go` is removed; callers supply the value.

### 3 — Last-query cache: `!rbplay <N>`

`handleRBPlay` needs to resolve an integer index against the results of the most
recent `!rbquery` in the same channel.

**Cache design:**

- `rbCache` struct (unexported) in the `command` package:
  ```go
  type rbCache struct {
      mu       sync.Mutex
      stations map[uint32][]radio.Station
  }
  ```
- `newRBCache() *rbCache` allocates an empty cache.
- `RegisterAll` in `registry.go` creates one shared `*rbCache` and one
  `*radio.RadioBrowser`, then registers the two RB handlers as closures:
  ```go
  cache := newRBCache()
  rb    := radio.NewRadioBrowser()
  d.Register(a("rb_query"), makeRBQueryHandler(cache, rb.Search), ...)
  d.Register(a("rb_play"),  makeRBPlayHandler(cache, rb.ByUUID), ...)
  ```
- `makeRBQueryHandler(cache *rbCache, searchFn func(string,int)([]radio.Station,error)) HandlerFunc`
  — stores results in cache after a successful search.
- `makeRBPlayHandler(cache *rbCache, byUUIDFn func(string)(*radio.Station,error)) HandlerFunc`
  — if `arg` parses as a positive integer (`strconv.Atoi` succeeds && n > 0), look up
  index `n-1` in the cache for the message's channel.  
  Cache miss: `"No recent !rbquery results for this channel."`.  
  Index out of range: `"Index out of range — use !rbquery to see available stations."`.  
  UUID flow (`strconv.Atoi` fails or n ≤ 0): unchanged, calls `byUUIDFn`.

`BotAPI` is **not** changed.

## Deliverables

| File | Change |
|---|---|
| `internal/radio/browser.go` | `Search(name string, limit int)` — remove `searchLimit` constant |
| `internal/command/handlers.go` | Add `rbCache`, `parseRBQueryArg`; replace `handleRBQuery`/`handleRBPlay` with `makeRBQueryHandler`/`makeRBPlayHandler`; update `buildRBTable` / `rbTableFull` / `rbTableShort` |
| `internal/command/registry.go` | Create shared cache + RadioBrowser; register closures; add `radio` import |
| `internal/radio/browser_test.go` | Update all `Search` calls to pass limit; add `TestSearch_customLimit` |
| `internal/command/handlers_test.go` | Update table-helper tests; add `parseRBQueryArg` tests and handler integration tests |
| `tasks/phase1/07-commands.md` | Update command table rows and `!rbquery` response-format section |

## Documentation requirements

`tasks/phase1/07-commands.md` must be updated before the PR is merged:

- Command table: `!rbquery <name>` → `!rbquery <name> [-n N]` with description
  "Search radio-browser.info; N results (default 10, max 50)."
- Command table: `!rbplay <uuid>` → `!rbplay <uuid|N>` with description
  "Play station by UUID or by index from last !rbquery."
- `!rbquery` response-format section: add `#` column and updated header to example table.
- `!rbplay` usage note: document integer-index behaviour and cache-miss reply.

## Test requirements

All new code paths must have unit tests before merge.

### `internal/radio/browser_test.go`

- Update all existing `rb.Search(name)` calls to `rb.Search(name, 10)`.
- `TestSearch_customLimit` — pass limit=20; assert `limit=20` in request URI.

(Capping is enforced at the handler layer, not in `radio.Search`.)

### `internal/command/handlers_test.go`

**`parseRBQueryArg` (pure function — no mocks needed):**
- `TestParseRBQueryArg_noFlag` — no flag → limit=10, name=full arg.
- `TestParseRBQueryArg_shortFlag` — `"soma -n 20"` → limit=20, name="soma".
- `TestParseRBQueryArg_longFlag` — `"soma --limit 20"` → limit=20, name="soma".
- `TestParseRBQueryArg_capLimit` — `"soma --limit 999"` → limit=50.
- `TestParseRBQueryArg_invalidLimit` — `"soma -n abc"` → error.
- `TestParseRBQueryArg_zeroLimit` — `"soma -n 0"` → error.
- `TestParseRBQueryArg_flagWithoutValue` — `"soma -n"` → error.
- `TestParseRBQueryArg_emptyNameWithFlag` — `"-n 20"` → error.
- `TestParseRBQueryArg_multiWordName` — `"bbc radio 4 -n 20"` → name="bbc radio 4", limit=20.

**Handler tests (inject mock searchFn / byUUIDFn):**
- `TestHandleRBQuery_NoArg_NoEnqueue` — update to use `makeRBQueryHandler`.
- `TestHandleRBQuery_defaultLimit` — no flag → searchFn called with limit=10.
- `TestHandleRBQuery_customLimit` — `"soma -n 20"` → searchFn called with limit=20.
- `TestHandleRBQuery_limitCap` — `"soma --limit 999"` → searchFn called with limit=50.
- `TestHandleRBQuery_invalidFlag` — `"soma -n abc"` → searchFn never called.
- `TestHandleRBQuery_setsCache` — after query, `cache.get(0)` contains returned stations.
- `TestHandleRBPlay_NoArg_NoEnqueue` — update to use `makeRBPlayHandler`.
- `TestHandleRBPlay_indexResolvesFromCache` — `"2"` resolves to `stations[1]`.
- `TestHandleRBPlay_indexCacheMiss` — no prior query → no enqueue.
- `TestHandleRBPlay_indexOutOfRange` — `"99"` with 3-item cache → no enqueue.
- `TestHandleRBPlay_uuidFlowUnchanged` — UUID string calls `byUUIDFn` and enqueues.

## Acceptance criteria

- `!rbquery soma` returns 10 results, each row prefixed with a 1-based `#` index.
- `!rbquery soma -n 20` returns up to 20 results.
- `!rbquery soma --limit 999` is silently capped to 50 results.
- `!rbquery soma -n abc` replies with a usage error and does not search.
- `!rbplay 2` after `!rbquery soma` queues the second result from that search.
- `!rbplay 2` without a prior `!rbquery` replies "No recent !rbquery results for this channel."
- `!rbplay <uuid>` still works as before; no regression.
- `go test ./...` and `go vet ./...` pass.
