package bot

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/konradk/gotamusique/internal/config"
	"log/slog"
)

func defaultCfg() *config.Config {
	return &config.Config{
		Server: config.ServerConfig{
			Host:          "127.0.0.1",
			Port:          64738,
			TLSSkipVerify: true,
		},
		Bot: config.BotConfig{
			Username: "testbot",
			Volume:   0.8,
		},
		Commands: config.CommandsConfig{Symbol: []string{"!"}},
		Radio:    map[string]config.RadioPreset{},
	}
}

func TestNew_DefaultConfig(t *testing.T) {
	b, err := New(defaultCfg())
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if b == nil {
		t.Fatal("New returned nil bot")
	}
	if b.log == nil {
		t.Fatal("bot.log is nil")
	}
}

func TestNew_DebugLoggingEnabled(t *testing.T) {
	cfg := defaultCfg()
	cfg.Debug.MumbleConnection = true

	b, err := New(cfg)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	if !b.log.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("expected debug logging to be enabled when MumbleConnection=true")
	}
}

func TestNew_DebugLoggingDisabled(t *testing.T) {
	b, err := New(defaultCfg())
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	if b.log.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("expected debug logging to be disabled by default")
	}
}

func TestNew_Logfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bot.log")
	cfg := defaultCfg()
	cfg.Bot.Logfile = path

	b, err := New(cfg)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if b == nil {
		t.Fatal("New returned nil bot")
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("logfile not created: %v", err)
	}
}

func TestNew_LogfileError(t *testing.T) {
	cfg := defaultCfg()
	cfg.Bot.Logfile = "/dev/null/impossible/path/bot.log"

	_, err := New(cfg)
	if err == nil {
		t.Fatal("expected error for invalid logfile path, got nil")
	}
}

func TestShutdown_NilClient(t *testing.T) {
	b, err := New(defaultCfg())
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	// client is nil — Shutdown must not panic
	b.Shutdown()
}

func TestShutdown_Idempotent(t *testing.T) {
	b, err := New(defaultCfg())
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	// calling Shutdown multiple times must not panic
	b.Shutdown()
	b.Shutdown()
}

func TestSetAvatar_EmptyPath(t *testing.T) {
	b, err := New(defaultCfg())
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	// empty avatar path — must return early without touching the nil client
	b.setAvatar()
}

func TestSetAvatar_MissingFile(t *testing.T) {
	cfg := defaultCfg()
	cfg.Bot.Avatar = "/nonexistent/avatar.png"

	b, err := New(cfg)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	// missing file — must log a warning and return without touching the nil client
	b.setAvatar()
}

func TestSetComment_Empty(t *testing.T) {
	b, err := New(defaultCfg())
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	// empty comment — must return early without touching the nil client
	b.setComment()
}

func TestJoinChannel_Empty(t *testing.T) {
	b, err := New(defaultCfg())
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	// empty channel — must return early without touching the nil client
	b.joinChannel()
}
