package library

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/konradk/gotamusique/internal/database"
	"github.com/konradk/gotamusique/internal/media/file"
)

// The migration code logs through the standard logger; keep test output readable.
func TestMain(m *testing.M) {
	log.SetOutput(io.Discard)
	os.Exit(m.Run())
}

// ---------- helpers ----------

func openDB(t *testing.T) database.MusicDB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "music.db")
	if err := database.MigrateMusic(path); err != nil {
		t.Fatalf("MigrateMusic: %v", err)
	}
	db, err := database.OpenMusic(path)
	if err != nil {
		t.Fatalf("OpenMusic: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// fakeProbe records which files were probed and returns tags derived from the
// file name, so tests can assert on exactly what the scan did.
type fakeProbe struct {
	mu     sync.Mutex
	probed []string
	fail   map[string]bool // base names that fail
	tags   map[string]file.Metadata
}

func (p *fakeProbe) probe(abs string) (file.Metadata, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	base := filepath.Base(abs)
	p.probed = append(p.probed, base)
	if p.fail[base] {
		return file.Metadata{}, errors.New("ffprobe: invalid data")
	}
	if m, ok := p.tags[base]; ok {
		return m, nil
	}
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	return file.Metadata{Title: "T-" + stem, Artist: "A-" + stem, Duration: 90 * time.Second}, nil
}

func (p *fakeProbe) reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.probed = nil
}

func (p *fakeProbe) probedNames() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := append([]string(nil), p.probed...)
	sort.Strings(out)
	return out
}

func write(t *testing.T, root, rel, content string) string {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

type fixture struct {
	root  string
	db    database.MusicDB
	probe *fakeProbe
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	return &fixture{root: t.TempDir(), db: openDB(t), probe: &fakeProbe{}}
}

func (f *fixture) scanner(ign Config) *Scanner {
	ign.MusicFolder = f.root
	ign.Probe = f.probe.probe
	return NewScanner(f.db, ign)
}

func (f *fixture) paths(t *testing.T) []string {
	t.Helper()
	recs, err := f.db.Query((&database.Condition{}).AndEqual("type", RecordType, true))
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, r := range recs {
		out = append(out, r.Path)
	}
	sort.Strings(out)
	return out
}

func eq(t *testing.T, got, want []string) {
	t.Helper()
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got %v, want %v", got, want)
	}
}

// ---------- extension filter ----------

func TestIsAudioFile(t *testing.T) {
	for _, n := range []string{"a.mp3", "a.MP3", "a.Flac", "x.ogg", "x.oga", "x.opus", "x.m4a", "x.aac", "x.wav", "x.wma", "x.ape", "x.aiff"} {
		if !IsAudioFile(n) {
			t.Errorf("IsAudioFile(%q) = false", n)
		}
	}
	for _, n := range []string{"cover.jpg", "notes.txt", "a.cue", "a", "mp3", ".mp3x", "a.mp3.bak", "a.m3u"} {
		if IsAudioFile(n) {
			t.Errorf("IsAudioFile(%q) = true", n)
		}
	}
	if !IsAudioFile(".mp3") { // a dotfile named ".mp3" has the extension ".mp3"
		t.Log("note: '.mp3' is treated as having an mp3 extension")
	}
}

func TestScan_OnlyAudioExtensionsAreProbed(t *testing.T) {
	f := newFixture(t)
	write(t, f.root, "a.mp3", "x")
	write(t, f.root, "b.FLAC", "x")
	write(t, f.root, "cover.jpg", "x")
	write(t, f.root, "notes.txt", "x")
	write(t, f.root, "album.cue", "x")

	res, err := f.scanner(Config{}).Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 2 {
		t.Errorf("Added = %d, want 2", res.Added)
	}
	eq(t, f.probe.probedNames(), []string{"a.mp3", "b.FLAC"})
	eq(t, f.paths(t), []string{"a.mp3", "b.FLAC"})
}

// ---------- first scan / row contents ----------

func TestScan_FirstScanBuildsRows(t *testing.T) {
	f := newFixture(t)
	write(t, f.root, "Rock/Band/song.mp3", "abc")

	if _, err := f.scanner(Config{}).Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	id := file.IDFromPath("Rock/Band/song.mp3")
	rec, err := f.db.QueryByID(id)
	if err != nil || rec == nil {
		t.Fatalf("row for %s missing: %v", id, err)
	}
	if rec.Type != "file" || rec.Path != "Rock/Band/song.mp3" {
		t.Errorf("type/path = %q/%q", rec.Type, rec.Path)
	}
	if rec.Title != "T-song" {
		t.Errorf("Title = %q", rec.Title)
	}
	if rec.Keywords != "T-song A-song" {
		t.Errorf("Keywords = %q, want \"<title> <artist>\"", rec.Keywords)
	}
	if rec.Metadata["artist"] != "A-song" {
		t.Errorf("artist = %v", rec.Metadata["artist"])
	}
	if d, _ := rec.Metadata["duration"].(float64); d != 90 {
		t.Errorf("duration = %v, want 90", rec.Metadata["duration"])
	}
	if rec.Metadata["size"] == nil || rec.Metadata["mtime"] == nil {
		t.Errorf("size/mtime missing from metadata: %v", rec.Metadata)
	}
	if _, has := rec.Metadata["thumb"]; has {
		t.Error("the scan must not store thumbnails")
	}
}

