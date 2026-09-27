package database

import (
	"path/filepath"
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
