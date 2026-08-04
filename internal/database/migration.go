package database

import (
	"database/sql"
	"fmt"
	"log"

	_ "modernc.org/sqlite"
)

const settingsDBVersion = 2
const musicDBVersion = 4

// Migrate opens both database files, detects their current schema version, and
// upgrades them to the current version. It must be called before OpenSettings
// or OpenMusic. Both paths are created if they do not yet exist.
func Migrate(settingsPath, musicPath string) error {
	sdb, err := openSQLite(settingsPath)
	if err != nil {
		return fmt.Errorf("migration: open settings db: %w", err)
	}
	defer sdb.Close()

	mdb, err := openSQLite(musicPath)
	if err != nil {
		return fmt.Errorf("migration: open music db: %w", err)
	}
	defer mdb.Close()

	if err := migrateSettings(sdb); err != nil {
		return fmt.Errorf("migration: settings: %w", err)
	}
	if err := migrateMusic(mdb); err != nil {
		return fmt.Errorf("migration: music: %w", err)
	}
	return nil
}

// --- settings migration ---

func migrateSettings(db *sql.DB) error {
	if !hasTable(db, "botamusique") {
		log.Printf("database: no settings table; creating version %d", settingsDBVersion)
		return createSettingsV2(db)
	}

	var current int
	row := db.QueryRow("SELECT value FROM botamusique WHERE section='bot' AND option='db_version'")
	var rawVer string
	if err := row.Scan(&rawVer); err == nil {
		fmt.Sscanf(rawVer, "%d", &current)
	}

	if current == settingsDBVersion {
		return nil
	}

	log.Printf("database: migrating settings v%d → v%d", current, settingsDBVersion)
	for current < settingsDBVersion {
		next, err := settingsMigrateStep(db, current)
		if err != nil {
			return fmt.Errorf("step %d: %w", current, err)
		}
		current = next
	}
	_, err := db.Exec("UPDATE botamusique SET value=? WHERE section='bot' AND option='db_version'", settingsDBVersion)
	return err
}

func settingsMigrateStep(db *sql.DB, from int) (int, error) {
	switch from {
	case 0:
		// v0→v2: drop and recreate (no data worth preserving in v0)
		if _, err := db.Exec("DROP TABLE botamusique"); err != nil {
			return 0, err
		}
		if err := createSettingsV2(db); err != nil {
			return 0, err
		}
		return 2, nil
	case 1:
		// v1→v2: music table was split into a separate file; nothing to do
		// on the settings side since we always pass a separate music path.
		_, err := db.Exec("UPDATE botamusique SET value=2 WHERE section='bot' AND option='db_version'")
		if err != nil {
			return 0, err
		}
		return 2, nil
	default:
		return 0, fmt.Errorf("unknown settings version %d", from)
	}
}

func createSettingsV2(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS botamusique (
		section TEXT,
		option  TEXT,
		value   TEXT,
		UNIQUE(section, option)
	)`)
	if err != nil {
		return err
	}
	_, err = db.Exec("INSERT INTO botamusique (section, option, value) VALUES ('bot', 'db_version', ?)", settingsDBVersion)
	return err
}

func createMusicV1(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE music (
		id        TEXT PRIMARY KEY,
		type      TEXT,
		title     TEXT,
		keywords  TEXT,
		metadata  TEXT,
		tags      TEXT,
		path      TEXT,
		create_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		return err
	}
	_, err = db.Exec("INSERT INTO music (id, title) VALUES ('info', ?)", musicDBVersion)
	return err
}

// --- music migration ---

func migrateMusic(db *sql.DB) error {
	if !hasTable(db, "music") {
		log.Printf("database: no music table; creating version %d", musicDBVersion)
		return createMusicV1(db)
	}

	var current int
	var rawVer sql.NullString
	_ = db.QueryRow("SELECT title FROM music WHERE id='info'").Scan(&rawVer)
	if rawVer.Valid {
		fmt.Sscanf(rawVer.String, "%d", &current)
	}

	if current == musicDBVersion {
		return nil
	}

	log.Printf("database: migrating music v%d → v%d", current, musicDBVersion)
	for current < musicDBVersion {
		next, err := musicMigrateStep(db, current)
		if err != nil {
			return fmt.Errorf("step %d: %w", current, err)
		}
		current = next
	}
	_, err := db.Exec("UPDATE music SET title=? WHERE id='info'", musicDBVersion)
	return err
}

func musicMigrateStep(db *sql.DB, from int) (int, error) {
	switch from {
	case 0:
		return musicMigrate0to1(db)
	case 1:
		return musicMigrate1to2(db)
	case 2, 3:
		return musicMigrate2to4(db)
	default:
		return 0, fmt.Errorf("unknown music version %d", from)
	}
}

// v0→v1: add keywords and path columns via rename-and-recreate.
func musicMigrate0to1(db *sql.DB) (int, error) {
	if _, err := db.Exec("ALTER TABLE music RENAME TO music_old"); err != nil {
		return 0, err
	}
	if err := createMusicV1(db); err != nil {
		return 0, err
	}
	if _, err := db.Exec("INSERT INTO music (id, type, title, metadata, tags) SELECT id, type, title, metadata, tags FROM music_old"); err != nil {
		return 0, err
	}
	if _, err := db.Exec("DROP TABLE music_old"); err != nil {
		return 0, err
	}
	return 1, nil
}

// v1→v2: populate keywords from title+artist in metadata.
func musicMigrate1to2(db *sql.DB) (int, error) {
	mdb := &musicDB{db: db}
	records, err := mdb.Query(nil)
	if err != nil {
		return 0, err
	}
	for _, rec := range records {
		kw := rec.Title
		if artist, ok := rec.Metadata["artist"].(string); ok && artist != "" {
			kw += " " + artist
		}
		rec.Keywords = kw
		// Strip empty tags.
		var clean []string
		for _, t := range rec.Tags {
			if t != "" {
				clean = append(clean, t)
			}
		}
		rec.Tags = clean
		if err := mdb.Insert(rec); err != nil {
			return 0, err
		}
	}
	return 2, nil
}

// v2/v3→v4: normalise duration (URLs were stored in minutes, need seconds).
func musicMigrate2to4(db *sql.DB) (int, error) {
	mdb := &musicDB{db: db}
	records, err := mdb.Query(nil)
	if err != nil {
		return 0, err
	}
	for _, rec := range records {
		if _, ok := rec.Metadata["duration"]; !ok {
			rec.Metadata["duration"] = float64(0)
		}
		if rec.Type == "url" || rec.Type == "url_from_playlist" {
			if d, ok := rec.Metadata["duration"].(float64); ok {
				rec.Metadata["duration"] = d * 60
			}
		}
		if err := mdb.Insert(rec); err != nil {
			return 0, err
		}
	}
	return 4, nil
}

// --- helpers ---

func hasTable(db *sql.DB, name string) bool {
	var found string
	err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", name).Scan(&found)
	return err == nil
}