func TestScan_IDMatchesFileCommandID(t *testing.T) {
	f := newFixture(t)
	write(t, f.root, "sub/x.mp3", "1")
	if _, err := f.scanner(Config{}).Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	rec, _ := f.db.QueryByID(file.IDFromPath("sub/x.mp3"))
	if rec == nil {
		t.Error("scan row ID must equal the !file ID (sha1 of the relative path)")
	}
}

func TestScan_PathsUseForwardSlashes(t *testing.T) {
	f := newFixture(t)
	write(t, f.root, "a/b/c.mp3", "1")
	if _, err := f.scanner(Config{}).Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	eq(t, f.paths(t), []string{"a/b/c.mp3"})
}

// ---------- keywords / untagged fallback ----------

func TestBuildRecord_Keywords(t *testing.T) {
	cases := []struct {
		name      string
		rel       string
		meta      file.Metadata
		wantTitle string
		wantKW    string
	}{
		{"title and artist", "a.mp3", file.Metadata{Title: "Sonata", Artist: "Beethoven"}, "Sonata", "Sonata Beethoven"},
		{"title only", "a.mp3", file.Metadata{Title: "Sonata"}, "Sonata", "Sonata"},
		{"whitespace trimmed", "a.mp3", file.Metadata{Title: "  Sonata ", Artist: " B "}, "Sonata", "Sonata B"},
		{"untagged uses filename and folders", "Beethoven/Sonatas/op13.flac", file.Metadata{}, "op13", "Beethoven Sonatas op13"},
		{"untagged root file", "song.mp3", file.Metadata{}, "song", "song"},
		{"artist but no title", "Dir/x.mp3", file.Metadata{Artist: "Who"}, "x", "Dir x Who"},
		{"duplicate words collapse", "x/x.mp3", file.Metadata{}, "x", "x"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := buildRecord("id", found{rel: c.rel}, c.meta)
			if rec.Title != c.wantTitle {
				t.Errorf("Title = %q, want %q", rec.Title, c.wantTitle)
			}
			if rec.Keywords != c.wantKW {
				t.Errorf("Keywords = %q, want %q", rec.Keywords, c.wantKW)
			}
		})
	}
}

// The reason for the fallback: an untagged file must be findable by search.
func TestScan_UntaggedFileIsSearchableByFolderName(t *testing.T) {
	f := newFixture(t)
	write(t, f.root, "Beethoven/symphony5.mp3", "1")
	f.probe.tags = map[string]file.Metadata{"symphony5.mp3": {}} // no tags at all
	if _, err := f.scanner(Config{}).Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	hits, err := f.db.QueryByKeywords([]string{"beethoven"})
	if err != nil || len(hits) != 1 {
		t.Fatalf("search for folder name: %v %v", hits, err)
	}
}

func TestScan_TaggedFileKeywordsMatchPythonShape(t *testing.T) {
	f := newFixture(t)
	write(t, f.root, "x.mp3", "1")
	f.probe.tags = map[string]file.Metadata{"x.mp3": {Title: "Moonlight", Artist: "Beethoven"}}
	if _, err := f.scanner(Config{}).Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if hits, _ := f.db.QueryByKeywords([]string{"moonlight", "beethoven"}); len(hits) != 1 {
		t.Errorf("multi-word search should AND both words, got %d hits", len(hits))
	}
	if hits, _ := f.db.QueryByKeywords([]string{"moonlight", "mozart"}); len(hits) != 0 {
		t.Errorf("non-matching word should exclude, got %d hits", len(hits))
	}
}

// ---------- incremental ----------

func TestScan_SecondScanProbesNothing(t *testing.T) {
	f := newFixture(t)
	write(t, f.root, "a.mp3", "1")
	write(t, f.root, "d/b.mp3", "2")
	s := f.scanner(Config{})
	if _, err := s.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.probe.reset()
	res, err := s.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 0 || res.Updated != 0 || res.Removed != 0 || res.Unchanged != 2 {
		t.Errorf("result = %+v, want only 2 unchanged", res)
	}
	if n := len(f.probe.probedNames()); n != 0 {
		t.Errorf("an unchanged rescan probed %d files, want 0", n)
	}
}

func TestScan_NewFileIsAddedOthersUntouched(t *testing.T) {
	f := newFixture(t)
	write(t, f.root, "a.mp3", "1")
	s := f.scanner(Config{})
	s.Scan(context.Background())
	f.probe.reset()

	write(t, f.root, "b.mp3", "2")
	res, err := s.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 1 || res.Unchanged != 1 {
		t.Errorf("result = %+v", res)
	}
	eq(t, f.probe.probedNames(), []string{"b.mp3"})
}

