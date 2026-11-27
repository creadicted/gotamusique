package database

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
)

// LazySettings is a settings database that does not exist until something is
// saved. Reads of a missing file return the fallback without creating it, so a
// bot that never changes a persisted setting never leaves a settings file behind.
// Safe for concurrent use.
type LazySettings struct {
	path string

	mu     sync.Mutex
	db     SettingsDB
	closed bool
}

// NewLazySettings returns a handle on the settings database at path. An empty
// path turns persistence off: reads return the fallback and writes do nothing.
func NewLazySettings(path string) *LazySettings { return &LazySettings{path: path} }

// Enabled reports whether settings are persisted at all.
func (l *LazySettings) Enabled() bool { return l != nil && l.path != "" }

// GetOrDefault returns the stored value, or fallback when persistence is off,
// the file does not exist yet, or the key is not set.
func (l *LazySettings) GetOrDefault(section, option, fallback string) string {
	if !l.Enabled() {
		return fallback
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.db == nil {
		if _, err := os.Stat(l.path); err != nil {
			return fallback // nothing saved yet; do not create the file
		}
		if err := l.openLocked(); err != nil {
			log.Printf("database: settings unreadable, using defaults: %v", err)
			return fallback
		}
	}
	return l.db.GetOrDefault(section, option, fallback)
}

// Set stores value, creating and migrating the database on the first write.
// It is a no-op when persistence is off.
func (l *LazySettings) Set(section, option string, value any) error {
	if !l.Enabled() {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.db == nil {
		if err := l.openLocked(); err != nil {
			return err
		}
	}
	return l.db.Set(section, option, value)
}

// Close releases the database if it was opened. Later calls reopen nothing:
// reads return the fallback and writes fail.
func (l *LazySettings) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closed = true
	if l.db == nil {
		return nil
	}
	err := l.db.Close()
	l.db = nil
	return err
}

func (l *LazySettings) openLocked() error {
	if l.closed {
		return errors.New("database: settings closed")
	}
	if dir := filepath.Dir(l.path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("database: settings directory: %w", err)
		}
	}
	if err := MigrateSettings(l.path); err != nil {
		return err
	}
	db, err := OpenSettings(l.path)
	if err != nil {
		return err
	}
	l.db = db
	return nil
}
