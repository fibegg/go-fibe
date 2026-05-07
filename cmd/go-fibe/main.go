package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/fibegg/go-fibe/internal/app"
	"github.com/fibegg/go-fibe/internal/config"
	"github.com/fibegg/go-fibe/internal/db"
	"github.com/fibegg/go-fibe/internal/jobs"
	"github.com/fibegg/go-fibe/internal/server"
)

func main() {
	if err := run(); err != nil {
		slog.Error("command failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		return errors.New("usage: go-fibe setup|serve|worker")
	}

	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}
	configureLogging(cfg)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch os.Args[1] {
	case "setup":
		return setup(ctx, cfg)
	case "serve":
		return server.Serve(ctx, cfg)
	case "worker":
		return jobs.RunWorker(ctx, cfg)
	default:
		return fmt.Errorf("unknown command %q", os.Args[1])
	}
}

func setup(ctx context.Context, cfg config.Config) error {
	rt, err := app.New(ctx, cfg)
	if err != nil {
		return err
	}
	defer rt.Close()

	if err := db.Migrate(ctx, rt.Store.Pool(), "migrations"); err != nil {
		return err
	}
	return rt.Store.Seed(ctx, cfg.DemoPassword)
}

func configureLogging(cfg config.Config) {
	level := slog.LevelInfo
	if cfg.IsDevelopment() {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})))
}
