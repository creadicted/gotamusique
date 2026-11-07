package library

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/konradk/gotamusique/internal/database"
	"github.com/konradk/gotamusique/internal/media/file"
)

// These tests use the real ffprobe (no fake prober), so they exercise
// file.ExtractTags end to end. They skip when ffmpeg/ffprobe are not installed.

func needFFmpeg(t *testing.T) {
	t.Helper()
	for _, bin := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not found", bin)
		}
	}
}

// makeAudio writes a short tagged WAV (title/artist set when given).
func makeAudio(t *testing.T, path, title, artist string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	args := []string{"-y", "-f", "lavfi", "-i", "anullsrc=r=8000:cl=mono", "-t", "1"}
	if title != "" {
		args = append(args, "-metadata", "title="+title)
	}
	if artist != "" {
		args = append(args, "-metadata", "artist="+artist)
	}
	args = append(args, path)
	if out, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v\n%s", err, out)
	}
}

func TestIntegration_RealFFprobeScan(t *testing.T) {
	needFFmpeg(t)
	root := t.TempDir()
	makeAudio(t, filepath.Join(root, "Beethoven", "moon.wav"), "Moonlight Sonata", "Beethoven")
	makeAudio(t, filepath.Join(root, "untagged.wav"), "", "")
	os.WriteFile(filepath.Join(root, "broken.mp3"), []byte("not audio at all"), 0o644)
	os.WriteFile(filepath.Join(root, "cover.jpg"), []byte("x"), 0o644)

	db := openDB(t)
	s := NewScanner(db, Config{MusicFolder: root}) // default prober = file.ExtractTags
	res, err := s.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 2 || res.Skipped != 1 {
		t.Errorf("result = %+v, want 2 added (moon, untagged) and 1 skipped (broken.mp3)", res)
	}

	moon, _ := db.QueryByID(file.IDFromPath("Beethoven/moon.wav"))
	if moon == nil {
		t.Fatal("tagged file missing")
	}
	if moon.Title != "Moonlight Sonata" || moon.Keywords != "Moonlight Sonata Beethoven" {
		t.Errorf("title/keywords = %q / %q", moon.Title, moon.Keywords)
	}
	if d, _ := moon.Metadata["duration"].(float64); d < 0.5 || d > 1.5 {
		t.Errorf("duration = %v, want about 1s", moon.Metadata["duration"])
	}

	un, _ := db.QueryByID(file.IDFromPath("untagged.wav"))
	if un == nil || un.Title != "untagged" {
		t.Errorf("untagged file should fall back to its filename, got %+v", un)
	}

	if hits, _ := db.QueryByKeywords([]string{"moonlight"}); len(hits) != 1 {
		t.Errorf("search by title: %d hits", len(hits))
	}
}

func TestIntegration_RescanProbesNothingNew(t *testing.T) {
	needFFmpeg(t)
	root := t.TempDir()
	makeAudio(t, filepath.Join(root, "a.wav"), "A", "X")
	db := openDB(t)
	s := NewScanner(db, Config{MusicFolder: root})
	if _, err := s.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	res, err := s.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Unchanged != 1 || res.Added != 0 || res.Updated != 0 {
		t.Errorf("a second real scan should find nothing to do, got %+v", res)
	}
}

func TestIntegration_ScanStoresNoThumbnail(t *testing.T) {
	needFFmpeg(t)
	root := t.TempDir()
	makeAudio(t, filepath.Join(root, "a.wav"), "A", "X")
	db := openDB(t)
	NewScanner(db, Config{MusicFolder: root}).Scan(context.Background())
	recs, _ := db.Query((&database.Condition{}).AndEqual("type", "file", true))
	if len(recs) != 1 {
		t.Fatalf("rows = %d", len(recs))
	}
	for k := range recs[0].Metadata {
		if k == "thumb" || k == "thumbnail" {
			t.Errorf("the scan stored artwork under %q", k)
		}
	}
}
