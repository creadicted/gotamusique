# 1-08 — Docker

**Status:** todo  
**Depends on:** 1-01 through 1-07  
**Unlocks:** deployment

## Objective

A minimal multi-stage Docker image that ships the Phase 1 bot. No Python, no pip, no Node.js at runtime.

## Dockerfile

```dockerfile
# Stage 1 — build
FROM golang:1.21-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -ldflags="-s -w" -o /bin/gotamusique ./cmd/gotamusique

# Stage 2 — runtime
FROM alpine:3.19
RUN apk add --no-cache ffmpeg ca-certificates
COPY --from=builder /bin/gotamusique /usr/local/bin/gotamusique
COPY configuration.default.ini /app/
WORKDIR /app
ENTRYPOINT ["gotamusique", "--config", "configuration.ini"]
```

No yt-dlp, no libopus-dev, no Python. ffmpeg only (handles Opus encoding internally).

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
tasks/
web/
static/
media/
lang/
*.py
*.sh
venv/
__pycache__/
.idea/
```

## Makefile targets

```makefile
docker-build:
    docker build -t gotamusique .

docker-run:
    docker compose up
```

## Image size target

< 60 MB (Alpine + ffmpeg binary, no Python, no Node.js).

## Acceptance criteria

- `docker build .` succeeds from a clean checkout
- `docker run` with a mounted `configuration.ini` connects to Mumble and plays radio
- Image is under 100 MB
- Container restarts automatically on crash (`restart: unless-stopped`)
