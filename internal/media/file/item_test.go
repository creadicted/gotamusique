package file

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

var testWAV string // absolute path to a 1-second silent WAV, set by TestMain

func TestMain(m *testing.M) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		// ffmpeg not available — tests that need it will skip individually
		os.Exit(m.Run())
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		os.Exit(m.Run())
	}

	dir, err := os.MkdirTemp("", "gotamusique-file-test-*")
	if err != nil {
		panic("cannot create temp dir: " + err.Error())
	}
	defer os.RemoveAll(dir)

	wav := filepath.Join(dir, "silence.wav")
	if err := exec.Command(
		"ffmpeg", "-y",
		"-f", "lavfi", "-i", "anullsrc=r=48000:cl=stereo",
		"-t", "1",
		wav,
	).Run(); err != nil {
		panic("cannot generate test WAV: " + err.Error())
	}
	testWAV = wav

	os.Exit(m.Run())
}

func skipIfNoFFmpeg(t *testing.T) {
	t.Helper()
	if testWAV == "" {
		t.Skip("ffmpeg/ffprobe not found")
	}
}

func TestIDFromPath(t *testing.T) {
	id1 := IDFromPath("music/song.mp3")
	id2 := IDFromPath("music/song.mp3")
	if id1 != id2 {
		t.Fatalf("IDFromPath is not deterministic: %q != %q", id1, id2)
	}
	if IDFromPath("a.mp3") == IDFromPath("b.mp3") {
		t.Fatal("different paths produced the same ID")
	}
	if len(id1) != 40 {
		t.Fatalf("expected 40-char sha1 hex, got %d chars", len(id1))
	}
}

func TestValidate_MissingFile(t *testing.T) {
	dir := t.TempDir()
	item := New("does_not_exist.mp3", dir)
	if err := item.Validate(); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestValidate_PathTraversal(t *testing.T) {
	dir := t.TempDir()
	item := New("../../etc/passwd", dir)
	if err := item.Validate(); err == nil {
		t.Fatal("expected error for path traversal")
	}
}

func TestValidate_OK(t *testing.T) {
	skipIfNoFFmpeg(t)
	musicDir := filepath.Dir(testWAV)
	item := New(filepath.Base(testWAV), musicDir)
	if err := item.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestPrepare_ExtractsMetadata(t *testing.T) {
	skipIfNoFFmpeg(t)
	musicDir := filepath.Dir(testWAV)
	item := New(filepath.Base(testWAV), musicDir)
	if err := item.Prepare(); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if item.Duration <= 0 {
		t.Errorf("expected positive duration, got %v", item.Duration)
	}
}

func TestFormatTitle(t *testing.T) {
	cases := []struct {
		artist, title, path string
		want                string
	}{
		{"The Beatles", "Help!", "help.mp3", "The Beatles - Help!"},
		{"", "Help!", "help.mp3", "Help!"},
		{"", "", "music/help.mp3", "help"},
		{"", "", "help.mp3", "help"},
	}
	for _, c := range cases {
		item := &FileItem{Artist: c.artist, Title: c.title, Path: c.path}
		if got := item.FormatTitle(); got != c.want {
			t.Errorf("FormatTitle(%q,%q,%q) = %q, want %q", c.artist, c.title, c.path, got, c.want)
		}
	}
}

func TestStreamURL_IsAbsolute(t *testing.T) {
	skipIfNoFFmpeg(t)
	musicDir := filepath.Dir(testWAV)
	item := New(filepath.Base(testWAV), musicDir)
	url := item.StreamURL()
	if !filepath.IsAbs(url) {
		t.Errorf("StreamURL should be absolute, got %q", url)
	}
}