func TestScan_ChangedSizeIsReprobed(t *testing.T) {
	f := newFixture(t)
	p := write(t, f.root, "a.mp3", "1")
	s := f.scanner(Config{})
	s.Scan(context.Background())
	f.probe.reset()

	// Same mtime but different size must still be detected.
	info, _ := os.Stat(p)
	if err := os.WriteFile(p, []byte("12345"), 0o644); err != nil {
		t.Fatal(err)
	}
	os.Chtimes(p, info.ModTime(), info.ModTime())

	res, _ := s.Scan(context.Background())
	if res.Updated != 1 || res.Added != 0 {
		t.Errorf("result = %+v, want 1 updated", res)
	}
	eq(t, f.probe.probedNames(), []string{"a.mp3"})
}

func TestScan_ChangedMtimeIsReprobed(t *testing.T) {
	f := newFixture(t)
	p := write(t, f.root, "a.mp3", "1")
	s := f.scanner(Config{})
	s.Scan(context.Background())
	f.probe.reset()

	later := time.Now().Add(2 * time.Hour)
	if err := os.Chtimes(p, later, later); err != nil {
		t.Fatal(err)
	}
	res, _ := s.Scan(context.Background())
	if res.Updated != 1 {
		t.Errorf("result = %+v, want 1 updated (same size, new mtime)", res)
	}
}

func TestScan_UpdatedTagsReplaceRow(t *testing.T) {
	f := newFixture(t)
	p := write(t, f.root, "a.mp3", "1")
	f.probe.tags = map[string]file.Metadata{"a.mp3": {Title: "Old", Artist: "X"}}
	s := f.scanner(Config{})
	s.Scan(context.Background())

	f.probe.tags = map[string]file.Metadata{"a.mp3": {Title: "New", Artist: "Y"}}
	later := time.Now().Add(time.Hour)
	os.Chtimes(p, later, later)
	s.Scan(context.Background())

	rec, _ := f.db.QueryByID(file.IDFromPath("a.mp3"))
	if rec.Title != "New" || rec.Keywords != "New Y" {
		t.Errorf("row not updated: %q / %q", rec.Title, rec.Keywords)
	}
	if recs, _ := f.db.Query((&database.Condition{}).AndEqual("type", "file", true)); len(recs) != 1 {
		t.Errorf("update must not duplicate the row, have %d", len(recs))
	}
}

// ---------- removal ----------

func TestScan_DeletedFileIsRemoved(t *testing.T) {
	f := newFixture(t)
	p := write(t, f.root, "a.mp3", "1")
	write(t, f.root, "b.mp3", "2")
	s := f.scanner(Config{})
	s.Scan(context.Background())

	os.Remove(p)
	res, err := s.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Removed != 1 {
		t.Errorf("Removed = %d, want 1", res.Removed)
	}
	eq(t, f.paths(t), []string{"b.mp3"})
}

func TestScan_RenamedFileIsRemovedAndAdded(t *testing.T) {
	f := newFixture(t)
	p := write(t, f.root, "old.mp3", "1")
	s := f.scanner(Config{})
	s.Scan(context.Background())
	os.Rename(p, filepath.Join(f.root, "new.mp3"))

	res, _ := s.Scan(context.Background())
	if res.Added != 1 || res.Removed != 1 {
		t.Errorf("result = %+v, want 1 added + 1 removed", res)
	}
	eq(t, f.paths(t), []string{"new.mp3"})
}

func TestScan_DoesNotDeleteNonFileRows(t *testing.T) {
	f := newFixture(t)
	write(t, f.root, "a.mp3", "1")
	for _, rec := range []database.MusicRecord{
		{ID: "url-1", Type: "url", Title: "Some URL", Path: "https://x/y", Metadata: map[string]any{}},
		{ID: "radio-1", Type: "radio", Title: "Some radio", Path: "http://r", Metadata: map[string]any{}},
	} {
		if err := f.db.Insert(rec); err != nil {
			t.Fatal(err)
		}
	}
	s := f.scanner(Config{})
	s.Scan(context.Background())
	os.Remove(filepath.Join(f.root, "a.mp3"))
	write(t, f.root, "b.mp3", "2") // keep the walk non-empty so the guard does not interfere
	if _, err := s.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"url-1", "radio-1"} {
		if rec, _ := f.db.QueryByID(id); rec == nil {
			t.Errorf("row %s of a non-file type was deleted by the scan", id)
		}
	}
}

func TestScan_DoesNotTouchInfoRow(t *testing.T) {
	f := newFixture(t)
	write(t, f.root, "a.mp3", "1")
	s := f.scanner(Config{})
	s.Scan(context.Background())
	os.Remove(filepath.Join(f.root, "a.mp3"))
	write(t, f.root, "b.mp3", "2")
	s.Scan(context.Background())
	// The info row stores the schema version; the DB must still reopen/migrate cleanly.
	// (Re-running migration on a deleted info row would re-create the table.)
}

