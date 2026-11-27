package file

import (
	"context"
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
	// NoAudio is set when ffprobe listed the file's streams and none is audio
	// (a video-only or image file with an audio extension). It stays false when
	// the streams are unknown, so a prober that does not report them is unaffected.
	NoAudio bool
}

type ffprobeOutput struct {
	Format struct {
		Duration string            `json:"duration"`
		Tags     map[string]string `json:"tags"`
	} `json:"format"`
	Streams []struct {
		CodecType string `json:"codec_type"`
	} `json:"streams"`
}

// probeTimeout bounds one ffprobe run, so a crafted file cannot hang a scan or an
// upload forever.
const probeTimeout = 60 * time.Second

// ExtractMetadata runs ffprobe on absPath and returns the parsed metadata,
// including embedded album art (which costs an extra ffmpeg run).
func ExtractMetadata(absPath string) (Metadata, error) {
	m, err := ExtractTags(absPath)
	if err != nil {
		return Metadata{}, err
	}
	m.Thumb = ExtractThumb(absPath)
	return m, nil
}

// ExtractTags runs ffprobe only and returns title, artist and duration. It does
// not extract artwork, so it is cheap enough to run over a whole library.
func ExtractTags(absPath string) (Metadata, error) {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ffprobe", "-v", "quiet",
		"-print_format", "json",
		"-show_format", "-show_streams",
		absPath,
	).Output()
	if err != nil {
		if ctx.Err() != nil {
			return Metadata{}, fmt.Errorf("ffprobe %q: timed out after %v", absPath, probeTimeout)
		}
		return Metadata{}, fmt.Errorf("ffprobe %q: %w", absPath, err)
	}
	return parseProbe(out)
}

// parseProbe turns ffprobe's JSON output into Metadata.
func parseProbe(out []byte) (Metadata, error) {
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

	hasAudio := false
	for _, st := range fp.Streams {
		if st.CodecType == "audio" {
			hasAudio = true
		}
	}

	return Metadata{
		Title:    get("title"),
		Artist:   get("artist"),
		Duration: dur,
		NoAudio:  len(fp.Streams) > 0 && !hasAudio,
	}, nil
}

// ExtractThumb attempts to extract embedded album art from absPath.
// Returns a base64-encoded PNG string, or "" if none is found.
func ExtractThumb(absPath string) string {
	tmp, err := os.CreateTemp("", "gotamusique-thumb-*.png")
	if err != nil {
		return ""
	}
	tmp.Close()
	defer os.Remove(tmp.Name())

	if err := exec.Command(
		"ffmpeg", "-y", "-i", absPath, "-an", "-vcodec", "png", "-frames:v", "1", tmp.Name(),
	).Run(); err != nil {
		return ""
	}

	data, err := os.ReadFile(tmp.Name())
	if err != nil || len(data) == 0 {
		return ""
	}
	return base64.StdEncoding.EncodeToString(data)
}
