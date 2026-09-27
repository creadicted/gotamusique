package database

import (
	"database/sql"
	"errors"
	"fmt"
	"log"

	_ "modernc.org/sqlite"
)

// SettingsDB persists runtime state that survives bot restarts.
type SettingsDB interface {
	Get(section, option string) (string, error)
	GetOrDefault(section, option, fallback string) string
	Set(section, option string, value any) error
	Has(section, option string) bool
	Remove(section, option string) error
	Items(section string) ([][2]string, error)
	Close() error
}

// ErrNotFound is returned by Get when the key does not exist.
var ErrNotFound = errors.New("database: item not found")

type settingsDB struct {
	db *sql.DB
}

// OpenSettings opens the settings database at path and ensures the table exists.
// Migration must be run separately before calling this.
func OpenSettings(path string) (SettingsDB, error) {
	db, err := openSQLite(path)
	if err != nil {
		return nil, err
	}
	return &settingsDB{db: db}, nil
}

func (s *settingsDB) Get(section, option string) (string, error) {
	var value string
	err := s.db.QueryRow(
		"SELECT value FROM botamusique WHERE section=? AND option=?",
		section, option,
	).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return value, err
}

func (s *settingsDB) GetOrDefault(section, option, fallback string) string {
	v, err := s.Get(section, option)
	if err != nil {
		if !errors.Is(err, ErrNotFound) {
			log.Printf("database: GetOrDefault(%q,%q): %v", section, option, err)
		}
		return fallback
	}
	return v
}

func (s *settingsDB) Set(section, option string, value any) error {
	text := anyToText(value)
	_, err := s.db.Exec(
		"INSERT OR REPLACE INTO botamusique (section, option, value) VALUES (?, ?, ?)",
		section, option, text,
	)
	return err
}

func (s *settingsDB) Has(section, option string) bool {
	var dummy int
	err := s.db.QueryRow(
		"SELECT 1 FROM botamusique WHERE section=? AND option=?",
		section, option,
	).Scan(&dummy)
	return err == nil
}

func (s *settingsDB) Remove(section, option string) error {
	_, err := s.db.Exec(
		"DELETE FROM botamusique WHERE section=? AND option=?",
		section, option,
	)
	return err
}

func (s *settingsDB) Items(section string) ([][2]string, error) {
	rows, err := s.db.Query(
		"SELECT option, value FROM botamusique WHERE section=?",
		section,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out [][2]string
	for rows.Next() {
		var opt, val string
		if err := rows.Scan(&opt, &val); err != nil {
			return nil, err
		}
		out = append(out, [2]string{opt, val})
	}
	return out, rows.Err()
}

func (s *settingsDB) Close() error { return s.db.Close() }

// anyToText serialises value to a TEXT string for SQLite storage.
// Booleans use "1"/"0" to match Python's getboolean convention.
func anyToText(value any) string {
	switch v := value.(type) {
	case bool:
		if v {
			return "1"
		}
		return "0"
	case string:
		return v
	default:
		return fmt.Sprintf("%v", v)
	}
}

// openSQLite opens a SQLite file with WAL mode and a single connection.
func openSQLite(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
