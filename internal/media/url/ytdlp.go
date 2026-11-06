// Package url implements URL-backed media items resolved through yt-dlp.
// Audio is always streamed; nothing is downloaded to disk.
package url

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os/exec"
	"strings"
	"time"

	neturl "net/url"

	"github.com/konradk/gotamusique/internal/config"
)

// ErrBinaryMissing is returned when the yt-dlp executable cannot be found.
var ErrBinaryMissing = errors.New("yt-dlp is not installed or not found (see [youtube_dl] binary)")

// YTInfo is the subset of yt-dlp's JSON output the bot uses.
type YTInfo struct {
	ID         string  `json:"id"`
	Title      string  `json:"title"`
	Uploader   string  `json:"uploader"`
	Channel    string  `json:"channel"`
	Duration   float64 `json:"duration"` // seconds; 0 when unknown
	Thumbnail  string  `json:"thumbnail"`
	WebpageURL string  `json:"webpage_url"`
	URL        string  `json:"url"`
	Type       string  `json:"_type"` // "playlist", "url", "video", ...
}

// DurationD returns the duration as a time.Duration.
func (i YTInfo) DurationD() time.Duration {
	return time.Duration(i.Duration * float64(time.Second))
}

// uploader returns the uploader or channel name.
func (i YTInfo) uploader() string {
	if i.Uploader != "" {
		return i.Uploader
	}
	return i.Channel
}

// entryURL returns the best page URL for a playlist entry.
func (i YTInfo) entryURL() string {
	if i.WebpageURL != "" {
		return i.WebpageURL
	}
	return i.URL
}

// CheckURL accepts only absolute http(s) URLs.
func CheckURL(raw string) error {
	u, err := neturl.Parse(raw)
	if err != nil || u.Host == "" {
		return fmt.Errorf("not a valid URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("only http and https URLs are supported")
	}
	return nil
}

// baseArgs returns the common yt-dlp arguments derived from config.
func baseArgs(cfg *config.Config) []string {
	args := []string{"--no-warnings", "--ignore-config"}
	yt := cfg.YoutubeDL
	if yt.CookieFile != "" {
		args = append(args, "--cookies", yt.CookieFile)
	}
	if yt.SourceAddress != "" {
		args = append(args, "--source-address", yt.SourceAddress)
	}
	if yt.UserAgent != "" {
		args = append(args, "--user-agent", yt.UserAgent)
	}
	return args
}

func binary(cfg *config.Config) string {
	if cfg.YoutubeDL.Binary != "" {
		return cfg.YoutubeDL.Binary
	}
	return "yt-dlp"
}

// run executes yt-dlp with args followed by "--" and rawURL. The URL is always
// passed after "--" so it can never be parsed as a flag, and no shell is used.
func run(ctx context.Context, cfg *config.Config, rawURL string, extra ...string) ([]byte, error) {
	if err := CheckURL(rawURL); err != nil {
		return nil, err
	}
	args := append(baseArgs(cfg), extra...)
	args = append(args, "--", rawURL)

	cmd := exec.CommandContext(ctx, binary(cfg), args...)
	var stdout, stderr bytes.Buffer
	// Child processes (e.g. ffmpeg spawned by yt-dlp) can keep the pipes open
	// after the context kills yt-dlp; WaitDelay force-closes them.
	cmd.WaitDelay = time.Second
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrNotExist) {
			return nil, ErrBinaryMissing
		}
		if ctx.Err() != nil {
			return nil, fmt.Errorf("yt-dlp timed out")
		}
		return nil, fmt.Errorf("yt-dlp failed: %s", lastLine(stderr.String(), err))
	}
	return stdout.Bytes(), nil
}

func lastLine(s string, fallback error) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return fallback.Error()
	}
	lines := strings.Split(s, "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// GetInfo fetches metadata for a single video (never expands a playlist).
func GetInfo(ctx context.Context, rawURL string, cfg *config.Config) (*YTInfo, error) {
	out, err := run(ctx, cfg, rawURL, "--dump-single-json", "--no-playlist")
	if err != nil {
		return nil, err
	}
	var info YTInfo
	if err := json.Unmarshal(out, &info); err != nil {
		return nil, fmt.Errorf("unexpected yt-dlp output: %w", err)
	}
	return &info, nil
}

// ResolveStream returns a direct audio URL for rawURL.
func ResolveStream(ctx context.Context, rawURL string, cfg *config.Config) (string, error) {
	out, err := run(ctx, cfg, rawURL, "-g", "-f", "bestaudio/best", "--no-playlist")
	if err != nil {
		return "", err
	}
	// -g prints one URL per line (video+audio for some formats); take the first.
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		if l := strings.TrimSpace(sc.Text()); l != "" {
			return l, nil
		}
	}
	return "", fmt.Errorf("yt-dlp returned no stream URL")
}

// FlatPlaylist lists playlist entries without resolving each one. A single
// video yields one entry. At most limit entries are returned (limit <= 0: all).
func FlatPlaylist(ctx context.Context, rawURL string, cfg *config.Config, limit int) ([]YTInfo, error) {
	extra := []string{"--flat-playlist", "--dump-json"}
	if limit > 0 {
		extra = append(extra, "--playlist-end", fmt.Sprint(limit))
	}
	out, err := run(ctx, cfg, rawURL, extra...)
	if err != nil {
		return nil, err
	}
	var entries []YTInfo
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e YTInfo
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return nil, fmt.Errorf("unexpected yt-dlp output: %w", err)
		}
		entries = append(entries, e)
		if limit > 0 && len(entries) >= limit {
			break
		}
	}
	return entries, nil
}
