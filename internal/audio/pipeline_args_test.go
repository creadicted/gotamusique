package audio

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func TestBuildFFmpegArgs_httpURL(t *testing.T) {
	args := buildFFmpegArgs("http://cdn.example.com/stream.m3u8", "warning", StreamOptions{})
	if !slices.Contains(args, "-reconnect") {
		t.Error("expected -reconnect flag for http:// URL")
	}
	if !slices.Contains(args, "-reconnect_at_eof") {
		t.Error("expected -reconnect_at_eof flag for http:// URL")
	}
	if !slices.Contains(args, "-reconnect_on_http_error") {
		t.Error("expected -reconnect_on_http_error flag for http:// URL")
	}
}

func TestBuildFFmpegArgs_httpsURL(t *testing.T) {
	args := buildFFmpegArgs("https://cdn.example.com/stream.m3u8", "warning", StreamOptions{})
	if !slices.Contains(args, "-reconnect") {
		t.Error("expected -reconnect flag for https:// URL")
	}
}

func TestBuildFFmpegArgs_localPath(t *testing.T) {
	args := buildFFmpegArgs("/music/track.flac", "warning", StreamOptions{})
	if slices.Contains(args, "-reconnect") {
		t.Error("unexpected -reconnect flag for local file path")
	}
}

func TestBuildFFmpegArgs_verbosityPropagated(t *testing.T) {
	args := buildFFmpegArgs("http://example.com/s.m3u8", "debug", StreamOptions{})
	idx := slices.Index(args, "-v")
	if idx == -1 || args[idx+1] != "debug" {
		t.Errorf("expected -v debug in args, got %v", args)
	}
}

func TestBuildFFmpegArgs_inputFlag(t *testing.T) {
	url := "https://cdn.example.com/live.m3u8"
	args := buildFFmpegArgs(url, "warning", StreamOptions{})
	idx := slices.Index(args, "-i")
	if idx == -1 || args[idx+1] != url {
		t.Errorf("expected -i %q in args, got %v", url, args)
	}
}

func TestBuildFFmpegArgs_outputFlags(t *testing.T) {
	args := buildFFmpegArgs("http://example.com/s.m3u8", "warning", StreamOptions{})
	for _, flag := range []string{"-ac", "-f", "-ar"} {
		if !slices.Contains(args, flag) {
			t.Errorf("expected %s in args", flag)
		}
	}
	last := args[len(args)-1]
	if last != "-" {
		t.Errorf("expected last arg to be \"-\" (stdout), got %q", last)
	}
}

func TestBuildFFmpegArgs_FiniteOmitsReconnectAtEOF(t *testing.T) {
	args := buildFFmpegArgs("https://cdn.example.com/a.m4a", "warning", StreamOptions{Finite: true})
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "reconnect_at_eof") {
		t.Errorf("finite args must not contain reconnect_at_eof: %v", args)
	}
	for _, want := range []string{"-reconnect 1", "-reconnect_streamed 1", "-reconnect_on_http_error 4xx,5xx"} {
		if !strings.Contains(joined, want) {
			t.Errorf("finite args missing %q: %v", want, args)
		}
	}
}

func TestBuildFFmpegArgs_LiveKeepsReconnectAtEOF(t *testing.T) {
	args := buildFFmpegArgs("https://cdn.example.com/live", "warning", StreamOptions{})
	if !strings.Contains(strings.Join(args, " "), "-reconnect_at_eof 1") {
		t.Errorf("live args must keep reconnect_at_eof: %v", args)
	}
}

func argValue(args []string, flag string) (string, bool) {
	i := slices.Index(args, flag)
	if i < 0 || i+1 >= len(args) {
		return "", false
	}
	return args[i+1], true
}

