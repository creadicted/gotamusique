package main

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/konradk/gotamusique/internal/library"
	"github.com/konradk/gotamusique/internal/queue"
	"github.com/konradk/gotamusique/internal/radio"
	"github.com/konradk/gotamusique/internal/status"
	"github.com/konradk/gotamusique/internal/web"
)

type dEntry struct {
	id       uint64
	title    string
	dur      time.Duration
	seekable bool
}

// demoBot is a stateful, in-memory stand-in for the real bot: a working queue, a
// simulated playhead, a small library, radio presets and search, uploads and a
// scan. It lets the web interface be tried, developed and tested without a Mumble
// server, ffmpeg or any music. It is a development tool and is not part of the
// released bot.
type demoBot struct {
	mu          sync.Mutex
	ver         uint64
	q           []dEntry
	nextID      uint64
	idx         int
	paused      string
	posBase     time.Duration
	startedAt   time.Time
	vol         float64
	muted       bool
	mode        string
	scanning    bool
	scanResult  string
	scanUntil   time.Time
	tracks      []library.Track
	tagsByTrack map[string][]string
}

var _ web.Bot = (*demoBot)(nil)

func newDemo() *demoBot {
	d := &demoBot{vol: 0.6, mode: "one-shot", nextID: 1}
	mk := func(p, title, artist string, secs int, tags ...string) {
		d.tracks = append(d.tracks, library.Track{Path: p, Title: title, Artist: artist, Duration: time.Duration(secs) * time.Second, Tags: tags})
	}
	mk("Blue Note Sessions/So What.mp3", "So What", "Miles Davis", 562, "jazz", "classic")
	mk("Blue Note Sessions/Take Five.mp3", "Take Five", "Dave Brubeck", 324, "jazz", "classic")
	mk("Blue Note Sessions/Blue in Green.mp3", "Blue in Green", "Miles Davis", 337, "jazz")
	mk("Rock/Heavy/Thunderstruck.mp3", "Thunderstruck", "AC/DC", 292, "rock", "loud")
	mk("Rock/Heavy/Paranoid.mp3", "Paranoid", "Black Sabbath", 168, "rock", "loud")
	mk("Rock/Seven Nation Army.mp3", "Seven Nation Army", "The White Stripes", 232, "rock")
	mk("Rock/Ümläut Überfall – 音楽.mp3", "Ümläut Überfall – 音楽 (a very long title that has to be shortened in the list)", "Die Ärzte", 201, "rock")
	mk("Ambient/Weightless.mp3", "Weightless", "Marconi Union", 480, "chill")
	mk("Ambient/Discreet Music.flac", "Discreet Music", "Brian Eno", 1800, "chill", "long")
	mk("loose track.mp3", "Loose track", "", 95)
	for i := 1; i <= 8; i++ {
		mk(fmt.Sprintf("uploads/alice/demo %02d.mp3", i), fmt.Sprintf("Demo %02d", i), "alice", 60+i*7)
	}
	d.add(dEntry{title: "So What – Miles Davis", dur: 562 * time.Second, seekable: true})
	d.add(dEntry{title: "[Radio] Jazz Yeah !"})
	d.add(dEntry{title: "Take Five – Dave Brubeck", dur: 324 * time.Second, seekable: true})
	d.add(dEntry{title: "Seven Nation Army – The White Stripes", dur: 232 * time.Second, seekable: true})
	d.startedAt = time.Now()
	d.posBase = 83 * time.Second
	return d
}

func (d *demoBot) add(e dEntry) uint64 {
	e.id = d.nextID
	d.nextID++
	d.q = append(d.q, e)
	d.ver++
	return e.id
}

func (d *demoBot) pos() time.Duration {
	if d.idx >= len(d.q) || d.paused != "" {
		return d.posBase
	}
	return d.posBase + time.Since(d.startedAt)
}

// advance moves on when a track has finished.
func (d *demoBot) advance() {
	for d.idx < len(d.q) && d.paused == "" {
		e := d.q[d.idx]
		if e.dur > 0 && d.pos() >= e.dur {
			d.idx++
			d.posBase, d.startedAt = 0, time.Now()
			d.ver++
			continue
		}
		break
	}
	if d.scanning && time.Now().After(d.scanUntil) {
		d.scanning = false
		d.scanResult = "3 added, 0 updated, 0 removed"
		d.ver++
	}
}

