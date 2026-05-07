package app

import (
	"context"
	"errors"
	"time"

	"github.com/fibegg/go-fibe/internal/config"
	"github.com/fibegg/go-fibe/internal/db"
	"github.com/fibegg/go-fibe/internal/event"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

type App struct {
	Config      config.Config
	Store       *db.Store
	Redis       *redis.Client
	Events      *event.Bus
	AsynqClient *asynq.Client
}

func New(ctx context.Context, cfg config.Config) (*App, error) {
	var store *db.Store
	if err := retry(ctx, 30*time.Second, 500*time.Millisecond, func() error {
		var err error
		store, err = db.Connect(ctx, cfg)
		return err
	}); err != nil {
		return nil, err
	}
	redisOpts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		store.Close()
		return nil, err
	}
	redisClient := redis.NewClient(redisOpts)
	if err := retry(ctx, 30*time.Second, 500*time.Millisecond, func() error {
		return redisClient.Ping(ctx).Err()
	}); err != nil {
		store.Close()
		return nil, err
	}
	asynqOpt, err := cfg.RedisAsynqOpt()
	if err != nil {
		store.Close()
		_ = redisClient.Close()
		return nil, err
	}
	return &App{
		Config:      cfg,
		Store:       store,
		Redis:       redisClient,
		Events:      event.NewBus(),
		AsynqClient: asynq.NewClient(asynqOpt),
	}, nil
}

func retry(ctx context.Context, timeout, interval time.Duration, fn func() error) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	var lastErr error
	for {
		lastErr = fn()
		if lastErr == nil {
			return nil
		}
		wait := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			wait.Stop()
			return errors.Join(ctx.Err(), lastErr)
		case <-deadline.C:
			wait.Stop()
			return lastErr
		case <-wait.C:
		}
	}
}

func (a *App) Close() {
	if a.AsynqClient != nil {
		_ = a.AsynqClient.Close()
	}
	if a.Redis != nil {
		_ = a.Redis.Close()
	}
	if a.Store != nil {
		a.Store.Close()
	}
}
