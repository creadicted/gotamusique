package bot

import (
	"io"
	"log/slog"
	"os"
	"sync"

	"github.com/konradk/gotamusique/internal/config"
	"layeh.com/gumble/gumble"
)

// Bot holds the gumble client and all shared state for the bot's lifetime.
type Bot struct {
	cfg    *config.Config
	client *gumble.Client
	queue  interface{} // placeholder until 1-06 (queue.Queue)
	audio  interface{} // placeholder until 1-04 (audio.Pipeline)
	log    *slog.Logger
	mu     sync.Mutex
	cancel func()
}

// New creates a Bot from cfg. The logger writes to stderr unless cfg.Bot.Logfile
// is set, in which case it writes to that file. Debug log level is enabled when
// cfg.Debug.MumbleConnection is true.
func New(cfg *config.Config) (*Bot, error) {
	level := slog.LevelInfo
	if cfg.Debug.MumbleConnection {
		level = slog.LevelDebug
	}

	var w io.Writer = os.Stderr
	if cfg.Bot.Logfile != "" {
		f, err := os.OpenFile(cfg.Bot.Logfile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return nil, err
		}
		w = f
	}

	log := slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level}))

	return &Bot{
		cfg: cfg,
		log: log,
	}, nil
}

// Shutdown stops the audio pipeline (if running) and disconnects from Mumble.
// Safe to call from any goroutine.
func (b *Bot) Shutdown() {
	b.mu.Lock()
	defer b.mu.Unlock()

	// TODO(1-04): stop audio pipeline when audio.Pipeline is wired
	// if b.audio != nil { b.audio.Stop() }

	if b.client != nil && b.client.State() != gumble.StateDisconnected {
		b.client.Disconnect() //nolint:errcheck
	}
}
