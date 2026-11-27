package database

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLazySettings_NoFileUntilFirstWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "settings.db")
	l := NewLazySettings(path)
	defer l.Close()

	if got := l.GetOrDefault("bot", "duck", "fallback"); got != "fallback" {
		t.Fatalf("read before any write = %q, want fallback", got)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("reading created the file (stat err = %v)", err)
	}

	if err := l.Set("bot", "duck", true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("first write did not create the file: %v", err)
	}
	if got := l.GetOrDefault("bot", "duck", "x"); got != "1" {
		t.Fatalf("read back = %q, want 1", got)
	}
}

func TestLazySettings_SurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.db")
	l := NewLazySettings(path)
	if err := l.Set("bot", "duck_threshold", 1234); err != nil {
		t.Fatal(err)
	}
	l.Close()

	l2 := NewLazySettings(path)
	defer l2.Close()
	if got := l2.GetOrDefault("bot", "duck_threshold", "4000"); got != "1234" {
		t.Fatalf("after reopen = %q, want 1234", got)
	}
	if got := l2.GetOrDefault("bot", "missing", "dflt"); got != "dflt" {
		t.Fatalf("missing key = %q, want dflt", got)
	}
}

func TestLazySettings_EmptyPathDisablesPersistence(t *testing.T) {
	l := NewLazySettings("")
	if l.Enabled() {
		t.Fatal("empty path must be disabled")
	}
	if err := l.Set("bot", "k", "v"); err != nil {
		t.Fatalf("Set on disabled store = %v, want nil", err)
	}
	if got := l.GetOrDefault("bot", "k", "dflt"); got != "dflt" {
		t.Fatalf("disabled read = %q, want dflt", got)
	}
}

func TestLazySettings_NilIsDisabled(t *testing.T) {
	var l *LazySettings
	if l.Enabled() || l.Set("a", "b", 1) != nil || l.GetOrDefault("a", "b", "d") != "d" || l.Close() != nil {
		t.Fatal("nil LazySettings must behave as disabled")
	}
}

func TestLazySettings_UnreadableFileFallsBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.db")
	if err := os.WriteFile(path, []byte("this is not a sqlite file at all"), 0o644); err != nil {
		t.Fatal(err)
	}
	l := NewLazySettings(path)
	defer l.Close()
	if got := l.GetOrDefault("bot", "k", "dflt"); got != "dflt" {
		t.Fatalf("corrupt file read = %q, want dflt", got)
	}
}

func TestLazySettings_WriteAfterCloseFails(t *testing.T) {
	l := NewLazySettings(filepath.Join(t.TempDir(), "settings.db"))
	l.Close()
	if err := l.Set("bot", "k", "v"); err == nil {
		t.Fatal("Set after Close must fail")
	}
}
