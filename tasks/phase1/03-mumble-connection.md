# 1-03 — Mumble Connection

**Status:** todo  
**Depends on:** 1-02  
**Unlocks:** 1-04, 1-07

## Objective

Connect to a Mumble server, join the configured channel, set the bot's comment and avatar, and handle SIGINT for a clean shutdown.

## Deliverables

- `internal/bot/bot.go` — `Bot` struct holding the gumble client and all shared state
- `internal/bot/connect.go` — `Connect(cfg *config.Config) error`, `JoinChannel()`, `SetComment()`, `SetAvatar()`
- `cmd/gotamusique/main.go` — wires config → bot → connect → loop

## Bot struct (Phase 1 fields)

```go
type Bot struct {
    cfg     *config.Config
    client  *gumble.Client
    queue   *queue.Queue
    audio   *audio.Pipeline
    log     *slog.Logger
    exit    chan struct{}
}
```

## Connection behaviour

- TLS certificate loaded from `config.Server.Certificate` if set; otherwise no client cert
- Tokens passed from `config.Server.Tokens`
- Channel join by name (`client.Channels.Find`) or tree path (split on `/`)
- If channel not found, stay in root channel (log a warning, don't crash)
- On connection failure: log error, exit 1
- Reconnect loop: on disconnect, wait 5s then retry; exponential backoff up to 60s

## SIGINT handling

```go
sig := make(chan os.Signal, 1)
signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
go func() {
    <-sig
    bot.Shutdown()
}()
```

`Shutdown()` stops audio, drains the Mumble sound buffer, disconnects.

## Acceptance criteria

- Bot appears in the Mumble channel list after startup
- Comment is visible on the bot's user entry
- SIGINT causes a clean disconnect (bot disappears from user list within 2s)
- Wrong host/port logs an error and exits 1 (no panic)
