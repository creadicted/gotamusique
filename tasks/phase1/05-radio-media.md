# 1-05 — Radio Media Item

**Status:** done  
**Depends on:** 1-02  
**Unlocks:** 1-06, 1-07

## Objective

Implement the `RadioItem` type for HTTP audio streams, the `RadioBrowser` client
for station search via radio-browser.info, and the `MediaItem` interface that
the queue and pipeline use to stay decoupled from concrete item types.

## MediaItem interface (`internal/audio/media.go`)

```go
type MediaItem interface {
    StreamURL() string
    FormatTitle() string
}
```

`RadioItem` satisfies this interface implicitly. Phase 2 adds `FileItem` and `URLItem`.

## RadioItem (`internal/radio/item.go`)

```go
type RadioItem struct {
    URL  string
    Name string   // station name, preset Comment, or URL hostname as fallback
}

func (r *RadioItem) Validate() error      // HEAD with 5s timeout; falls back to plain GET on non-2xx
func (r *RadioItem) StreamURL() string    // returns r.URL
func (r *RadioItem) FormatTitle() string  // returns "[Radio] " + r.Name
```

Validation: send HEAD to the URL; accept any 2xx. If HEAD returns non-2xx,
retry with a plain GET. If both fail, return a wrapped error. Constructors
do not call Validate — callers do this explicitly.

`ID` field omitted until Phase 2 (SQLite is the first consumer).

### Constructors

```go
func NewRadioItemFromURL(rawURL string) *RadioItem
func NewRadioItemFromPreset(alias string, preset config.RadioPreset) *RadioItem
func NewRadioItemFromStation(s Station) *RadioItem
```

`NewRadioItemFromPreset`: `preset.Comment` → `Name`; URL hostname if Comment is empty.

## RadioBrowser client (`internal/radio/browser.go`)

```go
type Station struct {
    UUID     string `json:"stationuuid"`
    Name     string `json:"name"`
    URL      string `json:"url"`
    Codec    string `json:"codec"`
    Bitrate  int    `json:"bitrate"`
    Country  string `json:"countrycode"`
    Tags     string `json:"tags"`
    Homepage string `json:"homepage"`
}

type RadioBrowser struct { ... }

func NewRadioBrowser() *RadioBrowser
func (rb *RadioBrowser) Search(name string) ([]Station, error)   // /stations/byname/{name}?limit=10
func (rb *RadioBrowser) ByUUID(uuid string) (*Station, error)    // /stations/byuuid/{uuid}; error if empty
```

HTTP calls: 10-second timeout, `User-Agent: gotamusique/0.1`.

Mirrors tried in order on any error (including non-2xx): `de1`, `nl1`, `at1`.

For testability, `baseURL` and `mirrors` are unexported fields.
`newRadioBrowserWithClient(client, baseURL)` is used in tests (no mirrors).

## Deliverables

- `internal/audio/media.go` — `MediaItem` interface
- `internal/radio/item.go` — `RadioItem`, constructors, `Validate()`
- `internal/radio/browser.go` — `Station`, `RadioBrowser`
- `internal/radio/item_test.go` — unit tests (httptest mocks)
- `internal/radio/browser_test.go` — unit + mirror fallback tests; integration tests skip with `-short`

## Acceptance criteria

- `NewRadioItemFromURL("http://example.com/stream.mp3").Validate()` returns nil for a reachable server
- Unreachable URL returns a descriptive error mentioning the URL
- HEAD non-2xx → GET fallback; both non-2xx → error
- `RadioBrowser.Search("jazz")` returns a non-empty list against the real API (integration test, skip with `-short`)
- `RadioBrowser.ByUUID(uuid)` returns the correct station; empty array → named error
- Mirror fallback: first mirror returning non-2xx → second mirror tried
- `go vet ./...` clean; `go test -short ./...` all pass
