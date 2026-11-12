package file

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestExtractTags_ErrorsOnMissingFile(t *testing.T) {
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not found")
	}
	if _, err := ExtractTags(filepath.Join(t.TempDir(), "nope.mp3")); err == nil {
		t.Error("want an error for a missing file")
	}
}

func TestExtractTags_ErrorsOnNonAudio(t *testing.T) {
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not found")
	}
	p := filepath.Join(t.TempDir(), "x.mp3")
	if err := os.WriteFile(p, []byte("this is not audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ExtractTags(p); err == nil {
		t.Error("want an error for a file that is not audio")
	}
}

func TestExtractTags_ReturnsDurationAndNoThumb(t *testing.T) {
	skipIfNoFFmpeg(t)
	m, err := ExtractTags(testWAV)
	if err != nil {
		t.Fatal(err)
	}
	if m.Duration <= 0 {
		t.Errorf("Duration = %v, want > 0", m.Duration)
	}
	if m.Thumb != "" {
		t.Error("ExtractTags must never extract artwork")
	}
}

func TestExtractMetadata_StillIncludesArtworkStep(t *testing.T) {
	skipIfNoFFmpeg(t)
	m, err := ExtractMetadata(testWAV)
	if err != nil {
		t.Fatal(err)
	}
	// A silent WAV has no embedded art, so Thumb is empty; what matters is that
	// the same tags come back as from ExtractTags and the call succeeds.
	tags, _ := ExtractTags(testWAV)
	if m.Title != tags.Title || m.Artist != tags.Artist || m.Duration != tags.Duration {
		t.Errorf("ExtractMetadata %+v differs from ExtractTags %+v", m, tags)
	}
}

func TestExtractThumb_MissingFileReturnsEmpty(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not found")
	}
	if got := ExtractThumb(filepath.Join(t.TempDir(), "nope.mp3")); got != "" {
		t.Errorf("ExtractThumb = %q, want empty", got)
	}
}

func TestParseProbe(t *testing.T) {
	audio := `{"streams":[{"codec_type":"audio"}],"format":{"duration":"12.5","tags":{"TITLE":"Song","artist":"Band"}}}`
	m, err := parseProbe([]byte(audio))
	if err != nil || m.Title != "Song" || m.Artist != "Band" || m.Duration != 12500*time.Millisecond || m.NoAudio {
		t.Errorf("audio file = %+v, %v", m, err)
	}

	// Cover art is a video stream next to the audio one; that is still audio.
	withArt := `{"streams":[{"codec_type":"audio"},{"codec_type":"video"}],"format":{"duration":"3"}}`
	if m, _ := parseProbe([]byte(withArt)); m.NoAudio {
		t.Error("an audio file with embedded cover art was taken for a video")
	}

	for name, js := range map[string]string{
		"video only": `{"streams":[{"codec_type":"video"}],"format":{"duration":"3"}}`,
		"data only":  `{"streams":[{"codec_type":"data"},{"codec_type":"subtitle"}],"format":{}}`,
	} {
		if m, err := parseProbe([]byte(js)); err != nil || !m.NoAudio {
			t.Errorf("%s: %+v, %v; want NoAudio", name, m, err)
		}
	}

	// No stream list (older output, a fake prober): unknown, so not flagged.
	if m, _ := parseProbe([]byte(`{"format":{"duration":"3"}}`)); m.NoAudio {
		t.Error("a missing stream list must not count as 'no audio'")
	}
	if _, err := parseProbe([]byte("not json")); err == nil {
		t.Error("garbage output must be an error")
	}
}
