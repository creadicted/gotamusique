# 1-02 — Configuration

**Status:** done  
**Depends on:** 1-01  
**Unlocks:** 1-03, 1-04, 1-05, 1-07

## Objective

Load a `configuration.ini` user file (next to the binary) merged over embedded defaults into a typed `Config` struct. Only sections needed for Phase 1 are required.

## Design decisions

| Decision | Choice |
|---|---|
| Defaults delivery | Embedded via `//go:embed` — single binary, no companion file required |
| User file location | `configuration.ini` next to the executable (`os.Executable()`); `--config` overrides |
| Unknown sections in user file | Silently skipped (Phase 2 adds `[webinterface]`; users may have it already) |
| Unknown keys in known sections | Hard error naming the offending key |
| Default file validation | None — we own it |
| `[commands]` and `[radio]` validation | Skipped — all keys valid (dynamic maps) |
| Unknown key detection | Explicit per-section allowlist, not reflection |
| CLI flag library | `github.com/spf13/pflag` |
| `Tokens` / `Admin` type | `[]string`, comma-split |
| `command_symbol` | `[]string`, colon-split (e.g. `!:！`) |
| Radio preset format | `key = URL "optional comment"` — empty URL is error, malformed comment uses raw remainder |

## Sections needed in Phase 1

```ini
[server]
host     = 127.0.0.1
port     = 64738
password =
channel  =
certificate =
tokens   =

[bot]
username  = gotamusique
volume    = 0.8
max_volume = 1.0
bandwidth = 96000
admin     =
comment   = "I play radio."
avatar    =
stereo    = True
logfile   =
announce_current_music = True

[commands]
command_symbol = !
# all command aliases — loaded generically, unused ones ignored by dispatcher

[radio]
# key = URL "optional comment"
jazz = http://jazz-wr04.ice.infomaniak.ch/jazz-wr04-128.mp3 "Jazz Yeah !"

[debug]
ffmpeg            = False
mumble_connection = False
```

## Deliverables

- `internal/config/configuration.default.ini` — embedded Phase 1 defaults
- `internal/config/config.go` — `Config` struct, `Load(userPath string) (*Config, error)`, `DefaultUserConfigPath() string`
- `internal/config/flags.go` — `ParseFlags() (userPath string, apply func(*Config))` via pflag
- `internal/config/config_test.go` — three table-driven tests (happy path, unknown key, missing user file)

## Config struct (Phase 1 fields)

```go
type Config struct {
    Server   ServerConfig
    Bot      BotConfig
    Commands CommandsConfig
    Radio    map[string]RadioPreset
    Debug    DebugConfig
}

type CommandsConfig struct {
    Symbol  []string            // colon-split, e.g. ["!", "！"]
    Aliases map[string][]string // canonical name → alias list
}

type RadioPreset struct {
    URL     string
    Comment string
}
```

## CLI flags

| Flag | Short | Overrides |
|---|---|---|
| `--config` | | user config file path |
| `--server` | `-s` | `Server.Host` |
| `--username` | `-u` | `Bot.Username` |
| `--password` | `-P` | `Server.Password` |
| `--port` | `-p` | `Server.Port` |
| `--certificate` | `-c` | `Server.Certificate` |
| `--channel` | `-C` | `Server.Channel` |
| `--bandwidth` | `-b` | `Bot.Bandwidth` |

`apply` uses `pflag.Changed` to override only explicitly set flags.

## Usage in main

```go
userPath, apply := config.ParseFlags()
if userPath == "" {
    userPath = config.DefaultUserConfigPath()
}
cfg, err := config.Load(userPath)
// handle err
apply(cfg)
```

## Acceptance criteria

- `config.Load("testdata/valid.ini")` returns a populated `Config`
- Unknown key in user file returns an error naming the offending key
- `Load` with a non-existent path succeeds with embedded defaults
- CLI flag `-s myhost` overrides `config.Server.Host`
- `config.Radio["jazz"].URL` returns the preset URL
