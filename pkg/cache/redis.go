package cache

import (
	"context"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

var Ctx = context.Background()

func Client() *redis.Client {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	return redis.NewClient(&redis.Options{Addr: addr})
}

// BalanceCacheKey per card
func BalanceKey(cardID string) string { return "card:balance:" + cardID }

func SetBalance(r *redis.Client, cardID string, usedLimit int64) {
	_ = r.Set(Ctx, BalanceKey(cardID), usedLimit, 60*time.Second).Err()
}
