package audio

import (
	"slices"
	"testing"
)

func TestBuildFFmpegArgs_httpURL(t *testing.T) {
	args := buildFFmpegArgs("http://cdn.example.com/stream.m3u8", "warning")
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
	args := buildFFmpegArgs("https://cdn.example.com/stream.m3u8", "warning")
	if !slices.Contains(args, "-reconnect") {
		t.Error("expected -reconnect flag for https:// URL")
	}
}

func TestBuildFFmpegArgs_localPath(t *testing.T) {
	args := buildFFmpegArgs("/music/track.flac", "warning")
	if slices.Contains(args, "-reconnect") {
		t.Error("unexpected -reconnect flag for local file path")
	}
}

func TestBuildFFmpegArgs_verbosityPropagated(t *testing.T) {
	args := buildFFmpegArgs("http://example.com/s.m3u8", "debug")
	idx := slices.Index(args, "-v")
	if idx == -1 || args[idx+1] != "debug" {
		t.Errorf("expected -v debug in args, got %v", args)
	}
}

func TestBuildFFmpegArgs_inputFlag(t *testing.T) {
	url := "https://cdn.example.com/live.m3u8"
	args := buildFFmpegArgs(url, "warning")
	idx := slices.Index(args, "-i")
	if idx == -1 || args[idx+1] != url {
		t.Errorf("expected -i %q in args, got %v", url, args)
	}
}

func TestBuildFFmpegArgs_outputFlags(t *testing.T) {
	args := buildFFmpegArgs("http://example.com/s.m3u8", "warning")
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