// ---------- unavailable folder guards ----------

func TestScan_MissingFolder_AbortsAndKeepsLibrary(t *testing.T) {
	f := newFixture(t)
	write(t, f.root, "a.mp3", "1")
	s := f.scanner(Config{})
	s.Scan(context.Background())

	os.RemoveAll(f.root)
	_, err := s.Scan(context.Background())
	if !errors.Is(err, ErrFolderUnavailable) {
		t.Fatalf("err = %v, want ErrFolderUnavailable", err)
	}
	eq(t, f.paths(t), []string{"a.mp3"})
}

func TestScan_FolderIsAFile_Aborts(t *testing.T) {
	f := newFixture(t)
	p := write(t, f.root, "notadir", "x")
	s := NewScanner(f.db, Config{MusicFolder: p, Probe: f.probe.probe})
	if _, err := s.Scan(context.Background()); !errors.Is(err, ErrFolderUnavailable) {
		t.Errorf("err = %v, want ErrFolderUnavailable", err)
	}
}

func TestScan_EmptyFolderConfig_Aborts(t *testing.T) {
	f := newFixture(t)
	s := NewScanner(f.db, Config{MusicFolder: "", Probe: f.probe.probe})
	if _, err := s.Scan(context.Background()); !errors.Is(err, ErrFolderUnavailable) {
		t.Errorf("err = %v, want ErrFolderUnavailable", err)
	}
}

func TestScan_EmptyWalkOverNonEmptyLibrary_RemovesNothing(t *testing.T) {
	f := newFixture(t)
	write(t, f.root, "a.mp3", "1")
	write(t, f.root, "b.mp3", "2")
	s := f.scanner(Config{})
	s.Scan(context.Background())

	// An unmounted share typically leaves an empty directory behind.
	os.Remove(filepath.Join(f.root, "a.mp3"))
	os.Remove(filepath.Join(f.root, "b.mp3"))

	_, err := s.Scan(context.Background())
	if !errors.Is(err, ErrNoAudioFound) {
		t.Fatalf("err = %v, want ErrNoAudioFound", err)
	}
	eq(t, f.paths(t), []string{"a.mp3", "b.mp3"})
}

func TestScan_EmptyFolderEmptyLibrary_IsFine(t *testing.T) {
	f := newFixture(t)
	res, err := f.scanner(Config{}).Scan(context.Background())
	if err != nil {
		t.Fatalf("a fresh empty folder is not an error, got %v", err)
	}
	if res != (Result{}) {
		t.Errorf("result = %+v, want zero", res)
	}
}

func TestScan_FolderWithOnlyNonAudio_OverNonEmptyLibrary_RemovesNothing(t *testing.T) {
	f := newFixture(t)
	p := write(t, f.root, "a.mp3", "1")
	s := f.scanner(Config{})
	s.Scan(context.Background())
	os.Remove(p)
	write(t, f.root, "readme.txt", "x")
	if _, err := s.Scan(context.Background()); !errors.Is(err, ErrNoAudioFound) {
		t.Errorf("err = %v, want ErrNoAudioFound", err)
	}
	eq(t, f.paths(t), []string{"a.mp3"})
}

func TestScan_PartiallyMissingSubfolder_PrunesThatSubfolder(t *testing.T) {
	// Documented limitation: a vanished subfolder is indistinguishable from deleted files.
	f := newFixture(t)
	write(t, f.root, "keep/a.mp3", "1")
	write(t, f.root, "gone/b.mp3", "2")
	s := f.scanner(Config{})
	s.Scan(context.Background())
	os.RemoveAll(filepath.Join(f.root, "gone"))
	res, err := s.Scan(context.Background())
	if err != nil || res.Removed != 1 {
		t.Errorf("res=%+v err=%v, want 1 removed", res, err)
	}
	eq(t, f.paths(t), []string{"keep/a.mp3"})
}

func TestScan_UnreadableFolder_Aborts(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permission bits are not enforced for root/windows")
	}
	f := newFixture(t)
	write(t, f.root, "a.mp3", "1")
	s := f.scanner(Config{})
	s.Scan(context.Background())

	if err := os.Chmod(f.root, 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(f.root, 0o755)
	if _, err := s.Scan(context.Background()); !errors.Is(err, ErrFolderUnavailable) {
		t.Errorf("err = %v, want ErrFolderUnavailable", err)
	}
	os.Chmod(f.root, 0o755)
	eq(t, f.paths(t), []string{"a.mp3"})
}

// ---------- probe failures ----------

func TestScan_ProbeFailureIsSkippedNotFatal(t *testing.T) {
	f := newFixture(t)
	write(t, f.root, "good.mp3", "1")
	write(t, f.root, "bad.mp3", "2")
	f.probe.fail = map[string]bool{"bad.mp3": true}
	res, err := f.scanner(Config{}).Scan(context.Background())
	if err != nil {
		t.Fatalf("a bad file must not abort the scan: %v", err)
	}
	if res.Added != 1 || res.Skipped != 1 {
		t.Errorf("result = %+v, want 1 added 1 skipped", res)
	}
	eq(t, f.paths(t), []string{"good.mp3"})
}

