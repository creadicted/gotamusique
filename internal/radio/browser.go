package radio

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	browserDiscoveryHost    = "all.api.radio-browser.info"
	browserHostSuffix       = ".api.radio-browser.info"
	browserDiscoveryTimeout = 5 * time.Second
	browserTimeout          = 10 * time.Second
	browserUserAgent        = "gotamusique/0.1"
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

	// discover, when set, returns live server base URLs (see discoverMirrors).
	// It runs once, lazily, on the first request; its results are tried before
	// baseURL and mirrors, which then act as the static fallback list.
	discover   func() []string
	discOnce   sync.Once
	discovered []string
}

// NewRadioBrowser returns a RadioBrowser that discovers live servers through
// DNS (all.api.radio-browser.info) and falls back to the static de1/nl1/at1 list.
func NewRadioBrowser() *RadioBrowser {
	return &RadioBrowser{
		client:   &http.Client{Timeout: browserTimeout},
		baseURL:  browserMirrors[0],
		mirrors:  browserMirrors[1:],
		discover: discoverMirrors,
	}
}

// discoverMirrors resolves all.api.radio-browser.info and reverse-resolves each
// address to its hostname (TLS needs the name, not the IP). The result is
// shuffled to spread load. It returns nil if discovery fails.
func discoverMirrors() []string {
	ctx, cancel := context.WithTimeout(context.Background(), browserDiscoveryTimeout)
	defer cancel()
	return resolveMirrors(ctx, net.DefaultResolver.LookupHost, net.DefaultResolver.LookupAddr)
}

func resolveMirrors(
	ctx context.Context,
	lookupHost func(context.Context, string) ([]string, error),
	lookupAddr func(context.Context, string) ([]string, error),
) []string {
	addrs, err := lookupHost(ctx, browserDiscoveryHost)
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, a := range addrs {
		names, err := lookupAddr(ctx, a)
		if err != nil {
			continue
		}
		for _, n := range names {
			n = strings.TrimSuffix(strings.ToLower(n), ".")
			if !strings.HasSuffix(n, browserHostSuffix) || seen[n] {
				continue
			}
			seen[n] = true
			out = append(out, "https://"+n+"/json")
			break
		}
	}
	rand.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
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

// targets returns discovered servers first, then baseURL and the static
// mirrors, without duplicates.
func (rb *RadioBrowser) targets() []string {
	if rb.discover != nil {
		rb.discOnce.Do(func() { rb.discovered = rb.discover() })
	}
	all := make([]string, 0, len(rb.discovered)+1+len(rb.mirrors))
	all = append(all, rb.discovered...)
	all = append(all, rb.baseURL)
	all = append(all, rb.mirrors...)
	seen := make(map[string]bool, len(all))
	out := all[:0]
	for _, t := range all {
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

// get tries each target in order, returning on the first success.
func (rb *RadioBrowser) get(path string, v interface{}) error {
	targets := rb.targets()

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