func (d *demoBot) Snapshot() status.Snapshot {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.advance()
	s := status.Snapshot{Version: d.ver, Index: d.idx, Volume: d.vol, Muted: d.muted, Mode: d.mode, Pause: d.paused,
		Scan: status.Scan{Available: true, Running: d.scanning, LastResult: d.scanResult}}
	for _, e := range d.q {
		s.Queue = append(s.Queue, status.Entry{ID: e.id, Title: e.title, Duration: e.dur, Seekable: e.seekable})
	}
	if d.idx < len(d.q) {
		s.Playing = d.paused == ""
		if d.q[d.idx].seekable {
			s.Position, s.PositionKnown = d.pos(), true
		}
	}
	return s
}

func (d *demoBot) touch() { d.ver++ }

func (d *demoBot) Pause() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.paused == "" && d.idx < len(d.q) {
		d.posBase = d.pos()
		d.paused = "manual"
		d.touch()
	}
	return true
}
func (d *demoBot) Resume() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.paused != "" {
		d.paused, d.startedAt = "", time.Now()
		d.touch()
	}
	return true
}
func (d *demoBot) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.idx, d.posBase, d.paused = 0, 0, ""
	d.idx = len(d.q)
	d.touch()
}
func (d *demoBot) Skip() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.idx < len(d.q) {
		d.idx++
		d.posBase, d.startedAt, d.paused = 0, time.Now(), ""
		d.touch()
	}
}
func (d *demoBot) Seek(p time.Duration) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.idx >= len(d.q) {
		return web.ErrNothingPlaying
	}
	if !d.q[d.idx].seekable {
		return web.ErrNotSeekable
	}
	d.posBase, d.startedAt = p, time.Now()
	d.touch()
	return nil
}
func (d *demoBot) SetVolume(pct int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.vol, d.muted = float64(pct)/100, false
	d.touch()
}
func (d *demoBot) Mute()   { d.mu.Lock(); d.muted = true; d.touch(); d.mu.Unlock() }
func (d *demoBot) Unmute() { d.mu.Lock(); d.muted = false; d.touch(); d.mu.Unlock() }
func (d *demoBot) SetPlaybackMode(n string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.mode = n
	d.touch()
	return nil
}
func (d *demoBot) Clear() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.q, d.idx, d.posBase = nil, 0, 0
	d.touch()
}
func (d *demoBot) RandomizeQueue() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.idx+1 < len(d.q) {
		rest := d.q[d.idx+1:]
		rand.Shuffle(len(rest), func(i, j int) { rest[i], rest[j] = rest[j], rest[i] })
	}
	d.touch()
}
func (d *demoBot) find(id uint64) int {
	for i, e := range d.q {
		if e.id == id {
			return i
		}
	}
	return -1
}
func (d *demoBot) PlayEntry(id uint64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	i := d.find(id)
	if i < 0 {
		return queue.ErrNotFound
	}
	d.idx, d.posBase, d.startedAt, d.paused = i, 0, time.Now(), ""
	d.touch()
	return nil
}
func (d *demoBot) RemoveEntry(id uint64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	i := d.find(id)
	if i < 0 {
		return queue.ErrNotFound
	}
	d.q = append(d.q[:i], d.q[i+1:]...)
	if i < d.idx {
		d.idx--
	} else if i == d.idx {
		d.posBase, d.startedAt = 0, time.Now()
	}
	d.touch()
	return nil
}
func (d *demoBot) MoveEntry(id, after uint64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	from := d.find(id)
	if from < 0 || (after != 0 && d.find(after) < 0) {
		return queue.ErrNotFound
	}
	curID := uint64(0)
	if d.idx < len(d.q) {
		curID = d.q[d.idx].id
	}
	e := d.q[from]
	d.q = append(d.q[:from], d.q[from+1:]...)
	to := 0
	if after != 0 {
		to = d.find(after) + 1
	}
	d.q = append(d.q[:to], append([]dEntry{e}, d.q[to:]...)...)
	if curID != 0 {
		d.idx = d.find(curID)
	}
	d.touch()
	return nil
}
func (d *demoBot) Add(ctx context.Context, src web.Source, pos queue.Position) (web.Added, error) {
	time.Sleep(250 * time.Millisecond) // like a real stream check
	d.mu.Lock()
	defer d.mu.Unlock()
	var e dEntry
	switch src.Type {
	case web.SourceLibrary:
		var tr *library.Track
		for i := range d.tracks {
			if d.tracks[i].Path == src.Path {
				tr = &d.tracks[i]
			}
		}
		if tr == nil {
			return web.Added{}, library.ErrNotInLibrary
		}
		e = dEntry{title: tr.Title + " – " + tr.Artist, dur: tr.Duration, seekable: true}
	case web.SourceRadio:
		switch {
		case src.Preset != "":
			e = dEntry{title: "[Radio] " + src.Preset}
		case src.UUID != "":
			e = dEntry{title: "[Radio] Station " + src.UUID}
		default:
			if !strings.HasPrefix(src.URL, "http") {
				return web.Added{}, fmt.Errorf("%w: not an http(s) URL", web.ErrBadSource)
			}
			e = dEntry{title: "[Radio] " + src.URL}
		}
	case web.SourceURL:
		if strings.Contains(src.URL, "youtube") {
			return web.Added{}, fmt.Errorf("%w: yt-dlp failed: Sign in to confirm you are not a bot", web.ErrBadSource)
		}
		e = dEntry{title: "Track from the web – Some Uploader", dur: 215 * time.Second, seekable: false}
	}
	id := d.add(e)
	if pos == queue.AfterCurrent && d.idx < len(d.q)-1 {
		from := len(d.q) - 1
		ne := d.q[from]
		d.q = d.q[:from]
		at := d.idx + 1
		d.q = append(d.q[:at], append([]dEntry{ne}, d.q[at:]...)...)
	}
	if len(d.q) == 1 {
		d.idx, d.startedAt = 0, time.Now()
	}
	d.touch()
	return web.Added{ID: id, Title: e.title}, nil
}
func (d *demoBot) SearchRadio(ctx context.Context, q string, limit int) ([]radio.Station, error) {
	time.Sleep(300 * time.Millisecond)
	if strings.Contains(q, "none") {
		return nil, nil
	}
	return []radio.Station{
		{UUID: "u-1", Name: q + " FM", Codec: "MP3", Bitrate: 128, Country: "DE"},
		{UUID: "u-2", Name: "Radio " + q + " Classic", Codec: "AAC", Bitrate: 64, Country: "GB"},
		{UUID: "u-3", Name: q, Codec: "OGG", Country: ""},
	}, nil
}
func (d *demoBot) RadioPresets() []web.RadioPreset {
	return []web.RadioPreset{{Name: "jazz", Comment: "Jazz Yeah !"}, {Name: "groove", Comment: "SomaFM Groove Salad"}, {Name: "plain"}}
}
func (d *demoBot) Library(q web.LibraryQuery) (web.LibraryPage, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	tagSet := map[string]bool{}
	for _, t := range d.tracks {
		for _, tag := range t.Tags {
			tagSet[tag] = true
		}
	}
	page := web.LibraryPage{}
	for t := range tagSet {
		page.Tags = append(page.Tags, t)
	}
	sort.Strings(page.Tags)
	prefix := ""
	if q.Dir != "" {
		prefix = q.Dir + "/"
	}
	filtering := q.Search != "" || len(q.Tags) > 0
	counts := map[string]int{}
	var here []library.Track
	for _, t := range d.tracks {
		if !strings.HasPrefix(t.Path, prefix) {
			continue
		}
		if filtering {
			text := strings.ToLower(t.Path + " " + t.Title + " " + t.Artist)
			ok := true
			for _, w := range strings.Fields(strings.ToLower(q.Search)) {
				ok = ok && strings.Contains(text, w)
			}
			for _, want := range q.Tags {
				found := false
				for _, have := range t.Tags {
					found = found || strings.EqualFold(have, want)
				}
				ok = ok && found
			}
			if ok {
				here = append(here, t)
			}
			continue
		}
		rest := t.Path[len(prefix):]
		if i := strings.IndexByte(rest, '/'); i >= 0 {
			counts[rest[:i]]++
		} else {
			here = append(here, t)
		}
	}
	for n, c := range counts {
		page.Dirs = append(page.Dirs, web.LibraryDir{Name: n, Path: prefix + n, Tracks: c})
	}
	sort.Slice(page.Dirs, func(i, j int) bool { return page.Dirs[i].Name < page.Dirs[j].Name })
	page.Total = len(here)
	lo := min(q.Offset, len(here))
	hi := min(lo+q.Limit, len(here))
	page.Tracks = here[lo:hi]
	return page, nil
}
func (d *demoBot) Upload(ctx context.Context, id web.Identity, filename string, r io.Reader) (library.Track, error) {
	n, _ := io.Copy(io.Discard, r)
	time.Sleep(600 * time.Millisecond)
	if strings.HasSuffix(filename, ".txt") {
		return library.Track{}, fmt.Errorf("%w: only audio files are accepted (.mp3 .flac .ogg ...)", library.ErrNotIndexable)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	tr := library.Track{Path: "uploads/" + id.Name + "/" + filename, Title: strings.TrimSuffix(filename, ".mp3"), Duration: time.Duration(n%300+30) * time.Second}
	d.tracks = append(d.tracks, tr)
	return tr, nil
}
func (d *demoBot) StartRescan() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.scanning {
		return library.ErrScanRunning
	}
	d.scanning, d.scanUntil = true, time.Now().Add(3*time.Second)
	d.touch()
	return nil
}
