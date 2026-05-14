# 1-02 — Configuration

**Status:** todo  
**Depends on:** 1-01  
**Unlocks:** 1-03, 1-04, 1-05, 1-07

## Objective

Load `configuration.ini` merged over `configuration.default.ini` into a typed `Config` struct. Only the sections needed for Phase 1 are required.

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
# all command aliases live here (see configuration.default.ini)

[radio]
# preset aliases → URL [comment]
jazz = http://example.com/jazz.mp3 "Jazz Yeah!"

[debug]
ffmpeg            = False
mumble_connection = False
```

## Deliverables

- `internal/config/config.go` — `Config` struct, `Load(defaultPath, userPath string) (*Config, error)`
- `internal/config/flags.go` — CLI flags that override config after loading (`-s`, `-u`, `-P`, `-p`, `-c`, `-C`, `-b`, `--config`)
- Unknown keys in the user config file return a descriptive error

## Config struct (Phase 1 fields)

```go
type Config struct {
    Server   ServerConfig
    Bot      BotConfig
    Commands CommandsConfig   // map[canonicalName][]string aliases
    Radio    map[string]RadioPreset
    Debug    DebugConfig
}

type RadioPreset struct {
    URL     string
    Comment string
}
```

## Acceptance criteria

- `config.Load("configuration.default.ini", "configuration.ini")` returns a populated `Config`
- Unknown key in user file returns an error listing the offending key
- CLI flag `-s myhost` overrides `config.Server.Host`
- `config.Radio["jazz"].URL` returns the preset URL
