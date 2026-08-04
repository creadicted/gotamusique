package database

import (
	"errors"
	"path/filepath"
	"testing"
)

func openTestSettings(t *testing.T) SettingsDB {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.db")
	if err := Migrate(path, filepath.Join(dir, "music.db")); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	db, err := OpenSettings(path)
	if err != nil {
		t.Fatalf("OpenSettings: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestSettings_SetGet_RoundTrip(t *testing.T) {
	db := openTestSettings(t)

	if err := db.Set("audio", "volume", "80"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := db.Get("audio", "volume")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "80" {
		t.Errorf("got %q, want %q", got, "80")
	}
}

func TestSettings_Get_NotFound(t *testing.T) {
	db := openTestSettings(t)
	_, err := db.Get("missing", "key")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestSettings_GetOrDefault(t *testing.T) {
	db := openTestSettings(t)
	got := db.GetOrDefault("missing", "key", "default")
	if got != "default" {
		t.Errorf("got %q, want %q", got, "default")
	}
}

func TestSettings_SetBool(t *testing.T) {
	db := openTestSettings(t)

	if err := db.Set("duck", "enabled", true); err != nil {
		t.Fatalf("Set true: %v", err)
	}
	got, _ := db.Get("duck", "enabled")
	if got != "1" {
		t.Errorf("bool true: got %q, want %q", got, "1")
	}

	if err := db.Set("duck", "enabled", false); err != nil {
		t.Fatalf("Set false: %v", err)
	}
	got, _ = db.Get("duck", "enabled")
	if got != "0" {
		t.Errorf("bool false: got %q, want %q", got, "0")
	}
}

func TestSettings_Has(t *testing.T) {
	db := openTestSettings(t)
	if db.Has("x", "y") {
		t.Error("Has: expected false for missing key")
	}
	_ = db.Set("x", "y", "z")
	if !db.Has("x", "y") {
		t.Error("Has: expected true after Set")
	}
}

func TestSettings_Remove(t *testing.T) {
	db := openTestSettings(t)
	_ = db.Set("s", "o", "v")
	if err := db.Remove("s", "o"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if db.Has("s", "o") {
		t.Error("key still present after Remove")
	}
}

func TestSettings_Items(t *testing.T) {
	db := openTestSettings(t)
	_ = db.Set("sec", "a", "1")
	_ = db.Set("sec", "b", "2")
	_ = db.Set("other", "c", "3")

	items, err := db.Items("sec")
	if err != nil {
		t.Fatalf("Items: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("got %d items, want 2", len(items))
	}
}

func TestSettings_Upsert(t *testing.T) {
	db := openTestSettings(t)
	_ = db.Set("s", "o", "first")
	_ = db.Set("s", "o", "second")
	got, _ := db.Get("s", "o")
	if got != "second" {
		t.Errorf("upsert: got %q, want %q", got, "second")
	}
}
