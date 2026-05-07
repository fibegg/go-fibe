package event

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const Channel = "uptime_console:events"

type Bus struct {
	mu          sync.Mutex
	nextID      int
	subscribers map[int]chan string
}

func NewBus() *Bus {
	return &Bus{subscribers: map[int]chan string{}}
}

func (b *Bus) Subscribe() (int, <-chan string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.nextID++
	ch := make(chan string, 16)
	b.subscribers[b.nextID] = ch
	return b.nextID, ch
}

func (b *Bus) Unsubscribe(id int) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if ch, ok := b.subscribers[id]; ok {
		delete(b.subscribers, id)
		close(ch)
	}
}

func (b *Bus) Publish(payload string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, ch := range b.subscribers {
		select {
		case ch <- payload:
		default:
		}
	}
}

func ForwardRedis(ctx context.Context, redisClient *redis.Client, bus *Bus) {
	for {
		pubsub := redisClient.Subscribe(ctx, Channel)
		ch := pubsub.Channel()
	readLoop:
		for {
			select {
			case <-ctx.Done():
				_ = pubsub.Close()
				return
			case msg, ok := <-ch:
				if !ok {
					_ = pubsub.Close()
					slog.Warn("redis event bridge disconnected")
					break readLoop
				}
				bus.Publish(msg.Payload)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}
