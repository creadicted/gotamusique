# Stage 1 — build
FROM golang:1.25-alpine AS builder
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
