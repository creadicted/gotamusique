package database

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// buildSettingsV0 creates a version-0 settings DB (no db_version row, old schema).
func buildSettingsV0(t *testing.T, path string) {
	t.Helper()
	db, err := openSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE botamusique (section TEXT, option TEXT, value TEXT, UNIQUE(section, option))`)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = db.Exec(`INSERT INTO botamusique VALUES ('audio','volume','75')`)
}

// buildSettingsV1 creates a version-1 settings DB (db_version=1, contains inline music table).
func buildSettingsV1(t *testing.T, path string) {
	t.Helper()
	db, err := openSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE botamusique (section TEXT, option TEXT, value TEXT, UNIQUE(section, option))`)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = db.Exec(`INSERT INTO botamusique VALUES ('bot','db_version','1')`)
	_, _ = db.Exec(`INSERT INTO botamusique VALUES ('audio','volume','60')`)
}

// buildMusicV0 creates a version-0 music DB (no keywords/path columns, no version sentinel).
func buildMusicV0(t *testing.T, path string) {
	t.Helper()
	db, err := openSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE music (id TEXT PRIMARY KEY, type TEXT, title TEXT, metadata TEXT, tags TEXT, create_at DATETIME DEFAULT CURRENT_TIMESTAMP)`)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = db.Exec(`INSERT INTO music (id, type, title, metadata, tags) VALUES ('song1','file','Old Song','{}','rock,')`)
}

// buildMusicV1 creates a version-1 music DB (has keywords/path, no duration in metadata).
func buildMusicV1(t *testing.T, path string) {
	t.Helper()
	db, err := openSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE music (
		id TEXT PRIMARY KEY, type TEXT, title TEXT, keywords TEXT,
		metadata TEXT, tags TEXT, path TEXT,
		create_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = db.Exec(`INSERT INTO music (id, title) VALUES ('info','1')`)
	_, _ = db.Exec(`INSERT INTO music (id, type, title, keywords, metadata, tags, path) VALUES ('song2','file','V1 Song','v1 song','{"artist":"Band"}','jazz,','/music/song2.mp3')`)
}

// buildMusicV2 creates a version-2 music DB (missing duration in URL entries).
func buildMusicV2(t *testing.T, path string) {
	t.Helper()
	db, err := openSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE music (
		id TEXT PRIMARY KEY, type TEXT, title TEXT, keywords TEXT,
		metadata TEXT, tags TEXT, path TEXT,
		create_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = db.Exec(`INSERT INTO music (id, title) VALUES ('info','2')`)
	// URL track with duration in minutes (needs * 60 in v4)
	_, _ = db.Exec(`INSERT INTO music (id, type, title, keywords, metadata, tags, path) VALUES ('radio1','url','Radio','radio','{"duration":3}','radio,','')`)
}

func TestMigration_SettingsV0(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.db")
	musicPath := filepath.Join(dir, "music.db")
	buildSettingsV0(t, settingsPath)

	if err := Migrate(settingsPath, musicPath); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	db, _ := openSQLite(settingsPath)
	defer db.Close()
	var ver string
	_ = db.QueryRow("SELECT value FROM botamusique WHERE section='bot' AND option='db_version'").Scan(&ver)
	if ver != "2" {
		t.Errorf("settings version: got %q, want %q", ver, "2")
	}
}

func TestMigration_SettingsV1(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.db")
	musicPath := filepath.Join(dir, "music.db")
	buildSettingsV1(t, settingsPath)

	if err := Migrate(settingsPath, musicPath); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	sdb, _ := OpenSettings(settingsPath)
	defer sdb.Close()
	// Volume should survive the v1→v2 migration (no data loss)
	vol, err := sdb.Get("audio", "volume")
	if err != nil {
		t.Fatalf("Get volume after v1 migration: %v", err)
	}
	if vol != "60" {
		t.Errorf("volume: got %q, want %q", vol, "60")
	}
}

func TestMigration_MusicV0(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.db")
	musicPath := filepath.Join(dir, "music.db")
	buildMusicV0(t, musicPath)

	if err := Migrate(settingsPath, musicPath); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	mdb, _ := OpenMusic(musicPath)
	defer mdb.Close()
	rec, err := mdb.QueryByID("song1")
	if err != nil {
		t.Fatalf("QueryByID: %v", err)
	}
	if rec == nil {
		t.Fatal("song1 missing after v0 migration")
	}
	// keywords should have been populated from title
	if rec.Keywords == "" {
		t.Error("keywords empty after v0→v4 migration")
	}
}

func TestMigration_MusicV1(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.db")
	musicPath := filepath.Join(dir, "music.db")
	buildMusicV1(t, musicPath)

	if err := Migrate(settingsPath, musicPath); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	mdb, _ := OpenMusic(musicPath)
	defer mdb.Close()
	rec, err := mdb.QueryByID("song2")
	if err != nil || rec == nil {
		t.Fatalf("song2 missing after v1→v4 migration: %v", err)
	}
	// duration field should have been added
	if _, ok := rec.Metadata["duration"]; !ok {
		t.Error("duration missing in metadata after v1→v4 migration")
	}
}

func TestMigration_MusicV2_DurationConversion(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.db")
	musicPath := filepath.Join(dir, "music.db")
	buildMusicV2(t, musicPath)

	if err := Migrate(settingsPath, musicPath); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	mdb, _ := OpenMusic(musicPath)
	defer mdb.Close()
	rec, err := mdb.QueryByID("radio1")
	if err != nil || rec == nil {
		t.Fatalf("radio1 missing: %v", err)
	}
	dur, _ := rec.Metadata["duration"].(float64)
	if dur != 180 { // 3 minutes * 60
		t.Errorf("duration: got %v, want 180", dur)
	}
}

func TestMigration_FreshDB(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.db")
	musicPath := filepath.Join(dir, "music.db")

	if err := Migrate(settingsPath, musicPath); err != nil {
		t.Fatalf("Migrate on fresh files: %v", err)
	}

	// Settings version row should exist
	sdb, _ := openSQLite(settingsPath)
	defer sdb.Close()
	var ver string
	_ = sdb.QueryRow("SELECT value FROM botamusique WHERE section='bot' AND option='db_version'").Scan(&ver)
	if ver != "2" {
		t.Errorf("settings version: got %q, want %q", ver, "2")
	}

	// Music version sentinel should exist
	mdb, _ := openSQLite(musicPath)
	defer mdb.Close()
	var mver sql.NullString
	_ = mdb.QueryRow("SELECT title FROM music WHERE id='info'").Scan(&mver)
	if !mver.Valid || mver.String != "4" {
		t.Errorf("music version: got %q, want %q", mver.String, "4")
	}
}

func TestMigration_Idempotent(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.db")
	musicPath := filepath.Join(dir, "music.db")

	// Run twice — second call should be a no-op.
	if err := Migrate(settingsPath, musicPath); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	if err := Migrate(settingsPath, musicPath); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
}
