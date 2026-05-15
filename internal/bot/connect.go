package bot

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"
	"time"

	"github.com/konradk/gotamusique/internal/audio"
	"github.com/konradk/gotamusique/internal/radio"
	"layeh.com/gumble/gumble"
	"layeh.com/gumble/gumbleutil"
)

// perConnContext creates a fresh cancellable context for one connection's loop
// goroutine and stores the cancel function on the bot.
func (b *Bot) startLoop() {
	ctx, cancel := context.WithCancel(context.Background())
	b.mu.Lock()
	b.cancelLoop = cancel
	b.mu.Unlock()
	go b.loop(ctx)
}

func (b *Bot) stopLoop() {
	b.mu.Lock()
	cancel := b.cancelLoop
	b.cancelLoop = nil
	b.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

const (
	backoffInitial = 5 * time.Second
	backoffMax     = 60 * time.Second
	maxRetries     = 10
	dialTimeout    = 10 * time.Second
)

// Run connects to the Mumble server and blocks until ctx is cancelled or the
// bot exhausts maxRetries consecutive failed reconnect attempts.
//
// The first connection failure returns immediately with an error (fail-fast).
// Subsequent disconnects trigger the exponential-backoff reconnect loop.
func (b *Bot) Run(ctx context.Context) error {
	b.cancel = func() {} // replaced by context cancel in main

	disconnected := make(chan struct{}, 1)

	gumbleCfg := b.buildGumbleConfig(disconnected)

	if err := b.connect(gumbleCfg); err != nil {
		return fmt.Errorf("initial connection failed: %w", err)
	}

	failures := 0
	backoff := backoffInitial

	for {
		select {
		case <-ctx.Done():
			b.Shutdown()
			return nil
		case <-disconnected:
			if failures >= maxRetries {
				return fmt.Errorf("gave up after %d consecutive reconnect failures", maxRetries)
			}

			b.log.Info("disconnected, reconnecting", slog.Duration("backoff", backoff))

			select {
			case <-ctx.Done():
				return nil
			case <-time.After(backoff):
			}

			if err := b.connect(gumbleCfg); err != nil {
				failures++
				b.log.Error("reconnect failed", slog.Int("attempt", failures), slog.String("err", err.Error()))
				backoff = min(backoff*2, backoffMax)
			} else {
				failures = 0
				backoff = backoffInitial
			}
		}
	}
}

// buildGumbleConfig constructs the gumble.Config with event handlers attached.
func (b *Bot) buildGumbleConfig(disconnected chan<- struct{}) *gumble.Config {
	cfg := gumble.NewConfig()
	cfg.Username = b.cfg.Bot.Username
	cfg.Password = b.cfg.Server.Password
	cfg.Tokens = b.cfg.Server.Tokens

	cfg.Attach(gumbleutil.Listener{
		Connect: func(e *gumble.ConnectEvent) {
			// The Connect event fires inside gumble's read goroutine before
			// DialWithDialer returns, so b.client must be set here first.
			b.mu.Lock()
			b.client = e.Client
			b.audio = audio.New(b.client, b.cfg, b.log)
			b.mu.Unlock()
			b.log.Debug("connected to server")
			b.joinChannel()
			b.setComment()
			b.setAvatar()
			b.startLoop()
			// DEBUG: auto-start the first configured radio preset on connect — uncomment to skip manual !radio calls during testing.
			if preset, ok := b.cfg.Radio["jazz"]; ok {
				b.Enqueue(radio.NewRadioItemFromPreset("jazz", preset))
			}
		},
		Disconnect: func(e *gumble.DisconnectEvent) {
			b.log.Debug("disconnected from server", slog.Int("type", int(e.Type)))
			b.stopLoop()
			// non-blocking send: if Run is shutting down, the channel may be full
			select {
			case disconnected <- struct{}{}:
			default:
			}
		},
		TextMessage: func(e *gumble.TextMessageEvent) {
			b.dispatcher.Dispatch(b, &e.TextMessage)
		},
	})

	return cfg
}

// describeConnErr wraps low-level network errors with an actionable hint.
// The original error is preserved via %w so errors.Is/As still work.
func describeConnErr(err error) error {
	s := err.Error()
	var hint string
	switch {
	case strings.Contains(s, "connection reset"):
		hint = "server reset the connection (wrong password, IP blocked, or TLS mismatch)"
	case strings.Contains(s, "connection refused"):
		hint = "connection refused — check host and port"
	case strings.Contains(s, "no such host"), strings.Contains(s, "name resolution"):
		hint = "hostname not found — check server address"
	case strings.Contains(s, "timeout"), strings.Contains(s, "timed out"):
		hint = "timed out — server may be unreachable"
	case strings.Contains(s, "certificate"):
		hint = "TLS certificate error — try setting tls_skip_verify = True in [server]"
	default:
		return err
	}
	return fmt.Errorf("%s: %w", hint, err)
}

// connect performs a single connection attempt and stores the client on success.
func (b *Bot) connect(cfg *gumble.Config) error {
	tlsCfg := &tls.Config{
		InsecureSkipVerify: b.cfg.Server.TLSSkipVerify, //nolint:gosec
		// Many Mumble servers (murmur built against older OpenSSL/Qt) RST
		// connections when they receive a TLS 1.3 ClientHello instead of
		// negotiating down to 1.2. Cap at 1.2 for compatibility.
		MaxVersion: tls.VersionTLS13,
	}

	if b.cfg.Server.Certificate != "" {
		cert, err := tls.LoadX509KeyPair(b.cfg.Server.Certificate, b.cfg.Server.Certificate)
		if err != nil {
			return fmt.Errorf("loading certificate: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}

	addr := net.JoinHostPort(b.cfg.Server.Host, fmt.Sprintf("%d", b.cfg.Server.Port))
	b.log.Debug("connecting", slog.String("addr", addr))

	client, err := gumble.DialWithDialer(&net.Dialer{Timeout: dialTimeout}, addr, cfg, tlsCfg)
	if err != nil {
		return describeConnErr(err)
	}

	b.mu.Lock()
	b.client = client
	b.mu.Unlock()
	return nil
}

// joinChannel moves the bot to the configured channel, or stays in root if not found.
func (b *Bot) joinChannel() {
	ch := b.cfg.Server.Channel
	if ch == "" {
		return
	}

	parts := strings.Split(ch, "/")
	channel := b.client.Channels.Find(parts...)
	if channel == nil {
		b.log.Warn("channel not found, staying in root", slog.String("channel", ch))
		return
	}
	b.client.Self.Move(channel)
}

// setComment sets the bot's comment visible on the user list.
func (b *Bot) setComment() {
	if b.cfg.Bot.Comment != "" {
		b.client.Self.SetComment(b.cfg.Bot.Comment)
	}
}

// setAvatar loads a PNG from cfg.Bot.Avatar and sends it as the bot's texture.
// A missing or unreadable file logs a warning and is otherwise ignored.
func (b *Bot) setAvatar() {
	path := b.cfg.Bot.Avatar
	if path == "" {
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		b.log.Warn("could not read avatar file, skipping", slog.String("path", path), slog.String("err", err.Error()))
		return
	}

	b.client.Self.SetTexture(data)
}