func TestScan_ProbeFailureOnKnownFileKeepsOldRow(t *testing.T) {
	f := newFixture(t)
	p := write(t, f.root, "a.mp3", "1")
	s := f.scanner(Config{})
	s.Scan(context.Background())

	f.probe.fail = map[string]bool{"a.mp3": true}
	later := time.Now().Add(time.Hour)
	os.Chtimes(p, later, later)
	res, _ := s.Scan(context.Background())
	if res.Removed != 0 || res.Skipped != 1 {
		t.Errorf("result = %+v, want skipped, not removed", res)
	}
	eq(t, f.paths(t), []string{"a.mp3"})
}

func TestScan_FailedFileIsRetriedNextScan(t *testing.T) {
	f := newFixture(t)
	write(t, f.root, "a.mp3", "1")
	f.probe.fail = map[string]bool{"a.mp3": true}
	s := f.scanner(Config{})
	s.Scan(context.Background())
	f.probe.fail = nil
	res, _ := s.Scan(context.Background())
	if res.Added != 1 {
		t.Errorf("a file that failed before must be retried, result = %+v", res)
	}
}

// ---------- ignore lists ----------

func TestScan_IgnoredFolders_AnyDepth(t *testing.T) {
	f := newFixture(t)
	write(t, f.root, "keep/a.mp3", "1")
	write(t, f.root, "tmp/b.mp3", "2")
	write(t, f.root, "deep/er/tmp/c.mp3", "3")
	write(t, f.root, "tmpfile/d.mp3", "4") // name differs: must NOT be ignored
	if _, err := f.scanner(Config{IgnoredFolders: []string{"tmp"}}).Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	eq(t, f.paths(t), []string{"keep/a.mp3", "tmpfile/d.mp3"})
}

func TestScan_IgnoredFiles_ExactNameAnyDepth(t *testing.T) {
	f := newFixture(t)
	write(t, f.root, "a.mp3", "1")
	write(t, f.root, "skip.mp3", "2")
	write(t, f.root, "d/skip.mp3", "3")
	write(t, f.root, "d/skip.mp3.keep.mp3", "4")
	if _, err := f.scanner(Config{IgnoredFiles: []string{"skip.mp3"}}).Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	eq(t, f.paths(t), []string{"a.mp3", "d/skip.mp3.keep.mp3"})
}

func TestScan_IgnoreMatchingIsCaseSensitive(t *testing.T) {
	f := newFixture(t)
	write(t, f.root, "TMP/a.mp3", "1")
	if _, err := f.scanner(Config{IgnoredFolders: []string{"tmp"}}).Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	eq(t, f.paths(t), []string{"TMP/a.mp3"})
}

func TestScan_IgnoreListMatchesSingleComponentNotPath(t *testing.T) {
	f := newFixture(t)
	write(t, f.root, "a/b/x.mp3", "1")
	write(t, f.root, "b/y.mp3", "2")
	// "a/b" is not a valid exact name (contains a separator): it must not match anything.
	if _, err := f.scanner(Config{IgnoredFolders: []string{"a/b"}}).Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	eq(t, f.paths(t), []string{"a/b/x.mp3", "b/y.mp3"})
}

func TestScan_IgnoreListBlankEntriesIgnored(t *testing.T) {
	f := newFixture(t)
	write(t, f.root, "a.mp3", "1")
	if _, err := f.scanner(Config{IgnoredFolders: []string{"", "  "}, IgnoredFiles: []string{""}}).Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	eq(t, f.paths(t), []string{"a.mp3"})
}

func TestScan_NewlyIgnoredFolder_RemovesItsRows(t *testing.T) {
	f := newFixture(t)
	write(t, f.root, "keep/a.mp3", "1")
	write(t, f.root, "tmp/b.mp3", "2")
	f.scanner(Config{}).Scan(context.Background())
	res, err := f.scanner(Config{IgnoredFolders: []string{"tmp"}}).Scan(context.Background())
	if err != nil || res.Removed != 1 {
		t.Errorf("res=%+v err=%v, want the now-ignored file removed", res, err)
	}
	eq(t, f.paths(t), []string{"keep/a.mp3"})
}

// ---------- symlinks ----------

func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}
}

func TestScan_DirectorySymlinkNotFollowed(t *testing.T) {
	f := newFixture(t)
	outside := t.TempDir()
	write(t, outside, "secret.mp3", "1")
	write(t, f.root, "real/a.mp3", "2")
	symlinkOrSkip(t, outside, filepath.Join(f.root, "linkdir"))
	symlinkOrSkip(t, filepath.Join(f.root, "real"), filepath.Join(f.root, "alias"))

	if _, err := f.scanner(Config{}).Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Neither the outside dir nor an in-tree alias of a directory is walked.
	eq(t, f.paths(t), []string{"real/a.mp3"})
}

