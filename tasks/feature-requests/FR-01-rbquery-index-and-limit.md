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
!rbquery soma 20        ← show up to 20 results (default stays 10)
!rbplay 2               ← play result #2 from the last rbquery in this channel
!rbplay <uuid>          ← existing UUID flow still works
```

## Proposed changes

### 1 — Numbered index column in `!rbquery` output

`buildRBTable` in `internal/command/handlers.go` adds a leading `#` column (1-based).  
The UUID column is kept — UUIDs remain the authoritative identifier for `!rbplay <uuid>`.

```
Radio-Browser results for "soma":
| # | rbplay ID | Station Name       | Genre | Codec/Bitrate | Country |
|---|-----------|--------------------| ...
| 1 | <uuid>    | SomaFM Groove ...  | ...
| 2 | <uuid>    | SomaFM Drone Zone  | ...
```

### 2 — Optional result-count argument: `!rbquery <name> [N]`

Argument parsing in `handleRBQuery`:

- Split arg on the last whitespace token.
- If the last token is a positive integer, treat it as `limit`; the remainder is `name`.
- Otherwise the whole arg is `name` and `limit` defaults to `defaultSearchLimit` (10).
- Hard cap at `maxSearchLimit` (50) to avoid flooding the chat.

`radio.RadioBrowser.Search` signature changes to accept a limit:

```go
func (rb *RadioBrowser) Search(name string, limit int) ([]Station, error)
```

The `searchLimit` constant in `browser.go` is removed; callers supply the value.

### 3 — Last-query cache: `!rbplay <N>`

`handleRBPlay` needs to resolve an integer index against the results of the most
recent `!rbquery` in the same channel.

**Cache design:**

- Add `rbqueryCache map[string][]radio.Station` to the `Bot` struct (keyed by channel name).
- Expose it via a new `BotAPI` method: `SetRBCache(channel string, stations []radio.Station)` and `GetRBCache(channel string) []radio.Station`.
- `handleRBQuery` stores results in the cache after a successful search.
- `handleRBPlay`: if `arg` parses as a positive integer, look up index `arg-1` in the cache for the message's channel. If no cache entry exists, reply `"No recent !rbquery results for this channel."`. If index is out of range, reply `"Index out of range — use !rbquery to see available stations."`.
- UUID flow (`!rbplay <uuid>`) is unchanged; detection is: `strconv.Atoi` fails → treat as UUID.

## Deliverables

| File | Change |
|---|---|
| `internal/radio/browser.go` | `Search(name string, limit int)` — remove `searchLimit` constant |
| `internal/command/handlers.go` | `handleRBQuery`: parse optional count, pass limit to `Search`, store cache; `handleRBPlay`: integer-index branch; `buildRBTable`: add `#` column |
| `internal/command/dispatcher.go` / `BotAPI` | Add `SetRBCache` / `GetRBCache` to interface |
| `internal/bot/bot.go` | Add `rbqueryCache` field, implement new BotAPI methods |
| `internal/radio/browser_test.go` | Update `TestSearch_limitsResults` — caller now supplies limit; add test for custom limit value |
| `internal/command/handlers_test.go` | Tests for count parsing, index resolution, cache miss, index out-of-range |
| `tasks/phase1/07-commands.md` | Update `!rbquery` and `!rbplay` rows in the command table and the `!rbquery` response-format section |

## Documentation requirements

`tasks/phase1/07-commands.md` must be updated before the PR is merged:

- Command table: change `!rbquery <name>` → `!rbquery <name> [N]` with description "Search radio-browser.info; N results (default 10, max 50)."
- Command table: change `!rbplay <uuid>` → `!rbplay <uuid|N>` with description "Play station by UUID or by index from last !rbquery."
- `!rbquery` response-format section: add the `#` column to the example table.
- `!rbplay` usage note: document integer-index behaviour and cache-miss reply.

## Test requirements

All new code paths must have unit tests before merge:

### `internal/radio/browser_test.go`

- `TestSearch_limitsResults` — update to pass an explicit limit; assert `limit=N` appears in the request URI.
- `TestSearch_customLimit` — pass limit=20; assert `limit=20` in request URI.
- `TestSearch_limitCap` — assert that passing a limit > 50 is rejected (or capped) at the handler layer (radio package itself is not responsible for capping).

### `internal/command/handlers_test.go`

- `TestHandleRBQuery_defaultLimit` — no count suffix → limit=10 used.
- `TestHandleRBQuery_customLimit` — `"soma 20"` → limit=20.
- `TestHandleRBQuery_limitCap` — `"soma 999"` → limit capped at 50.
- `TestHandleRBQuery_setsCache` — after query, cache contains returned stations.
- `TestHandleRBPlay_indexResolvesFromCache` — `"2"` resolves to station[1] from cache.
- `TestHandleRBPlay_indexCacheMiss` — no prior query → "No recent !rbquery results" reply.
- `TestHandleRBPlay_indexOutOfRange` — `"99"` with a 3-item cache → out-of-range reply.
- `TestHandleRBPlay_uuidFlowUnchanged` — UUID string still calls `ByUUID`.

## Acceptance criteria

- `!rbquery soma` returns 10 results, each row prefixed with a 1-based index.
- `!rbquery soma 20` returns up to 20 results.
- `!rbquery soma 999` is silently capped to 50 results.
- `!rbplay 2` after `!rbquery soma` queues the second result from that search.
- `!rbplay 2` without a prior `!rbquery` replies "No recent !rbquery results for this channel."
- `!rbplay <uuid>` still works as before; no regression.
- `go test ./...` and `go vet ./...` pass.
