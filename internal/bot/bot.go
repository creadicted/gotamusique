package bot

import (
	"context"
	"io"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"

	"github.com/konradk/gotamusique/internal/audio"
	"github.com/konradk/gotamusique/internal/command"
	"github.com/konradk/gotamusique/internal/config"
	"github.com/konradk/gotamusique/internal/queue"
	"github.com/konradk/gotamusique/internal/web"
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
	wakeCh        chan struct{}   // buffered(1): non-blocking wake for the loop goroutine
	launchVersion atomic.Int64    // incremented by Play() to suppress onTrackEnd's queue.Next()
	playEpoch     atomic.Int64    // incremented by every playback control; invalidates in-flight resolves
	resolveCancel func()          // cancels the in-flight Resolver call (guarded by mu)
	fails         failGuard       // distinct entries that failed since the last good track
	pauseState    atomic.Int32    // pauseNone / pauseManual / pauseAuto; see pause.go
	resume        resumePoint     // where a paused local file stopped, for Resume
	nowPlaying    audio.MediaItem // item handed to the pipeline last (guarded by mu)
	last          *lastPlayed     // item remembered from the previous run; consumed by RestoreLast (guarded by mu)

	webAuth *web.Auth // login tokens and sessions; nil unless the web interface uses token auth

	snapMu  sync.Mutex // guards snapSig and snapRev (see Snapshot)
	snapSig snapSig
	snapRev uint64

	// Music library (nil when music_folder is unset or the database failed to open).
	libMu          sync.Mutex
	lib            *musicLibrary
	autoplayActive atomic.Bool     // autoplay is running (cleared when it stops itself)
	autoFails      atomic.Int64    // consecutive failed tracks in autoplay with none played between
	life           context.Context // cancelled by Shutdown; stops in-flight scans
	lifeCancel     context.CancelFunc
	cancel         func() // cancels the top-level Run context (set by main)
	dispatcher     *command.Dispatcher
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

	life, lifeCancel := context.WithCancel(context.Background())
	b := &Bot{
		life:       life,
		lifeCancel: lifeCancel,
		cfg:        cfg,
		log:        log,
		queue:      queue.NewQueue(),
		wakeCh:     make(chan struct{}, 1),
	}
	if cfg.Web.Enabled && cfg.Web.AuthMethod == config.AuthToken {
		b.webAuth = web.NewAuth(web.AuthOptions{})
	}
	b.last = loadLastPlayed(cfg.Bot.LastPlayedPath, log)
	b.lib = openLibrary(b)
	b.queue.SetMode(startupMode(cfg, log, b.lib != nil))
	b.autoplayActive.Store(b.queue.Mode() == queue.ModeAutoplay)
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
	b.saveOnShutdown()
	b.lifeCancel()
	defer b.closeLibrary()
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.audio != nil {
		b.audio.Interrupt()
	}

	if b.client != nil && b.client.State() != gumble.StateDisconnected {
		b.client.Disconnect() //nolint:errcheck
	}
}

// startupMode parses [bot] playback_mode. An invalid value, or autoplay without
// a music library, logs a warning and falls back to one-shot so the bot still
// starts, e.g. with a config copied from botamusique.
func startupMode(cfg *config.Config, log *slog.Logger, haveLibrary bool) queue.Mode {
	m, err := queue.ParseMode(cfg.Bot.PlaybackMode)
	if err != nil {
		log.Warn("invalid playback_mode in config, using one-shot",
			slog.String("value", cfg.Bot.PlaybackMode), slog.String("err", err.Error()))
		return queue.ModeOneShot
	}
	if m == queue.ModeAutoplay && !haveLibrary {
		log.Warn("playback_mode = autoplay needs a music library (set [files] music_folder); using one-shot")
		return queue.ModeOneShot
	}
	return m
}