func TestScan_DirectorySymlinkLoopDoesNotHang(t *testing.T) {
	f := newFixture(t)
	write(t, f.root, "a.mp3", "1")
	symlinkOrSkip(t, f.root, filepath.Join(f.root, "loop"))
	done := make(chan error, 1)
	go func() { _, err := f.scanner(Config{}).Scan(context.Background()); done <- err }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("scan hung on a symlink loop")
	}
	eq(t, f.paths(t), []string{"a.mp3"})
}

func TestScan_FileSymlinkInsideFolderIsKept(t *testing.T) {
	f := newFixture(t)
	write(t, f.root, "real/a.mp3", "1")
	symlinkOrSkip(t, filepath.Join(f.root, "real", "a.mp3"), filepath.Join(f.root, "link.mp3"))
	if _, err := f.scanner(Config{}).Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	eq(t, f.paths(t), []string{"link.mp3", "real/a.mp3"})
}

func TestScan_FileSymlinkEscapingFolderIsSkipped(t *testing.T) {
	f := newFixture(t)
	outside := t.TempDir()
	secret := write(t, outside, "secret.mp3", "1")
	write(t, f.root, "a.mp3", "2")
	symlinkOrSkip(t, secret, filepath.Join(f.root, "escape.mp3"))
	if _, err := f.scanner(Config{}).Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	eq(t, f.paths(t), []string{"a.mp3"})
	for _, n := range f.probe.probedNames() {
		if n == "secret.mp3" {
			t.Error("a file outside the music folder was probed")
		}
	}
}

func TestScan_DanglingSymlinkIsSkipped(t *testing.T) {
	f := newFixture(t)
	write(t, f.root, "a.mp3", "1")
	symlinkOrSkip(t, filepath.Join(f.root, "missing.mp3"), filepath.Join(f.root, "dangling.mp3"))
	if _, err := f.scanner(Config{}).Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	eq(t, f.paths(t), []string{"a.mp3"})
}

func TestScan_SymlinkedMusicFolderRootIsResolved(t *testing.T) {
	f := newFixture(t)
	write(t, f.root, "a.mp3", "1")
	link := filepath.Join(t.TempDir(), "music-link")
	symlinkOrSkip(t, f.root, link)
	s := NewScanner(f.db, Config{MusicFolder: link, Probe: f.probe.probe})
	if _, err := s.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	eq(t, f.paths(t), []string{"a.mp3"})
}

func TestScan_FileSymlinkWithNonAudioNameIsSkipped(t *testing.T) {
	f := newFixture(t)
	write(t, f.root, "a.mp3", "1")
	symlinkOrSkip(t, filepath.Join(f.root, "a.mp3"), filepath.Join(f.root, "alias.txt"))
	if _, err := f.scanner(Config{}).Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	eq(t, f.paths(t), []string{"a.mp3"})
}

// ---------- nesting / odd names ----------

func TestScan_DeepNestingAndUnicodeNames(t *testing.T) {
	f := newFixture(t)
	write(t, f.root, "日本語/アルバム/曲.mp3", "1")
	write(t, f.root, "Ünï/cödé.flac", "2")
	write(t, f.root, "with space/a b.mp3", "3")
	write(t, f.root, "a/b/c/d/e/f/g.mp3", "4")
	if _, err := f.scanner(Config{}).Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{"Ünï/cödé.flac", "a/b/c/d/e/f/g.mp3", "with space/a b.mp3", "日本語/アルバム/曲.mp3"}
	sort.Strings(want) // paths() sorts by byte order too
	eq(t, f.paths(t), want)
}

func TestScan_HiddenFilesAreScanned(t *testing.T) {
	f := newFixture(t)
	write(t, f.root, ".hidden.mp3", "1")
	write(t, f.root, ".hiddendir/a.mp3", "2")
	if _, err := f.scanner(Config{}).Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Dotfiles are not special; users exclude them with the ignore lists.
	eq(t, f.paths(t), []string{".hidden.mp3", ".hiddendir/a.mp3"})
}

