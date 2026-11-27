package main

import (
	"math"
	"testing"
)

func TestPlainText(t *testing.T) {
	for in, want := range map[string]string{
		`Web UI: <a href="http://x/login?token=abc">open</a> (one use)`: "Web UI: open (one use)",
		"line one<br>line two<br/>three":                                "line one\nline two\nthree",
		"Tom &amp; Jerry &lt;3":                                         "Tom & Jerry <3",
		"  lots   of    space  ":                                        "lots of space",
		"<b>bold</b> and <i>italic</i>":                                 "bold and italic",
		"":                                                              "",
		"no tags at all":                                                "no tags at all",
		"<pre>code</pre>":                                               "code",
		"Sent you the web link in a private message.":                   "Sent you the web link in a private message.",
	} {
		if got := plainText(in); got != want {
			t.Errorf("plainText(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFindURL(t *testing.T) {
	const link = "http://127.0.0.1:8181/login?token=aB3_-xYz9"
	for name, c := range map[string]struct {
		msg  string
		want string
		ok   bool
	}{
		"the formatted reply":          {`Web UI: <a href="` + link + `">open</a> (one use, valid for 5 minutes)`, link, true},
		"the plain reply":              {"Web UI: " + link + " (one use, valid for 5 minutes)", link, true},
		"an escaped ampersand":         {`<a href="http://h/login?token=t&amp;x=1">open</a>`, "http://h/login?token=t&x=1", true},
		"trailing punctuation":         {"Open " + link + ".", link, true},
		"inside parentheses":           {"(" + link + ")", link, true},
		"https":                        {`<a href="https://music.example.com/login?token=abc">open</a>`, "https://music.example.com/login?token=abc", true},
		"the token link beats others":  {"Docs at http://127.0.0.1:8181/api/docs and " + link, link, true},
		"no login: the plain address":  {`Web UI: <a href="http://127.0.0.1:8181/">open</a>`, "http://127.0.0.1:8181/", true},
		"a refusal has no address":     {"The web interface is not enabled.", "", false},
		"nothing":                      {"", "", false},
		"a word that merely says http": {"the http protocol", "", false},
	} {
		got, ok := findURL(c.msg)
		if ok != c.ok || got != c.want {
			t.Errorf("%s: findURL = %q, %v; want %q, %v", name, got, ok, c.want, c.ok)
		}
	}
}

func TestAudioStats(t *testing.T) {
	a := newAudioStats()
	if p, s, peak, rms, users := a.Snapshot(); p != 0 || s != 0 || peak != 0 || rms != 0 || len(users) != 0 {
		t.Errorf("empty = %d %d %d %v %v", p, s, peak, rms, users)
	}

	// A full-scale-ish sine: rms is amplitude / sqrt(2).
	const amp = 8000
	buf := make([]int16, sampleRate) // one second
	for i := range buf {
		buf[i] = int16(amp * math.Sin(2*math.Pi*440*float64(i)/sampleRate))
	}
	a.Add("bot", buf[:960])
	a.Add("bot", buf[960:])
	p, s, peak, rms, users := a.Snapshot()
	if p != 2 || s != sampleRate || len(users) != 1 || users[0] != "bot" {
		t.Errorf("totals = %d packets %d samples %v", p, s, users)
	}
	if math.Abs(rms-amp/math.Sqrt2) > 20 {
		t.Errorf("rms = %.1f, want about %.1f", rms, amp/math.Sqrt2)
	}
	if peak < amp-5 || peak > amp {
		t.Errorf("peak = %d, want about %d", peak, amp)
	}
	if math.Abs(seconds(s)-1) > 1e-9 {
		t.Errorf("seconds = %v, want 1", seconds(s))
	}

	// Silence counts as packets but has no level.
	q := newAudioStats()
	q.Add("bot", make([]int16, 960))
	if p, _, peak, rms, _ := q.Snapshot(); p != 1 || peak != 0 || rms != 0 {
		t.Errorf("silence = %d packets, peak %d, rms %v", p, peak, rms)
	}
	// The most negative sample must not overflow when its absolute value is taken.
	q.Add("bot", []int16{math.MinInt16})
	if _, _, peak, _, _ := q.Snapshot(); peak != 32768 {
		t.Errorf("peak of MinInt16 = %d, want 32768", peak)
	}
}
