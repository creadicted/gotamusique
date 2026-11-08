# 3-08 — Web Frontend

**Status:** todo  
**Depends on:** 3-07  
**Unlocks:** nothing

## Objective

Serve the existing webpack-built frontend from the Go binary and verify end-to-end functionality.

## Approach (recommended)

Embed the built `static/` directory into the binary with `//go:embed`:

```go
//go:embed static
var staticFS embed.FS

mux.Handle("/static/", http.FileServer(http.FS(staticFS)))
```

The Makefile `build` target runs `npm run build` in `web/` first.

## Template handling

The Python version pre-processes HTML templates per language. For the Go version, serve the pre-processed HTML files from `web/templates/`. **Note:** the frontend sources currently live only in the local, gitignored `ref/web/` and `ref/static/` folders, not in the repository; copy them into the repo first. Select the file matching `config.Bot.Language` at the `/` route.

## Deliverables

- `Makefile` — `build-frontend` target; `build` depends on it
- Updated `internal/web/server.go` — embed and serve static files
- Verified: full web UI works end-to-end with the new backend

## Acceptance criteria

- `http://localhost:8181` shows the web interface
- Playlist, controls, library, upload all work
- No JS console errors for missing API endpoints
