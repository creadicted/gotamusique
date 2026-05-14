# 1-05 — Radio Media Item

**Status:** todo  
**Depends on:** 1-02  
**Unlocks:** 1-06, 1-07

## Objective

Implement the `RadioItem` type for HTTP audio streams and the `RadioBrowser` client for station search via radio-browser.info.

## RadioItem

```go
type RadioItem struct {
    ID    string   // sha1("radio" + url)
    URL   string
    Name  string   // station name or URL hostname if unknown
}

func (r *RadioItem) Validate() error    // HTTP HEAD with 5s timeout
func (r *RadioItem) StreamURL() string  // returns r.URL directly
func (r *RadioItem) FormatTitle() string
```

Validation: send `HEAD` to the URL; accept any 2xx response. If HEAD returns non-2xx or times out, try `GET` with `Range: bytes=0-0`. If both fail, return a `ValidationError`.

Radio streams have no duration and never finish — ffmpeg runs until `Interrupt()` is called.

## RadioBrowser client

API base: `https://de1.api.radio-browser.info/json`  
Fallback mirrors: `nl1`, `at1` (try in order on network error).

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

type RadioBrowser struct { client *http.Client }

func NewRadioBrowser() *RadioBrowser
func (rb *RadioBrowser) Search(name string) ([]Station, error)
func (rb *RadioBrowser) ByUUID(uuid string) (*Station, error)
```

HTTP calls use a 10-second timeout and a `User-Agent` header (radio-browser.info requires one).

## Config radio presets

The `[radio]` config section maps alias → `"URL [optional comment]"`.

```go
func NewRadioItemFromPreset(alias string, preset config.RadioPreset) *RadioItem
func NewRadioItemFromURL(url string) *RadioItem
func NewRadioItemFromStation(s Station) *RadioItem
```

## Deliverables

- `internal/radio/item.go`
- `internal/radio/browser.go`
- Unit tests with `httptest.NewServer` (mock HEAD response, mock JSON API response)

## Acceptance criteria

- `NewRadioItemFromURL("http://example.com/stream.mp3").Validate()` returns nil for a reachable server
- Unreachable URL returns a descriptive error
- `RadioBrowser.Search("jazz")` returns a non-empty list against the real API (integration test, skip in CI with `-short`)
- `RadioBrowser.ByUUID(uuid)` returns the correct station name
