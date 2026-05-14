# 1-01 — Project Scaffold

**Status:** todo  
**Depends on:** nothing  
**Unlocks:** everything

## Objective

Bootstrap a working Go module with a clean directory structure and a passing `go build`.

## Deliverables

- `go.mod` — module path (e.g. `github.com/yourname/gotamusique`)
- `go.sum` — pinned dependencies
- `cmd/gotamusique/main.go` — entrypoint; prints version and exits
- `Makefile` — targets: `build`, `test`, `run`
- `.gitignore` additions: `bin/`, `*.db`

## Directory layout

```
cmd/gotamusique/      main entrypoint
internal/
  config/             INI loading
  bot/                Bot struct, main loop
  audio/              ffmpeg pipeline + volume
  radio/              RadioItem, RadioBrowser client
  queue/              simple in-memory queue
  command/            dispatcher + handlers
```

## Acceptance criteria

- `go build ./...` succeeds
- `go vet ./...` is clean
- `go test ./...` runs (zero test files is fine at this point)
- Binary prints `gotamusique v0.1.0` and exits 0
