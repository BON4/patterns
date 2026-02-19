package ratelimit

import (
	"time"
)

type fixedWindowLimiter struct {
	limit int64
}

func GetFixedWindowLimiter(
	ip string,
	params *RateLimiterParams,
) (RateLimiter, error) {
	f := fixedWindowLimiter{}

	cmd := params.RClient.Incr(params.Ctx, ip)
	if err := cmd.Err(); err != nil {
		return &f, err
	}

	limit, err := cmd.Result()
	if err != nil {
		return &f, err
	}

	if limit != 1 {
		f.limit = params.Rpm - limit
		return &f, nil
	}

	return &f, params.RClient.Expire(params.Ctx, ip, time.Second*60).Err()
}

func (l *fixedWindowLimiter) Allow() bool {
	if l.limit <= 0 {
		return false
	}

	return true
}
