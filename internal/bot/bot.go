package bot

import (
	"io"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"

	"github.com/konradk/gotamusique/internal/audio"
	"github.com/konradk/gotamusique/internal/command"
	"github.com/konradk/gotamusique/internal/config"
	"github.com/konradk/gotamusique/internal/queue"
	"layeh.com/gumble/gumble"
)

// Bot holds the gumble client and all shared state for the bot's lifetime.
type Bot struct {
	cfg *config.Config
	log *slog.Logger

	mu         sync.Mutex
	client     *gumble.Client
	audio      *audio.Pipeline
	cancelLoop func() // cancels the per-connection loop goroutine
	isMuted    bool
	prevVol    float64 // volume saved before Mute()

	queue         *queue.Queue
	wakeCh        chan struct{} // buffered(1): non-blocking wake for the loop goroutine
	launchVersion atomic.Int64  // incremented by Play() to suppress onTrackEnd's queue.Next()
	cancel        func()        // cancels the top-level Run context (set by main)
	dispatcher    *command.Dispatcher
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

	b := &Bot{
		cfg:    cfg,
		log:    log,
		queue:  queue.NewQueue(),
		wakeCh: make(chan struct{}, 1),
	}
	d := command.NewDispatcher(cfg.Commands.Symbol)
	command.RegisterAll(b, d)
	b.dispatcher = d
	return b, nil
}

// Queue returns the bot's play queue.
func (b *Bot) Queue() *queue.Queue { return b.queue }

// Shutdown stops the audio pipeline (if running) and disconnects from Mumble.
// Safe to call from any goroutine.
func (b *Bot) Shutdown() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.audio != nil {
		b.audio.Interrupt()
	}

	if b.client != nil && b.client.State() != gumble.StateDisconnected {
		b.client.Disconnect() //nolint:errcheck
	}
}