func TestBuildFFmpegArgs_HeadersBeforeInput(t *testing.T) {
	opts := StreamOptions{Headers: map[string]string{
		"Referer": "https://site.example/", "Accept-Language": "en-us", "User-Agent": "MyUA",
	}}
	args := buildFFmpegArgs("https://cdn.example.com/a.m4a", "warning", opts)

	h, ok := argValue(args, "-headers")
	if !ok {
		t.Fatalf("missing -headers: %v", args)
	}
	if h != "Accept-Language: en-us\r\nReferer: https://site.example/\r\n" {
		t.Errorf("-headers = %q (want sorted, CRLF-terminated, no User-Agent)", h)
	}
	if ua, _ := argValue(args, "-user_agent"); ua != "MyUA" {
		t.Errorf("-user_agent = %q, want MyUA", ua)
	}
	if slices.Index(args, "-headers") > slices.Index(args, "-i") || slices.Index(args, "-user_agent") > slices.Index(args, "-i") {
		t.Errorf("input options must precede -i: %v", args)
	}
}

func TestBuildFFmpegArgs_CookiesJoinedByNewline(t *testing.T) {
	opts := StreamOptions{Cookies: []string{"a=1; Domain=.x.com; Path=/", "b=2"}}
	args := buildFFmpegArgs("https://cdn.example.com/a.m4a", "warning", opts)
	c, ok := argValue(args, "-cookies")
	if !ok || c != "a=1; Domain=.x.com; Path=/\nb=2" {
		t.Errorf("-cookies = %q, %v", c, ok)
	}
}

func TestBuildFFmpegArgs_NoHeaderArgsForLocalFile(t *testing.T) {
	opts := StreamOptions{Headers: map[string]string{"Referer": "x"}, Cookies: []string{"a=1"}}
	args := buildFFmpegArgs("/music/a.flac", "warning", opts)
	for _, f := range []string{"-headers", "-user_agent", "-cookies"} {
		if slices.Contains(args, f) {
			t.Errorf("local file must not get %s: %v", f, args)
		}
	}
}

func TestBuildFFmpegArgs_HeaderInjectionDropped(t *testing.T) {
	opts := StreamOptions{
		Headers: map[string]string{
			"X-Evil":      "ok\r\nX-Injected: 1",
			"Bad\nName":   "v",
			"Bad:Name":    "v",
			"Cookie":      "stolen=1",
			"Referer":     "https://ok.example/",
			"X-Also-Evil": "a\nb",
		},
		Cookies: []string{"good=1", "bad=1\r\nSet-Cookie: x=2"},
	}
	args := buildFFmpegArgs("https://cdn.example.com/a.m4a", "warning", opts)
	h, _ := argValue(args, "-headers")
	if h != "Referer: https://ok.example/\r\n" {
		t.Errorf("-headers = %q, want only the clean Referer", h)
	}
	if c, _ := argValue(args, "-cookies"); c != "good=1" {
		t.Errorf("-cookies = %q, want only good=1", c)
	}
}

func TestBuildFFmpegArgs_FiniteWithHeadersKeepsBoth(t *testing.T) {
	opts := StreamOptions{Finite: true, Headers: map[string]string{"Referer": "r"}}
	args := buildFFmpegArgs("https://cdn.example.com/a.m4a", "warning", opts)
	if slices.Contains(args, "-reconnect_at_eof") {
		t.Errorf("finite must omit reconnect_at_eof: %v", args)
	}
	if !slices.Contains(args, "-headers") {
		t.Errorf("headers lost: %v", args)
	}
}

func TestBuildFFmpegArgs_StartAtSeeksBeforeInput(t *testing.T) {
	args := buildFFmpegArgs("/music/a.flac", "warning", StreamOptions{StartAt: 83*time.Second + 250*time.Millisecond})
	ss := slices.Index(args, "-ss")
	in := slices.Index(args, "-i")
	if ss == -1 || args[ss+1] != "83.250" {
		t.Fatalf("want -ss 83.250, got %v", args)
	}
	if ss > in {
		t.Errorf("-ss must come before -i (fast input seek), got %v", args)
	}
}

func TestBuildFFmpegArgs_NoStartAtNoSeek(t *testing.T) {
	if slices.Contains(buildFFmpegArgs("/music/a.flac", "warning", StreamOptions{}), "-ss") {
		t.Error("unexpected -ss without StartAt")
	}
}
