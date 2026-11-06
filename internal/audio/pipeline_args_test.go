package audio

import (
	"slices"
	"strings"
	"testing"
)

func TestBuildFFmpegArgs_httpURL(t *testing.T) {
	args := buildFFmpegArgs("http://cdn.example.com/stream.m3u8", "warning", false)
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
	args := buildFFmpegArgs("https://cdn.example.com/stream.m3u8", "warning", false)
	if !slices.Contains(args, "-reconnect") {
		t.Error("expected -reconnect flag for https:// URL")
	}
}

func TestBuildFFmpegArgs_localPath(t *testing.T) {
	args := buildFFmpegArgs("/music/track.flac", "warning", false)
	if slices.Contains(args, "-reconnect") {
		t.Error("unexpected -reconnect flag for local file path")
	}
}

func TestBuildFFmpegArgs_verbosityPropagated(t *testing.T) {
	args := buildFFmpegArgs("http://example.com/s.m3u8", "debug", false)
	idx := slices.Index(args, "-v")
	if idx == -1 || args[idx+1] != "debug" {
		t.Errorf("expected -v debug in args, got %v", args)
	}
}

func TestBuildFFmpegArgs_inputFlag(t *testing.T) {
	url := "https://cdn.example.com/live.m3u8"
	args := buildFFmpegArgs(url, "warning", false)
	idx := slices.Index(args, "-i")
	if idx == -1 || args[idx+1] != url {
		t.Errorf("expected -i %q in args, got %v", url, args)
	}
}

func TestBuildFFmpegArgs_outputFlags(t *testing.T) {
	args := buildFFmpegArgs("http://example.com/s.m3u8", "warning", false)
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
	args := buildFFmpegArgs("https://cdn.example.com/a.m4a", "warning", true)
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
	args := buildFFmpegArgs("https://cdn.example.com/live", "warning", false)
	if !strings.Contains(strings.Join(args, " "), "-reconnect_at_eof 1") {
		t.Errorf("live args must keep reconnect_at_eof: %v", args)
	}
}
