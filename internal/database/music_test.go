package database

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func openTestMusic(t *testing.T) MusicDB {
	t.Helper()
	dir := t.TempDir()
	musicPath := filepath.Join(dir, "music.db")
	if err := Migrate(filepath.Join(dir, "settings.db"), musicPath); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	db, err := OpenMusic(musicPath)
	if err != nil {
		t.Fatalf("OpenMusic: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func sampleRecord(id string) MusicRecord {
	return MusicRecord{
		ID:       id,
		Type:     "file",
		Title:    "Test Song " + id,
		Keywords: "test song " + id,
		Tags:     []string{"rock", "classic"},
		Path:     "/music/" + id + ".mp3",
		Metadata: map[string]any{"duration": float64(180), "artist": "Band"},
	}
}

func TestMusic_InsertQuery_RoundTrip(t *testing.T) {
	db := openTestMusic(t)

	rec := sampleRecord("abc123")
	if err := db.Insert(rec); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	got, err := db.QueryByID("abc123")
	if err != nil {
		t.Fatalf("QueryByID: %v", err)
	}
	if got == nil {
		t.Fatal("QueryByID: got nil")
	}
	if got.Title != rec.Title {
		t.Errorf("Title: got %q, want %q", got.Title, rec.Title)
	}
	if got.Path != rec.Path {
		t.Errorf("Path: got %q, want %q", got.Path, rec.Path)
	}
	if len(got.Tags) != 2 {
		t.Errorf("Tags: got %v, want [rock classic]", got.Tags)
	}
	if got.Metadata["artist"] != "Band" {
		t.Errorf("Metadata artist: got %v", got.Metadata["artist"])
	}
}

func TestMusic_Insert_Upsert(t *testing.T) {
	db := openTestMusic(t)

	rec := sampleRecord("dup")
	_ = db.Insert(rec)
	rec.Title = "Updated Title"
	if err := db.Insert(rec); err != nil {
		t.Fatalf("second Insert: %v", err)
	}

	got, _ := db.QueryByID("dup")
	if got.Title != "Updated Title" {
		t.Errorf("upsert: got %q, want %q", got.Title, "Updated Title")
	}
}

func TestMusic_QueryByKeywords(t *testing.T) {
	db := openTestMusic(t)
	_ = db.Insert(sampleRecord("k1"))
	_ = db.Insert(MusicRecord{ID: "k2", Type: "file", Title: "Jazz Night", Keywords: "jazz night", Tags: []string{"jazz"}, Metadata: map[string]any{}})

	results, err := db.QueryByKeywords([]string{"jazz"})
	if err != nil {
		t.Fatalf("QueryByKeywords: %v", err)
	}
	if len(results) != 1 || results[0].ID != "k2" {
		t.Errorf("got %v, want [k2]", results)
	}
}

func TestMusic_QueryByTags(t *testing.T) {
	db := openTestMusic(t)
	_ = db.Insert(sampleRecord("t1"))
	_ = db.Insert(MusicRecord{ID: "t2", Type: "file", Title: "Pop Song", Keywords: "pop", Tags: []string{"pop"}, Metadata: map[string]any{}})

	results, err := db.QueryByTags([]string{"rock"})
	if err != nil {
		t.Fatalf("QueryByTags: %v", err)
	}
	if len(results) != 1 || results[0].ID != "t1" {
		t.Errorf("got %v, want [t1]", results)
	}
}

func TestMusic_Delete(t *testing.T) {
	db := openTestMusic(t)
	_ = db.Insert(sampleRecord("del1"))
	_ = db.Insert(sampleRecord("del2"))

	cond := (&Condition{}).AndEqual("id", "del1", true)
	if err := db.Delete(cond); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	got, _ := db.QueryByID("del1")
	if got != nil {
		t.Error("del1 still present after Delete")
	}
	got, _ = db.QueryByID("del2")
	if got == nil {
		t.Error("del2 deleted unexpectedly")
	}
}

func TestMusic_AllPaths(t *testing.T) {
	db := openTestMusic(t)
	_ = db.Insert(sampleRecord("p1"))
	_ = db.Insert(MusicRecord{ID: "p2", Type: "url", Title: "Stream", Metadata: map[string]any{}})

	paths, err := db.AllPaths()
	if err != nil {
		t.Fatalf("AllPaths: %v", err)
	}
	if len(paths) != 1 {
		t.Errorf("got %d paths, want 1 (only file type)", len(paths))
	}
}

func TestMusic_AllTags(t *testing.T) {
	db := openTestMusic(t)
	_ = db.Insert(sampleRecord("tag1"))
	_ = db.Insert(MusicRecord{ID: "tag2", Type: "file", Title: "X", Tags: []string{"jazz", "rock"}, Metadata: map[string]any{}})

	tags, err := db.AllTags()
	if err != nil {
		t.Fatalf("AllTags: %v", err)
	}
	// rock appears in both, jazz in one — deduplication expected
	seen := map[string]int{}
	for _, t := range tags {
		seen[t]++
	}
	if seen["rock"] != 1 {
		t.Errorf("rock should appear exactly once, got %d", seen["rock"])
	}
	if seen["jazz"] != 1 {
		t.Errorf("jazz should appear exactly once, got %d", seen["jazz"])
	}
}

func TestMusic_InfoRowExcluded(t *testing.T) {
	db := openTestMusic(t)
	results, err := db.Query(nil)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	for _, r := range results {
		if r.ID == "info" {
			t.Error("sentinel row id='info' appeared in Query results")
		}
	}
}

func TestMusic_QueryRandom(t *testing.T) {
	db := openTestMusic(t)
	for i := 0; i < 5; i++ {
		_ = db.Insert(sampleRecord(string(rune('a' + i))))
	}

	results, err := db.QueryRandom(3, nil)
	if err != nil {
		t.Fatalf("QueryRandom: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("got %d results, want 3", len(results))
	}
}

// --- MigrateMusic ---

func TestMigrateMusic_CreatesOnlyTheMusicDB(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "music.db")
	if err := MigrateMusic(p); err != nil {
		t.Fatalf("MigrateMusic: %v", err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.Contains(e.Name(), "settings") {
			t.Errorf("MigrateMusic created a settings file: %s", e.Name())
		}
	}
	db, err := OpenMusic(p)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Insert(sampleRecord("x")); err != nil {
		t.Errorf("schema not usable after MigrateMusic: %v", err)
	}
}

func TestMigrateMusic_IsIdempotent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "music.db")
	if err := MigrateMusic(p); err != nil {
		t.Fatal(err)
	}
	db, _ := OpenMusic(p)
	db.Insert(sampleRecord("keep"))
	db.Close()

	if err := MigrateMusic(p); err != nil {
		t.Fatalf("second MigrateMusic: %v", err)
	}
	db, _ = OpenMusic(p)
	defer db.Close()
	if rec, _ := db.QueryByID("keep"); rec == nil {
		t.Error("a second MigrateMusic lost data")
	}
}

func TestMigrateMusic_BadPathErrors(t *testing.T) {
	if err := MigrateMusic(filepath.Join(t.TempDir(), "no", "such", "dir", "music.db")); err == nil {
		t.Error("want an error when the directory does not exist")
	}
}

func TestMigrateMusic_RefusesNonSQLiteFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "music.db")
	os.WriteFile(p, []byte("this is not a sqlite database, just text padding to exceed the header size........................"), 0o644)
	if err := MigrateMusic(p); err == nil {
		t.Error("want an error for a corrupt database file")
	}
}

