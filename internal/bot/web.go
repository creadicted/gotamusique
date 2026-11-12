package bot

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"time"

	"github.com/konradk/gotamusique/internal/config"
	"github.com/konradk/gotamusique/internal/web"
)

// WebLogin implements the !web command's side: it returns the address a user opens
// to reach the web UI. With token auth that is a one-time login link valid for
// ttl; without a login it is the plain address and ttl is 0. It returns
// web.ErrDisabled when the web interface is not enabled.
func (b *Bot) WebLogin(id web.Identity) (link string, ttl time.Duration, err error) {
	w := b.cfg.Web
	if !w.Enabled || (b.webAuth == nil && w.AuthMethod == config.AuthToken) {
		return "", 0, web.ErrDisabled
	}
	if w.AuthMethod == config.AuthNone {
		return w.AccessAddress + "/", 0, nil
	}
	token, err := b.webAuth.IssueToken(id)
	if err != nil {
		return "", 0, err
	}
	b.log.Info("web login link issued", slog.String("user", id.Name), slog.String("subject", id.Subject))
	return web.LoginURL(w.AccessAddress, token), b.webAuth.TokenTTL(), nil
}

// StartWeb starts the web interface when it is enabled. It listens before
// returning, so a bad address or a port already in use is reported to the caller
// instead of failing later in the background. The server stops when ctx is
// cancelled. It does nothing when the web interface is disabled.
func (b *Bot) StartWeb(ctx context.Context) error {
	w := b.cfg.Web
	if !w.Enabled {
		return nil
	}
	addr := net.JoinHostPort(w.Addr, strconv.Itoa(w.Port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("web interface: %w", err)
	}
	srv := web.NewServer(b, b.webAuth, web.Options{
		Addr: addr, AccessAddress: w.AccessAddress, AuthMethod: w.AuthMethod, Log: b.log,
	})
	go func() {
		if err := srv.Serve(ctx, ln); err != nil {
			b.log.Error("web interface stopped", slog.String("err", err.Error()))
		}
	}()
	return nil
}
