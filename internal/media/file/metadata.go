package file

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Metadata holds audio file metadata extracted by ffprobe.
type Metadata struct {
	Title    string
	Artist   string
	Duration time.Duration
	Thumb    string // base64-encoded PNG, or "" if none
}

type ffprobeOutput struct {
	Format struct {
		Duration string            `json:"duration"`
		Tags     map[string]string `json:"tags"`
	} `json:"format"`
}

// ExtractMetadata runs ffprobe on absPath and returns the parsed metadata.
func ExtractMetadata(absPath string) (Metadata, error) {
	out, err := exec.Command(
		"ffprobe", "-v", "quiet",
		"-print_format", "json",
		"-show_format", "-show_streams",
		absPath,
	).Output()
	if err != nil {
		return Metadata{}, fmt.Errorf("ffprobe %q: %w", absPath, err)
	}

	var fp ffprobeOutput
	if err := json.Unmarshal(out, &fp); err != nil {
		return Metadata{}, fmt.Errorf("ffprobe parse: %w", err)
	}

	get := func(key string) string {
		if v, ok := fp.Format.Tags[key]; ok {
			return v
		}
		if v, ok := fp.Format.Tags[strings.ToUpper(key)]; ok {
			return v
		}
		return ""
	}

	var dur time.Duration
	if s := fp.Format.Duration; s != "" {
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			dur = time.Duration(f * float64(time.Second))
		}
	}

	return Metadata{
		Title:    get("title"),
		Artist:   get("artist"),
		Duration: dur,
		Thumb:    extractThumb(absPath),
	}, nil
}

// extractThumb attempts to extract embedded album art from absPath.
// Returns a base64-encoded PNG string, or "" if none is found.
func extractThumb(absPath string) string {
	tmp, err := os.CreateTemp("", "gotamusique-thumb-*.png")
	if err != nil {
		return ""
	}
	tmp.Close()
	defer os.Remove(tmp.Name())

	if err := exec.Command(
		"ffmpeg", "-y", "-i", absPath, "-an", "-vcodec", "copy", tmp.Name(),
	).Run(); err != nil {
		return ""
	}

	data, err := os.ReadFile(tmp.Name())
	if err != nil || len(data) == 0 {
		return ""
	}
	return base64.StdEncoding.EncodeToString(data)
}