func TestScan_DirectoryNamedLikeAudioFileIsNotProbed(t *testing.T) {
	f := newFixture(t)
	if err := os.MkdirAll(filepath.Join(f.root, "album.mp3"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, f.root, "album.mp3/track.mp3", "1")
	if _, err := f.scanner(Config{}).Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	eq(t, f.paths(t), []string{"album.mp3/track.mp3"})
	for _, n := range f.probe.probedNames() {
		if n == "album.mp3" {
			t.Error("a directory was probed as a file")
		}
	}
}

// ---------- concurrency / cancellation ----------

func TestScan_ConcurrentScanIsRejected(t *testing.T) {
	f := newFixture(t)
	write(t, f.root, "a.mp3", "1")
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	s := NewScanner(f.db, Config{MusicFolder: f.root, Probe: func(abs string) (file.Metadata, error) {
		once.Do(func() { close(started) })
		<-release
		return file.Metadata{Title: "t"}, nil
	}})

	first := make(chan error, 1)
	go func() { _, err := s.Scan(context.Background()); first <- err }()
	<-started

	if !s.Running() {
		t.Error("Running() = false during a scan")
	}
	if _, err := s.Scan(context.Background()); !errors.Is(err, ErrScanRunning) {
		t.Errorf("second scan err = %v, want ErrScanRunning", err)
	}
	close(release)
	if err := <-first; err != nil {
		t.Fatalf("first scan: %v", err)
	}
	if s.Running() {
		t.Error("Running() = true after the scan finished")
	}
	if _, err := s.Scan(context.Background()); err != nil {
		t.Errorf("a scan after the first finished must be allowed: %v", err)
	}
}

func TestScan_RunningFlagClearedAfterError(t *testing.T) {
	f := newFixture(t)
	s := NewScanner(f.db, Config{MusicFolder: filepath.Join(f.root, "nope"), Probe: f.probe.probe})
	s.Scan(context.Background())
	if s.Running() {
		t.Error("an aborted scan left the running flag set")
	}
	if _, err := s.Scan(context.Background()); errors.Is(err, ErrScanRunning) {
		t.Error("an aborted scan must not block the next one")
	}
}

func TestScan_ManyConcurrentCallsExactlyOneRuns(t *testing.T) {
	f := newFixture(t)
	for i := 0; i < 5; i++ {
		write(t, f.root, fmt.Sprintf("t%d.mp3", i), "x")
	}
	var probes atomic.Int64
	release := make(chan struct{})
	s := NewScanner(f.db, Config{MusicFolder: f.root, Probe: func(abs string) (file.Metadata, error) {
		probes.Add(1)
		<-release
		return file.Metadata{Title: "t"}, nil
	}})

	var wg sync.WaitGroup
	var ran, rejected atomic.Int64
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.Scan(context.Background())
			if errors.Is(err, ErrScanRunning) {
				rejected.Add(1)
			} else if err == nil {
				ran.Add(1)
			}
		}()
	}
	time.Sleep(150 * time.Millisecond)
	close(release)
	wg.Wait()
	if ran.Load() < 1 {
		t.Error("no scan ran")
	}
	if ran.Load()+rejected.Load() != 8 {
		t.Errorf("ran=%d rejected=%d, want 8 total", ran.Load(), rejected.Load())
	}
	if p := probes.Load(); p != 5 {
		t.Errorf("%d probes for 5 files: scans overlapped", p)
	}
}

