package status

import (
	"testing"
	"time"
)

func base() Snapshot {
	return Snapshot{
		Version: 1,
		Queue: []Entry{
			{ID: 1, Title: "a", Duration: time.Minute, Seekable: true},
			{ID: 2, Title: "b"},
		},
		Index: 0, Volume: 0.5, Mode: "one-shot",
		Scan: Scan{Available: true},
	}
}

func TestFingerprint_ChangesWithEveryRenderedField(t *testing.T) {
	want := base().Fingerprint()
	edits := map[string]func(*Snapshot){
		"entry id":       func(s *Snapshot) { s.Queue[0].ID = 9 },
		"entry title":    func(s *Snapshot) { s.Queue[0].Title = "A" },
		"entry duration": func(s *Snapshot) { s.Queue[1].Duration = time.Second },
		"entry seekable": func(s *Snapshot) { s.Queue[1].Seekable = true },
		"queue length":   func(s *Snapshot) { s.Queue = s.Queue[:1] },
		"index":          func(s *Snapshot) { s.Index = 1 },
		"playing":        func(s *Snapshot) { s.Playing = true },
		"pause":          func(s *Snapshot) { s.Pause = "manual" },
		"volume":         func(s *Snapshot) { s.Volume = 0.6 },
		"muted":          func(s *Snapshot) { s.Muted = true },
		"mode":           func(s *Snapshot) { s.Mode = "repeat" },
		"scan available": func(s *Snapshot) { s.Scan.Available = false },
		"scan running":   func(s *Snapshot) { s.Scan.Running = true },
		"scan result":    func(s *Snapshot) { s.Scan.LastResult = "1 added" },
		"scan error":     func(s *Snapshot) { s.Scan.LastError = "boom" },
	}
	for name, edit := range edits {
		s := base()
		edit(&s)
		if s.Fingerprint() == want {
			t.Errorf("%s did not change the fingerprint", name)
		}
	}
}

func TestFingerprint_IgnoresVersion(t *testing.T) {
	a, b := base(), base()
	b.Version = 99
	if a.Fingerprint() != b.Fingerprint() {
		t.Error("Version must not feed its own fingerprint")
	}
}

func TestFingerprint_StringBoundariesAreUnambiguous(t *testing.T) {
	a, b := base(), base()
	a.Queue = []Entry{{ID: 1, Title: "ab"}, {ID: 2, Title: "c"}}
	b.Queue = []Entry{{ID: 1, Title: "a"}, {ID: 2, Title: "bc"}}
	if a.Fingerprint() == b.Fingerprint() {
		t.Error("different titles collided")
	}
}

// The position moves constantly while playing and clients interpolate it, so it
// must not make every reading look new. When nothing plays it is static, so a
// change (a seek while paused) must show.
func TestFingerprint_PositionCountsOnlyWhenNotPlaying(t *testing.T) {
	playing := func(pos time.Duration) Snapshot {
		s := base()
		s.Playing, s.Position, s.PositionKnown = true, pos, true
		return s
	}
	if playing(time.Second).Fingerprint() != playing(2*time.Second).Fingerprint() {
		t.Error("the advancing position of a playing track must not change the fingerprint")
	}

	paused := func(pos time.Duration) Snapshot {
		s := base()
		s.Pause, s.Position, s.PositionKnown = "manual", pos, true
		return s
	}
	if paused(time.Second).Fingerprint() == paused(2*time.Second).Fingerprint() {
		t.Error("a changed resume position while paused must change the fingerprint")
	}
}
