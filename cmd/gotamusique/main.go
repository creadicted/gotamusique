package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/konradk/gotamusique/internal/bot"
	"github.com/konradk/gotamusique/internal/config"
	_ "layeh.com/gumble/opus" // registers Opus encoder/decoder with gumble
)

const version = "0.1.9"

func main() {
	os.Exit(run())
}

func run() int {
	userPath, apply := config.ParseFlags()
	if userPath == "" {
		userPath = config.DefaultUserConfigPath()
	}

	cfg, err := config.Load(userPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gotamusique: config: %v\n", err)
		return 1
	}
	apply(cfg)

	b, err := bot.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gotamusique: %v\n", err)
		return 1
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		cancel()
	}()

	slog.Info("gotamusique starting", slog.String("version", version))

	if err := b.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "gotamusique: %v\n", err)
		return 1
	}

	return 0
}