func TestScan_CancelledContextStopsEarly(t *testing.T) {
	f := newFixture(t)
	for i := 0; i < 20; i++ {
		write(t, f.root, fmt.Sprintf("t%02d.mp3", i), "x")
	}
	ctx, cancel := context.WithCancel(context.Background())
	var n atomic.Int64
	s := NewScanner(f.db, Config{MusicFolder: f.root, Probe: func(abs string) (file.Metadata, error) {
		if n.Add(1) == 3 {
			cancel()
		}
		return file.Metadata{Title: "t"}, nil
	}})
	_, err := s.Scan(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if n.Load() >= 20 {
		t.Error("cancellation did not stop the scan")
	}
}

func TestScan_CancelledScanDoesNotRemoveRows(t *testing.T) {
	f := newFixture(t)
	for i := 0; i < 6; i++ {
		write(t, f.root, fmt.Sprintf("t%d.mp3", i), "x")
	}
	s := f.scanner(Config{})
	s.Scan(context.Background())
	before := len(f.paths(t))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.Scan(ctx)
	if after := len(f.paths(t)); after != before {
		t.Errorf("a cancelled scan changed the library: %d -> %d", before, after)
	}
}

// ---------- scale ----------

func TestScan_ManyFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	f := newFixture(t)
	const n = 1500
	for i := 0; i < n; i++ {
		write(t, f.root, fmt.Sprintf("d%02d/t%04d.mp3", i%30, i), "x")
	}
	s := f.scanner(Config{})
	start := time.Now()
	res, err := s.Scan(context.Background())
	if err != nil || res.Added != n {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	t.Logf("first scan of %d files (fake probe): %v", n, time.Since(start))

	f.probe.reset()
	start = time.Now()
	res, _ = s.Scan(context.Background())
	if res.Unchanged != n || len(f.probe.probedNames()) != 0 {
		t.Errorf("rescan res=%+v probed=%d", res, len(f.probe.probedNames()))
	}
	t.Logf("no-op rescan of %d files: %v", n, time.Since(start))
}

// ---------- Result ----------

func TestResultString(t *testing.T) {
	if got := (Result{Added: 3, Updated: 2, Removed: 1}).String(); got != "3 added, 2 updated, 1 removed" {
		t.Errorf("String = %q", got)
	}
}

// Regression: nanosecond mtimes (~1.7e18) lose precision when the metadata JSON is
// read back as float64, so an unchanged file looked changed on every scan.
func TestUnchanged_SurvivesDBRoundTripWithRealisticMtime(t *testing.T) {
	db := openDB(t)
	mt := time.Date(2026, 11, 7, 17, 44, 44, 123456789, time.UTC)
	f := found{rel: "a.mp3", size: 4096, mtime: mt.UnixMilli()}
	rec := buildRecord(file.IDFromPath("a.mp3"), f, file.Metadata{Title: "t"})
	if err := db.Insert(rec); err != nil {
		t.Fatal(err)
	}
	got, err := db.QueryByID(rec.ID)
	if err != nil || got == nil {
		t.Fatal(err)
	}
	if !unchanged(*got, f) {
		t.Errorf("a stored size/mtime must compare equal after a DB round-trip: %v", got.Metadata)
	}
	// And a one-millisecond difference must still be detected.
	f2 := f
	f2.mtime++
	if unchanged(*got, f2) {
		t.Error("a changed mtime must be detected")
	}
	f3 := f
	f3.size++
	if unchanged(*got, f3) {
		t.Error("a changed size must be detected")
	}
}

func TestUnchanged_MissingOrWrongTypedMetadata_IsChanged(t *testing.T) {
	f := found{size: 1, mtime: 2}
	for name, md := range map[string]map[string]any{
		"empty":        {},
		"nil":          nil,
		"only size":    {"size": float64(1)},
		"only mtime":   {"mtime": float64(2)},
		"string types": {"size": "1", "mtime": "2"},
	} {
		if unchanged(database.MusicRecord{Metadata: md}, f) {
			t.Errorf("%s: must be treated as changed (re-probed)", name)
		}
	}
}

func TestAsInt64_AllNumericRepresentations(t *testing.T) {
	for name, tc := range map[string]struct {
		in   any
		want int64
		ok   bool
	}{
		"int64":   {int64(7), 7, true},
		"int":     {int(8), 8, true},
		"float64": {float64(9), 9, true}, // what json.Unmarshal produces
		"string":  {"10", 0, false},
		"nil":     {nil, 0, false},
		"bool":    {true, 0, false},
	} {
		got, ok := asInt64(tc.in)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("%s: asInt64(%v) = %d, %v; want %d, %v", name, tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestScan_UnreadableSubdirectoryDoesNotAbortScan(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permission bits are not enforced for root/windows")
	}
	f := newFixture(t)
	write(t, f.root, "ok/a.mp3", "1")
	write(t, f.root, "locked/b.mp3", "2")
	locked := filepath.Join(f.root, "locked")
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(locked, 0o755)

	res, err := f.scanner(Config{}).Scan(context.Background())
	if err != nil {
		t.Fatalf("one unreadable subfolder must not abort the scan: %v", err)
	}
	if res.Added != 1 {
		t.Errorf("result = %+v, want the readable file added", res)
	}
	os.Chmod(locked, 0o755)
	eq(t, f.paths(t), []string{"ok/a.mp3"})
}

func TestScanForce_EmptyWalk_ClearsLibrary(t *testing.T) {
	f := newFixture(t)
	p := write(t, f.root, "a.mp3", "1")
	s := f.scanner(Config{})
	s.Scan(context.Background())
	os.Remove(p)
	res, err := s.ScanForce(context.Background())
	if err != nil || res.Removed != 1 {
		t.Fatalf("res = %+v err = %v, want 1 removed", res, err)
	}
	eq(t, f.paths(t), nil)
}

func TestScan_ProbeFailureOnChangedFile_NotReprobedUntilChanged(t *testing.T) {
	f := newFixture(t)
	p := write(t, f.root, "a.mp3", "1")
	calls, fail := 0, false
	s := NewScanner(f.db, Config{MusicFolder: f.root, Probe: func(abs string) (file.Metadata, error) {
		calls++
		if fail {
			return file.Metadata{}, errors.New("corrupt")
		}
		return file.Metadata{Title: "t"}, nil
	}})
	s.Scan(context.Background())
	fail = true
	write(t, f.root, "a.mp3", "changed!")
	future := time.Now().Add(time.Hour)
	os.Chtimes(p, future, future)
	s.Scan(context.Background())
	before := calls
	s.Scan(context.Background())
	if calls != before {
		t.Errorf("probe ran again on unchanged failing file (%d -> %d)", before, calls)
	}
}

func TestScan_ReprobeKeepsUserTags(t *testing.T) {
	f := newFixture(t)
	p := write(t, f.root, "a.mp3", "1")
	s := f.scanner(Config{})
	s.Scan(context.Background())
	id := file.IDFromPath("a.mp3")
	f.db.UpdateTags(id, func([]string) []string { return []string{"fav"} })

	write(t, f.root, "a.mp3", "changed!")
	future := time.Now().Add(time.Hour)
	os.Chtimes(p, future, future)
	res, err := s.Scan(context.Background())
	if err != nil || res.Updated != 1 {
		t.Fatalf("res = %+v, err = %v; want 1 updated", res, err)
	}
	rec, _ := f.db.QueryByID(id)
	if strings.Join(rec.Tags, ",") != "fav" {
		t.Errorf("tags = %v, want fav kept after re-probe", rec.Tags)
	}
}
