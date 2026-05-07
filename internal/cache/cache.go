package cache

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

func GetJSON[T any](ctx context.Context, client *redis.Client, key string) (T, bool, error) {
	var zero T
	raw, err := client.Get(ctx, key).Result()
	if err == redis.Nil {
		return zero, false, nil
	}
	if err != nil {
		return zero, false, err
	}
	if err := json.Unmarshal([]byte(raw), &zero); err != nil {
		return zero, false, err
	}
	return zero, true, nil
}

func SetJSON(ctx context.Context, client *redis.Client, key string, value any, ttl time.Duration) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return client.Set(ctx, key, raw, ttl).Err()
}

func Delete(ctx context.Context, client *redis.Client, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	return client.Del(ctx, keys...).Err()
}
