package infra

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

func NewRedis(ctx context.Context, url string) (*redis.Client, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, err
	}

	rdb := redis.NewClient(opts)

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, err
	}

	return rdb, nil
}

var ErrLockNotAcquired error = fmt.Errorf("lock is not acquired")

func ReleaseLock(ctx context.Context, rdb *redis.Client, lockName string, lockUUID string) {
	// Remove lock by uuid
	lua := `
    if redis.call("get", KEYS[1]) == ARGV[1] then
        return redis.call("del", KEYS[1])
    else
        return 0
    end
    `
	_, _ = rdb.Eval(ctx, lua, []string{lockName}, lockUUID).Result()
}
