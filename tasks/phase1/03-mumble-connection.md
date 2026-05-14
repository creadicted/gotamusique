# 1-03 — Mumble Connection

**Status:** todo  
**Depends on:** 1-02  
**Unlocks:** 1-04, 1-07

## Objective

Connect to a Mumble server, join the configured channel, set the bot's comment and avatar, and handle SIGINT for a clean shutdown.

## Deliverables

- `internal/bot/bot.go` — `Bot` struct, `New()` constructor, `Shutdown()`
- `internal/bot/connect.go` — `connect()`, `joinChannel()`, `setComment()`, `setAvatar()` (all methods on `*Bot`), `Run(ctx)`
- `cmd/gotamusique/main.go` — wires config → `bot.New` → `bot.Run` → exit code

## Bot struct (Phase 1 fields)

```go
type Bot struct {
    cfg    *config.Config
    client *gumble.Client
    queue  *queue.Queue      // nil until 1-06
    audio  *audio.Pipeline   // nil until 1-04
    log    *slog.Logger
    mu     sync.Mutex
    cancel context.CancelFunc
}
```

## Constructor

```go
func New(cfg *config.Config) *Bot
```

- Builds the `slog.Logger`: writes to stderr by default; if `cfg.Bot.Logfile` is non-empty, opens the file and writes there instead
- If `cfg.Debug.MumbleConnection` is true, sets log level to `slog.LevelDebug`
- Initialises all fields; `queue` and `audio` start nil

## Connection behaviour

- `connect()` is a single-attempt method; `Run()` owns the retry loop
- TLS: load client cert from `cfg.Server.Certificate` if set; set `InsecureSkipVerify` from `cfg.Server.TLSSkipVerify` (default `true`)
- Tokens passed from `cfg.Server.Tokens`
- Channel join: always `strings.Split(cfg.Server.Channel, "/")` → `client.Channels.Find(...)`; if not found, stay in root and log a warning
- On **initial** connect failure: log error, return error → `main.go` exits 1
- On **post-connect** disconnect: reconnect loop with exponential backoff (see below)

## Reconnect loop (`Run`)

```go
func (b *Bot) Run(ctx context.Context) error
```

- Returns `nil` on clean shutdown (context cancelled)
- Returns an error after 10 consecutive failed reconnect attempts
- Backoff schedule: 5 → 10 → 20 → 40 → 60s (2× doubling, capped at 60s)
- Retry counter resets to 0 after each successful connection

## gumble event handlers (this milestone only)

Register exactly two handlers; leave TODO comments for later milestones:

```go
// TODO(1-07): register TextMessageEvent handler
// TODO(1-04): register audio event handler
client.Config.Attach(events.ConnectHandler(...))   // calls joinChannel, setComment, setAvatar
client.Config.Attach(events.DisconnectHandler(...)) // signals reconnect loop
```

## Avatar

- Loaded from `cfg.Bot.Avatar` (file path); expected format: PNG
- If the path is empty or the file does not exist: log a warning and continue — a missing avatar must not prevent connection
- Sent via `client.Self.SetTexture(bytes)`

## SIGINT handling

```go
ctx, cancel := context.WithCancel(context.Background())
sig := make(chan os.Signal, 1)
signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
go func() {
    <-sig
    cancel()
}()
err := bot.Run(ctx)
```

## Shutdown

```go
func (b *Bot) Shutdown()
```

- Acquires `b.mu`
- Nil-guards: `if b.audio != nil { b.audio.Stop() }` — audio pipeline not present until 1-04
- Calls `b.client.Disconnect()`
- No explicit sound-buffer drain in this milestone — revisit in 1-04 once the pipeline exists

## main.go wiring

```go
userPath, apply := config.ParseFlags()
if userPath == "" {
    userPath = config.DefaultUserConfigPath()
}
cfg, err := config.Load(userPath)
// handle err → exit 1
apply(cfg)

bot := bot.New(cfg)

ctx, cancel := context.WithCancel(context.Background())
sig := make(chan os.Signal, 1)
signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
go func() { <-sig; cancel() }()

if err := bot.Run(ctx); err != nil {
    log.Println(err)
    os.Exit(1)
}
```

## Acceptance criteria

- Bot appears in the Mumble channel list after startup
- Comment is visible on the bot's user entry
- SIGINT causes a clean disconnect (bot disappears from user list within 2s)
- Wrong host/port logs an error and exits 1 (no panic)
- After a disconnect, the bot reconnects automatically with backoff; exits 1 after 10 consecutive failures
- `tls_skip_verify = True` allows connection to servers with self-signed certificates
