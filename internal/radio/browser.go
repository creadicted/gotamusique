package radio

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

const (
	browserTimeout   = 10 * time.Second
	browserUserAgent = "gotamusique/0.1"
)

var browserMirrors = []string{
	"https://de1.api.radio-browser.info/json",
	"https://nl1.api.radio-browser.info/json",
	"https://at1.api.radio-browser.info/json",
}

// Station holds station data returned by the radio-browser.info API.
type Station struct {
	UUID        string `json:"stationuuid"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	URLResolved string `json:"url_resolved"`
	Codec       string `json:"codec"`
	Bitrate     int    `json:"bitrate"`
	Country     string `json:"countrycode"`
	Tags        string `json:"tags"`
	Homepage    string `json:"homepage"`
}

// RadioBrowser is a client for the radio-browser.info API.
type RadioBrowser struct {
	client  *http.Client
	baseURL string
	mirrors []string // additional fallbacks tried in order after baseURL
}

// NewRadioBrowser returns a RadioBrowser using the standard de1/nl1/at1 mirrors.
func NewRadioBrowser() *RadioBrowser {
	return &RadioBrowser{
		client:  &http.Client{Timeout: browserTimeout},
		baseURL: browserMirrors[0],
		mirrors: browserMirrors[1:],
	}
}

// newRadioBrowserWithClient is used in tests to inject a custom server.
// No mirror fallbacks are configured, so only baseURL is tried.
func newRadioBrowserWithClient(client *http.Client, baseURL string) *RadioBrowser {
	return &RadioBrowser{client: client, baseURL: baseURL}
}

// Search returns up to limit stations whose name matches the query.
func (rb *RadioBrowser) Search(name string, limit int) ([]Station, error) {
	path := fmt.Sprintf("/stations/byname/%s?limit=%d", url.PathEscape(name), limit)
	var stations []Station
	if err := rb.get(path, &stations); err != nil {
		return nil, err
	}
	return stations, nil
}

// ByUUID returns the station with the given UUID, or an error if not found.
func (rb *RadioBrowser) ByUUID(uuid string) (*Station, error) {
	path := fmt.Sprintf("/stations/byuuid/%s", url.PathEscape(uuid))
	var stations []Station
	if err := rb.get(path, &stations); err != nil {
		return nil, err
	}
	if len(stations) == 0 {
		return nil, fmt.Errorf("station %q not found", uuid)
	}
	return &stations[0], nil
}

// get tries baseURL then each mirror in order, returning on the first success.
func (rb *RadioBrowser) get(path string, v interface{}) error {
	targets := make([]string, 0, 1+len(rb.mirrors))
	targets = append(targets, rb.baseURL)
	targets = append(targets, rb.mirrors...)

	var lastErr error
	for _, base := range targets {
		if err := rb.doGet(base+path, v); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}
	return lastErr
}

func (rb *RadioBrowser) doGet(rawURL string, v interface{}) error {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", browserUserAgent)

	resp, err := rb.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("radio-browser API returned %d for %s", resp.StatusCode, rawURL)
	}

	return json.NewDecoder(resp.Body).Decode(v)
}
