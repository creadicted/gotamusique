# 1-08 — Docker

**Status:** todo  
**Depends on:** 1-01 through 1-07  
**Unlocks:** 1-09 (GHCR publish), deployment

## Objective

A minimal multi-stage Docker image that ships the Phase 1 bot. No Python, no pip, no Node.js at runtime.

## Dockerfile

```dockerfile
# Stage 1 — build
FROM golang:1.25.9-alpine AS builder
WORKDIR /src
RUN apk add --no-cache gcc musl-dev opus-dev
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -ldflags="-s -w -extldflags '-static'" -o /bin/gotamusique ./cmd/gotamusique

# Stage 2 — runtime
FROM alpine:3.19
RUN apk add --no-cache ffmpeg ca-certificates
COPY --from=builder /bin/gotamusique /usr/local/bin/gotamusique
COPY internal/config/configuration.default.ini /app/configuration.default.ini
WORKDIR /app
ENTRYPOINT ["gotamusique"]
CMD ["--config", "configuration.ini"]
```

- Static linking (`-extldflags '-static'` + `musl-dev`) means the runtime image needs no `libopus.so`.
- `gcc` and `opus-dev` are build-time only; they do not appear in the runtime image. `opus-dev` is required because `layeh.com/gopus` (used by gumble to encode PCM into Opus frames for Mumble) is a CGO wrapper around libopus.
- `ffmpeg` decodes the input stream to raw 48 kHz s16le PCM; gumble (via gopus/libopus) then encodes those frames as Opus and sends them to Mumble.
- `ca-certificates` is needed for TLS to the Mumble server.
- `configuration.default.ini` is baked in as the base config. Users mount `configuration.ini` on top.
- Splitting `ENTRYPOINT`/`CMD` allows `docker run gotamusique --config /other.ini` without `--entrypoint`.

## docker-compose.yml

```yaml
services:
  gotamusique:
    build: .
    volumes:
      - ./configuration.ini:/app/configuration.ini:ro
    restart: unless-stopped
```

## .dockerignore

```
.git
bin/
ref/
tasks/
web/
static/
media/
lang/
.idea/
# local config and secrets — never needed at build time
configuration.ini
.env
*.db
*.db-shm
*.db-wal
```

## Makefile targets

```makefile
docker-build:
	docker build -t gotamusique .

docker-run:
	docker compose up
```

## Image size target

Under 200 MB.

## Acceptance criteria

- `docker build .` succeeds from a clean checkout
- `docker run` with a mounted `configuration.ini` connects to Mumble and plays radio
- Image is under 200 MB
- Container restarts automatically on crash (`restart: unless-stopped`)
- `docker stop` triggers a clean shutdown (SIGTERM handled — verified in `main.go`)
