package library

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/konradk/gotamusique/internal/media/file"
)

func TestSafeName(t *testing.T) {
	for in, want := range map[string]string{
		"song.mp3":                  "song.mp3",
		"My Song (live).flac":       "My Song (live).flac",
		"../../etc/passwd.mp3":      "passwd.mp3",
		"/abs/path/a.mp3":           "a.mp3",
		`C:\Users\me\Music\a.mp3`:   "a.mp3",
		`..\..\a.mp3`:               "a.mp3",
		"a/b/../c.mp3":              "c.mp3",
		".hidden.mp3":               "hidden.mp3",
		"  spaced.mp3  ":            "spaced.mp3",
		"trailing.dot.":             "trailing.dot",
		"we:ird*na?me\"<>|.mp3":     "we_ird_na_me____.mp3",
		"ctrl\x00\x07\x1bchars.mp3": "ctrlchars.mp3",
		"tab\tin\nname.mp3":         "tabinname.mp3",
		"ümläut ♪ 音楽.mp3":           "ümläut ♪ 音楽.mp3",
		"":                          "",
		".":                         "",
		"..":                        "",
		"...":                       "",
		"/":                         "",
		"//":                        "",
		" . ":                       "",
	} {
		if got := SafeName(in); got != want {
			t.Errorf("SafeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSafeName_NeverEscapesAndIsBounded(t *testing.T) {
	long := strings.Repeat("ä", 300) + ".mp3" // 2-byte characters: must not be cut in half
	got := SafeName(long)
	if len(got) > maxNameBytes || !strings.HasSuffix(got, ".mp3") || !isValidUTF8(got) {
		t.Errorf("long name -> %d bytes %q", len(got), got)
	}
	for _, in := range []string{"a/../../b.mp3", "....//....//x.mp3", "%2e%2e/x.mp3", "..%2fx.mp3", "x.mp3/"} {
		got := SafeName(in)
		if strings.ContainsAny(got, `/\`) || got == ".." || got == "." || strings.HasPrefix(got, ".") {
			t.Errorf("SafeName(%q) = %q is not a plain name", in, got)
		}
	}
}

func isValidUTF8(s string) bool { return strings.ToValidUTF8(s, "\uFFFD") == s }

// ---- Store ----

const bigLimit = 1 << 20

func store(t *testing.T, s *Scanner, owner, name, content string) error {
	t.Helper()
	_, err := s.Store(context.Background(), owner, name, strings.NewReader(content), bigLimit)
	return err
}

// leftovers lists everything below dir, to prove failures leave nothing behind.
func leftovers(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(root, p)
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(out)
	return out
}

func TestStore_SavesAndIndexes(t *testing.T) {
	f := newFixture(t)
	s := f.scanner(Config{})
	rec, err := s.Store(context.Background(), "bob", "Song.mp3", strings.NewReader("audio-bytes"), bigLimit)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Path != "uploads/bob/Song.mp3" || rec.Title != "T-Song" || rec.Type != RecordType {
		t.Errorf("record = %+v", rec)
	}
	got, err := os.ReadFile(filepath.Join(f.root, "uploads", "bob", "Song.mp3"))
	if err != nil || string(got) != "audio-bytes" {
		t.Errorf("stored file = %q, %v", got, err)
	}
	eq(t, f.paths(t), []string{"uploads/bob/Song.mp3"})
	eq(t, leftovers(t, f.root), []string{"uploads/bob/Song.mp3"}) // no .part file left
}

func TestStore_NeverOverwrites(t *testing.T) {
	f := newFixture(t)
	s := f.scanner(Config{})
	for i, content := range []string{"first", "second", "third"} {
		rec, err := s.Store(context.Background(), "bob", "a.mp3", strings.NewReader(content), bigLimit)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"uploads/bob/a.mp3", "uploads/bob/a (1).mp3", "uploads/bob/a (2).mp3"}[i]
		if rec.Path != want {
			t.Errorf("upload %d stored as %q, want %q", i, rec.Path, want)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(f.root, "uploads", "bob", "a.mp3")); string(b) != "first" {
		t.Errorf("the first upload was overwritten: %q", b)
	}
}

// An existing file the library does not know about is just as protected.
func TestStore_DoesNotTouchAnExistingFile(t *testing.T) {
	f := newFixture(t)
	write(t, f.root, "uploads/bob/a.mp3", "precious")
	s := f.scanner(Config{})
	rec, err := s.Store(context.Background(), "bob", "a.mp3", strings.NewReader("new"), bigLimit)
	if err != nil || rec.Path != "uploads/bob/a (1).mp3" {
		t.Fatalf("= %+v, %v", rec, err)
	}
	if b, _ := os.ReadFile(filepath.Join(f.root, "uploads/bob/a.mp3")); string(b) != "precious" {
		t.Errorf("existing file changed: %q", b)
	}
}

func TestStore_ConcurrentSameNameGetsDistinctFiles(t *testing.T) {
	f := newFixture(t)
	s := f.scanner(Config{})
	const n = 20
	var wg sync.WaitGroup
	paths := make(chan string, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec, err := s.Store(context.Background(), "bob", "same.mp3", strings.NewReader(fmt.Sprint("content-", i)), bigLimit)
			if err != nil {
				t.Error(err)
				return
			}
			paths <- rec.Path
		}()
	}
	wg.Wait()
	close(paths)
	seen := map[string]bool{}
	for p := range paths {
		if seen[p] {
			t.Errorf("two uploads got %q", p)
		}
		seen[p] = true
	}
	if len(seen) != n || len(leftovers(t, f.root)) != n {
		t.Errorf("%d distinct paths, %d files on disk, want %d of each", len(seen), len(leftovers(t, f.root)), n)
	}
}

func TestStore_RejectsNonAudioNames(t *testing.T) {
	f := newFixture(t)
	s := f.scanner(Config{})
	for _, name := range []string{"notes.txt", "run.exe", "a.mp3.exe", "noextension", ".mp3", "x.php", "", "..", "x.html"} {
		if err := store(t, s, "bob", name, "data"); !errors.Is(err, ErrNotIndexable) {
			t.Errorf("%q: err = %v, want ErrNotIndexable", name, err)
		}
	}
	if got := leftovers(t, f.root); len(got) != 0 {
		t.Errorf("rejected names left files behind: %v", got)
	}
}

func TestStore_TraversalStaysInTheUploadFolder(t *testing.T) {
	f := newFixture(t)
	s := f.scanner(Config{})
	outside := t.TempDir()
	for _, c := range [][2]string{
		{"bob", "../../../evil.mp3"},
		{"bob", `..\..\evil.mp3`},
		{"../../escape", "x.mp3"},
		{"..", "y.mp3"},
		{"", "z.mp3"},
		{"a/b/c", "w.mp3"},
	} {
		rec, err := s.Store(context.Background(), c[0], c[1], strings.NewReader("data"), bigLimit)
		if err != nil {
			t.Errorf("%v: %v", c, err)
			continue
		}
		if !strings.HasPrefix(rec.Path, "uploads/") || strings.Contains(rec.Path, "..") || strings.Count(rec.Path, "/") != 2 {
			t.Errorf("%v stored at %q", c, rec.Path)
		}
	}
	for _, p := range leftovers(t, f.root) {
		if !strings.HasPrefix(p, "uploads/") {
			t.Errorf("a file landed outside uploads/: %s", p)
		}
	}
	if got := leftovers(t, outside); len(got) != 0 {
		t.Errorf("files outside the music folder: %v", got)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(f.root), "evil.mp3")); err == nil {
		t.Error("evil.mp3 escaped to the parent of the music folder")
	}
}

func TestStore_SizeLimit(t *testing.T) {
	f := newFixture(t)
	s := f.scanner(Config{})
	if _, err := s.Store(context.Background(), "bob", "ok.mp3", strings.NewReader("0123456789"), 10); err != nil {
		t.Errorf("exactly at the limit: %v", err)
	}
	_, err := s.Store(context.Background(), "bob", "big.mp3", strings.NewReader("0123456789X"), 10)
	if !errors.Is(err, ErrTooLarge) {
		t.Errorf("one byte over = %v, want ErrTooLarge", err)
	}
	eq(t, leftovers(t, f.root), []string{"uploads/bob/ok.mp3"})
	eq(t, f.paths(t), []string{"uploads/bob/ok.mp3"})

	// An endless stream is cut off, not buffered.
	_, err = s.Store(context.Background(), "bob", "endless.mp3", zeros{}, 1024)
	if !errors.Is(err, ErrTooLarge) {
		t.Errorf("endless stream = %v, want ErrTooLarge", err)
	}
}

type zeros struct{}

func (zeros) Read(p []byte) (int, error) { return len(p), nil }

func TestStore_EmptyFile(t *testing.T) {
	f := newFixture(t)
	if err := store(t, f.scanner(Config{}), "bob", "empty.mp3", ""); !errors.Is(err, ErrNotIndexable) {
		t.Errorf("err = %v, want ErrNotIndexable", err)
	}
	eq(t, leftovers(t, f.root), nil)
}

// What ffprobe says can contain server paths; the caller only gets generic text.
func TestStore_UnreadableAudioIsRemovedAndTheErrorIsGeneric(t *testing.T) {
	f := newFixture(t)
	s := NewScanner(f.db, Config{MusicFolder: f.root, Probe: func(abs string) (file.Metadata, error) {
		return file.Metadata{}, fmt.Errorf("ffprobe %q: exit status 1: /secret/server/path", abs)
	}})
	err := store(t, s, "bob", "bad.mp3", "not really audio")
	if !errors.Is(err, ErrNotIndexable) {
		t.Fatalf("err = %v, want ErrNotIndexable", err)
	}
	for _, leak := range []string{f.root, "/secret/server/path", "exit status"} {
		if strings.Contains(err.Error(), leak) {
			t.Errorf("the error leaks %q: %v", leak, err)
		}
	}
	eq(t, leftovers(t, f.root), nil)
	eq(t, f.paths(t), nil)
}

func TestStore_RejectsFilesWithoutAnAudioStream(t *testing.T) {
	f := newFixture(t)
	s := NewScanner(f.db, Config{MusicFolder: f.root, Probe: func(string) (file.Metadata, error) {
		return file.Metadata{Title: "movie", NoAudio: true}, nil
	}})
	if err := store(t, s, "bob", "video.mp3", "mp4 data"); !errors.Is(err, ErrNotIndexable) {
		t.Errorf("err = %v, want ErrNotIndexable", err)
	}
	eq(t, leftovers(t, f.root), nil)
	eq(t, f.paths(t), nil)
}

// entries lists everything directly in dir, folders included: leftovers only sees
// files, which is how a stray directory created through a symlink once slipped by.
func entries(t *testing.T, dir string) []string {
	t.Helper()
	es, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range es {
		out = append(out, e.Name())
	}
	return out
}

// spyReader reports what is in dir at the moment the upload body is first read.
// Store cleans up after a failure, so only looking while the bytes arrive can tell
// whether anything was ever created there.
type spyReader struct {
	dir     string
	read    bool
	present []string
}

func (s *spyReader) Read(p []byte) (int, error) {
	if !s.read {
		s.read = true
		entries, _ := os.ReadDir(s.dir)
		for _, e := range entries {
			s.present = append(s.present, e.Name())
		}
	}
	return copy(p, "audio"), io.EOF
}

// A symlink at uploads/<user> must not turn an upload into a write outside the
// music folder: the destination is checked before anything is created in it, so
// nothing is ever written there, not even for a moment.
func TestStore_RefusesASymlinkedUploadFolder(t *testing.T) {
	f := newFixture(t)
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(f.root, "uploads"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(f.root, "uploads", "bob")); err != nil {
		t.Skip("symlinks not available:", err)
	}
	spy := &spyReader{dir: outside}
	_, err := f.scanner(Config{}).Store(context.Background(), "bob", "a.mp3", spy, bigLimit)
	if !errors.Is(err, ErrNotIndexable) {
		t.Errorf("err = %v, want ErrNotIndexable", err)
	}
	if spy.read {
		t.Errorf("the body was read although the destination is outside the music folder; outside held %v", spy.present)
	}
	if got := entries(t, outside); len(got) != 0 {
		t.Errorf("something was created outside the music folder: %v", got)
	}

	// The same for a symlinked uploads/ itself. uploads/bob does not exist yet, so
	// a MkdirAll would create it inside the link's target before any check ran.
	f2 := newFixture(t)
	outside2 := t.TempDir()
	if err := os.Symlink(outside2, filepath.Join(f2.root, "uploads")); err != nil {
		t.Skip("symlinks not available:", err)
	}
	spy2 := &spyReader{dir: outside2}
	if _, err := f2.scanner(Config{}).Store(context.Background(), "bob", "a.mp3", spy2, bigLimit); !errors.Is(err, ErrNotIndexable) {
		t.Errorf("symlinked uploads/ = %v, want ErrNotIndexable", err)
	}
	if spy2.read || len(entries(t, outside2)) != 0 {
		t.Errorf("something was created through the symlinked uploads/ (body read: %v, outside now holds %v)", spy2.read, entries(t, outside2))
	}

	// uploads/ being a plain file, or uploads/bob a symlink to a file, is refused too.
	f3 := newFixture(t)
	write(t, f3.root, "uploads", "i am a file")
	if err := store(t, f3.scanner(Config{}), "bob", "a.mp3", "data"); !errors.Is(err, ErrNotIndexable) {
		t.Errorf("uploads/ is a file: err = %v, want ErrNotIndexable", err)
	}
}

func TestStore_HonoursTheIgnoreList(t *testing.T) {
	f := newFixture(t)
	for _, ign := range []Config{{IgnoredFolders: []string{"uploads"}}, {IgnoredFolders: []string{"bob"}}} {
		if err := store(t, f.scanner(ign), "bob", "a.mp3", "data"); !errors.Is(err, ErrNotIndexable) {
			t.Errorf("%v: err = %v, want ErrNotIndexable", ign.IgnoredFolders, err)
		}
	}
	eq(t, leftovers(t, f.root), nil)
}

func TestStore_FailedReadCleansUp(t *testing.T) {
	f := newFixture(t)
	boom := errors.New("connection reset")
	r := io.MultiReader(strings.NewReader("some bytes"), errReader{boom})
	_, err := f.scanner(Config{}).Store(context.Background(), "bob", "a.mp3", r, bigLimit)
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want the read error", err)
	}
	eq(t, leftovers(t, f.root), nil)
}

type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }

func TestStore_StopsWhenTheContextEnds(t *testing.T) {
	f := newFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	r := &cancelAfter{cancel: cancel, after: 3}
	_, err := f.scanner(Config{}).Store(ctx, "bob", "a.mp3", r, bigLimit)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	eq(t, leftovers(t, f.root), nil)
}

type cancelAfter struct {
	cancel func()
	after  int
	n      int
}

func (c *cancelAfter) Read(p []byte) (int, error) {
	c.n++
	if c.n == c.after {
		c.cancel()
	}
	time.Sleep(time.Millisecond)
	return copy(p, bytes.Repeat([]byte{'x'}, min(len(p), 16))), nil
}

func TestStore_FolderUnavailable(t *testing.T) {
	f := newFixture(t)
	s := NewScanner(f.db, Config{MusicFolder: filepath.Join(f.root, "missing"), Probe: f.probe.probe})
	if err := store(t, s, "bob", "a.mp3", "data"); !errors.Is(err, ErrFolderUnavailable) {
		t.Errorf("err = %v, want ErrFolderUnavailable", err)
	}
}

// IndexFile alone must also refuse a file without audio.
func TestIndexFile_RejectsNoAudio(t *testing.T) {
	f := newFixture(t)
	write(t, f.root, "v.mp3", "x")
	s := NewScanner(f.db, Config{MusicFolder: f.root, Probe: func(string) (file.Metadata, error) {
		return file.Metadata{NoAudio: true}, nil
	}})
	if _, err := s.IndexFile(context.Background(), "v.mp3"); !errors.Is(err, ErrNotIndexable) {
		t.Errorf("err = %v, want ErrNotIndexable", err)
	}
	eq(t, f.paths(t), nil)
}
