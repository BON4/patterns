package ratelimit

import (
	"context"

	"github.com/redis/go-redis/v9"
)

type RateLimiter interface {
	Allow() bool
}

type RateLimiterParams struct {
	Ctx     context.Context
	RClient *redis.Client
	Rpm     int64
}