func TestMigrateMusic_MatchesFullMigrate(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.db"), filepath.Join(dir, "b.db")
	if err := MigrateMusic(a); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(filepath.Join(dir, "s.db"), b); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{a, b} {
		db, err := OpenMusic(p)
		if err != nil {
			t.Fatal(err)
		}
		if err := db.Insert(sampleRecord("z")); err != nil {
			t.Errorf("%s: %v", p, err)
		}
		db.Close()
	}
}

func TestAndContains_WildcardsAreLiteral(t *testing.T) {
	db := openTestMusic(t)
	for i, kw := range []string{"100% hits", "a_b song", "axb song", "back\\slash", "plain"} {
		if err := db.Insert(MusicRecord{ID: fmt.Sprint(i), Type: "file", Title: kw, Keywords: kw, Path: kw}); err != nil {
			t.Fatal(err)
		}
	}
	for q, want := range map[string]int{"100%": 1, "a_b": 1, "\\": 1, "SONG": 2, "x": 1} {
		got, err := db.Query((&Condition{}).AndContains("keywords", q, false))
		if err != nil || len(got) != want {
			t.Errorf("AndContains(%q) = %d rows, err %v; want %d", q, len(got), err, want)
		}
	}
}

func TestQueryByTags_WholeTagCaseInsensitive(t *testing.T) {
	db := openTestMusic(t)
	for id, tags := range map[string][]string{"1": {"rock"}, "2": {"hardrock"}, "3": {"Rock", "live"}, "4": {"jazz"}} {
		r := sampleRecord(id)
		r.Tags = tags
		if err := db.Insert(r); err != nil {
			t.Fatal(err)
		}
	}
	got, err := db.QueryByTags([]string{"ROCK"})
	if err != nil || len(got) != 2 {
		t.Fatalf("rock: %d rows, err %v; want 2 (not hardrock)", len(got), err)
	}
	got, _ = db.QueryByTags([]string{"rock", "live"})
	if len(got) != 1 || got[0].ID != "3" {
		t.Errorf("rock+live = %v, want only row 3", got)
	}
	if got, _ := db.QueryByTags(nil); len(got) != 0 {
		t.Errorf("no tags must match nothing, got %d", len(got))
	}
}

