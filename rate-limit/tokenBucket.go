package ratelimit

import (
	"time"

	"github.com/redis/go-redis/v9"
)

var script = redis.NewScript(`
local key = KEYS[1]

local capacity = tonumber(ARGV[1])
local refill_rate = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local requested = tonumber(ARGV[4])

local bucket = redis.call("HMGET", key, "tokens", "last_refill")

local tokens = tonumber(bucket[1])
local last_refill = tonumber(bucket[2])

if tokens == nil then
  tokens = capacity
  last_refill = now
end

local delta = math.max(0, now - last_refill)
local refill = delta * refill_rate
tokens = math.min(capacity, tokens + refill)

local allowed = tokens >= requested

if allowed then
  tokens = tokens - requested
end

redis.call("HMSET", key,
  "tokens", tokens,
  "last_refill", now
)

redis.call("EXPIRE", key, math.ceil(capacity / refill_rate * 2))

return allowed and 1 or 0
`)

type tokenBucketRateLimiter struct {
	allowed int
}

func GetTokenBucketLimiter(
	ip string,
	params *RateLimiterParams,
) (RateLimiter, error) {
	capacity := float64(params.Rpm)
	refillRate := capacity / 60000.0
	now := float64(time.Now().UnixMilli())

	allowed, err := script.Run(params.Ctx, params.RClient,
		[]string{"ratelimit:" + ip},
		capacity,
		refillRate,
		now,
		1,
	).Int()

	if err != nil {
		return nil, err
	}

	return &tokenBucketRateLimiter{allowed: allowed}, nil
}

func (l *tokenBucketRateLimiter) Allow() bool {
	if l.allowed != 1 {
		return false
	}

	return true
}
