package library

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestIndexFile_AddsOneFileWithoutScanning(t *testing.T) {
	f := newFixture(t)
	write(t, f.root, "a.mp3", "x")
	write(t, f.root, "up/bob/b.mp3", "x")
	s := f.scanner(Config{})

	rec, err := s.IndexFile(context.Background(), "up/bob/b.mp3")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Path != "up/bob/b.mp3" || rec.Title != "T-b" || rec.Type != RecordType {
		t.Errorf("record = %+v", rec)
	}
	eq(t, f.paths(t), []string{"up/bob/b.mp3"}) // a.mp3 was not touched
	eq(t, f.probe.probedNames(), []string{"b.mp3"})
}

func TestIndexFile_KeepsUserTagsOnReindex(t *testing.T) {
	f := newFixture(t)
	write(t, f.root, "a.mp3", "x")
	s := f.scanner(Config{})
	rec, err := s.IndexFile(context.Background(), "a.mp3")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.db.UpdateTags(rec.ID, func([]string) []string { return []string{"rock"} }); err != nil {
		t.Fatal(err)
	}
	rec, err = s.IndexFile(context.Background(), "a.mp3")
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.Tags) != 1 || rec.Tags[0] != "rock" {
		t.Errorf("tags = %v, want [rock]", rec.Tags)
	}
}

func TestIndexFile_Rejects(t *testing.T) {
	f := newFixture(t)
	write(t, f.root, "a.mp3", "x")
	write(t, f.root, "notes.txt", "x")
	write(t, f.root, "skip/c.mp3", "x")
	write(t, f.root, "ign.mp3", "x")
	write(t, f.root, "bad.mp3", "x")
	f.probe.fail = map[string]bool{"bad.mp3": true}
	outside := t.TempDir()
	write(t, outside, "evil.mp3", "x")
	if err := os.Symlink(filepath.Join(outside, "evil.mp3"), filepath.Join(f.root, "evil.mp3")); err != nil && runtime.GOOS != "windows" {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(f.root, "dir.mp3"), 0o755); err != nil {
		t.Fatal(err)
	}
	s := f.scanner(Config{IgnoredFolders: []string{"skip"}, IgnoredFiles: []string{"ign.mp3"}})

	for _, rel := range []string{
		"", ".", "..", "../a.mp3", "/etc/passwd.mp3", "sub/../../a.mp3",
		"notes.txt", "skip/c.mp3", "ign.mp3", "missing.mp3", "dir.mp3", "evil.mp3", "bad.mp3",
	} {
		if _, err := s.IndexFile(context.Background(), rel); !errors.Is(err, ErrNotIndexable) {
			t.Errorf("IndexFile(%q) = %v, want ErrNotIndexable", rel, err)
		}
	}
	if got := f.paths(t); len(got) != 0 {
		t.Errorf("rejected files must leave the library untouched, got %v", got)
	}
}

func TestIndexFile_FolderUnavailable(t *testing.T) {
	f := newFixture(t)
	s := NewScanner(f.db, Config{MusicFolder: filepath.Join(f.root, "nope"), Probe: f.probe.probe})
	if _, err := s.IndexFile(context.Background(), "a.mp3"); !errors.Is(err, ErrFolderUnavailable) {
		t.Errorf("err = %v, want ErrFolderUnavailable", err)
	}
}

// A scan whose walk predates the upload must not delete the row IndexFile wrote
// between the walk and the scan's database read.
func TestIndexFile_DuringScanSurvivesReconcile(t *testing.T) {
	f := newFixture(t)
	write(t, f.root, "a.mp3", "x")
	s := f.scanner(Config{})
	s.afterWalk = func() {
		write(t, f.root, "new.mp3", "x") // arrives after the walk
		if _, err := s.IndexFile(context.Background(), "new.mp3"); err != nil {
			t.Error(err)
		}
	}
	if _, err := s.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	eq(t, f.paths(t), []string{"a.mp3", "new.mp3"})

	// The guard is per scan: once the file is gone, the next scan removes it.
	s.afterWalk = nil
	if err := os.Remove(filepath.Join(f.root, "new.mp3")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	eq(t, f.paths(t), []string{"a.mp3"})
}