func TestUpdateTags(t *testing.T) {
	db := openTestMusic(t)
	r := sampleRecord("a")
	r.Tags = []string{"x"}
	db.Insert(r)

	if err := db.UpdateTags("a", func(tags []string) []string { return append(tags, "y") }); err != nil {
		t.Fatal(err)
	}
	got, _ := db.QueryByID("a")
	if strings.Join(got.Tags, ",") != "x,y" {
		t.Errorf("tags = %v", got.Tags)
	}
	if got.Title != r.Title || got.Path != r.Path {
		t.Errorf("other columns changed: %+v", got)
	}
	if err := db.UpdateTags("a", func([]string) []string { return nil }); err != nil {
		t.Fatal(err)
	}
	if got, _ = db.QueryByID("a"); len(got.Tags) != 0 {
		t.Errorf("tags = %v, want none", got.Tags)
	}
	if err := db.UpdateTags("missing", func(t []string) []string { return t }); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
	if err := db.UpdateTags("info", func(t []string) []string { return t }); !errors.Is(err, ErrNotFound) {
		t.Errorf("info row must not be touchable, err = %v", err)
	}
}

func TestUpsertProbed_KeepsExistingTagsInsertsNewWithTags(t *testing.T) {
	db := openTestMusic(t)
	r := sampleRecord("a")
	r.Tags = []string{"old"}
	db.Insert(r)

	// A tag change lands after the scan read its snapshot...
	db.UpdateTags("a", func([]string) []string { return []string{"fresh"} })
	// ...then the scan writes its (stale) record.
	stale := r
	stale.Title = "New Title"
	if err := db.UpsertProbed(stale); err != nil {
		t.Fatal(err)
	}
	got, _ := db.QueryByID("a")
	if got.Title != "New Title" || strings.Join(got.Tags, ",") != "fresh" {
		t.Errorf("got title %q tags %v; want new title, tag fresh kept", got.Title, got.Tags)
	}

	n := sampleRecord("b")
	n.Tags = []string{"seed"}
	if err := db.UpsertProbed(n); err != nil {
		t.Fatal(err)
	}
	if got, _ = db.QueryByID("b"); got == nil || strings.Join(got.Tags, ",") != "seed" {
		t.Errorf("new row = %+v, want tags seed", got)
	}
}

func TestQueryByTags_NonASCIICaseInsensitive(t *testing.T) {
	db := openTestMusic(t)
	r := sampleRecord("1")
	r.Tags = []string{"Ärger", "ÉTÉ"}
	db.Insert(r)
	for _, q := range []string{"ärger", "été", "ÄRGER"} {
		if got, err := db.QueryByTags([]string{q}); err != nil || len(got) != 1 {
			t.Errorf("QueryByTags(%q) = %d rows, err %v; want 1", q, len(got), err)
		}
	}
	if got, _ := db.QueryByTags([]string{"arger"}); len(got) != 0 {
		t.Error("an unaccented tag must not match an accented one")
	}
}
