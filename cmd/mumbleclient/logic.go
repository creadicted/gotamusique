package main

import (
	"html"
	"math"
	"regexp"
	"strings"
	"sync"
)

// The parts of the client that need no network, so they can be tested.

var (
	tagRE = regexp.MustCompile(`<[^>]*>`)
	brRE  = regexp.MustCompile(`(?i)<br\s*/?>`)
	urlRE = regexp.MustCompile(`https?://[^\s"'<>)]+`)
	wsRE  = regexp.MustCompile(`[ \t]+`)
)

// plainText turns a Mumble chat message (a small piece of HTML) into text a terminal can
// show: line breaks kept, tags removed, entities decoded.
func plainText(msg string) string {
	s := brRE.ReplaceAllString(msg, "\n")
	s = tagRE.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = wsRE.ReplaceAllString(s, " ")
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// findURL returns the address a !web reply points to. A one-time login link
// (".../login?token=...") wins over any other address in the message; without one the
// first address is returned (a bot without a login answers with its plain address).
func findURL(msg string) (string, bool) {
	// Addresses sit in an href or in the text; entities such as &amp; must be decoded first.
	s := html.UnescapeString(msg)
	all := urlRE.FindAllString(s, -1)
	for _, u := range all {
		if strings.Contains(u, "/login?token=") {
			return trimURL(u), true
		}
	}
	if len(all) > 0 {
		return trimURL(all[0]), true
	}
	return "", false
}

// trimURL drops punctuation that follows an address in running text.
func trimURL(u string) string { return strings.TrimRight(u, ".,;:!?") }

// audioStats accumulates what a listener heard. It is safe for concurrent use: every
// speaking user's stream is read by its own goroutine.
type audioStats struct {
	mu      sync.Mutex
	packets int
	samples int
	sumSq   float64
	peak    int
	from    map[string]bool
}

func newAudioStats() *audioStats { return &audioStats{from: map[string]bool{}} }

// Add records one packet of 16-bit samples from a user.
func (a *audioStats) Add(user string, samples []int16) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.packets++
	a.from[user] = true
	for _, s := range samples {
		v := int(s)
		a.sumSq += float64(v) * float64(v)
		if v < 0 {
			v = -v
		}
		if v > a.peak {
			a.peak = v
		}
	}
	a.samples += len(samples)
}

// Snapshot returns the totals so far.
func (a *audioStats) Snapshot() (packets, samples, peak int, rms float64, users []string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.samples > 0 {
		rms = math.Sqrt(a.sumSq / float64(a.samples))
	}
	for u := range a.from {
		users = append(users, u)
	}
	return a.packets, a.samples, a.peak, rms, users
}

// sampleRate is what Mumble delivers: 48 kHz, one channel.
const sampleRate = 48000

// seconds converts a sample count to seconds of audio.
func seconds(samples int) float64 { return float64(samples) / sampleRate }
