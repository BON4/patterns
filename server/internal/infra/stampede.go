package infra

import (
	"bytes"
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	STAMPEDE_PRIMARY_PRIORITY   = "primary"
	STAMPEDE_SECONDARY_PRIORITY = "secondary"
)

var ErrStampedeTimeout = errors.New("stampede timeout: resource unavailable")

type RedisStampede[T any] struct {
	rdb        *redis.Client
	LockPrefix string
	key        string
	chacheTTL  time.Duration
}

func NewRedisStampede[T any](client *redis.Client, key string, ttl time.Duration) *RedisStampede[T] {

	return &RedisStampede[T]{rdb: client, key: key, LockPrefix: "stampede-" + key + "-lock", chacheTTL: ttl}
}

//  1. GetKey if exists return obj
//  2. If not exits try to get lock
//  3. if lock is free - get lock, fetch resource, release lock
//  4. if lock is not free - wait with timeout, and get resource
//  5. if timeout, become primary by trying to get lock,
//     all other waiter wiil failt with timeout
func (s *RedisStampede[T]) Get(ctx context.Context, fetch func(ctx context.Context) (*T, error)) (*T, error) {
	var (
		foud      = true
		lockValue = uuid.New().String()
		obj       = new(T)
	)

	res := s.rdb.Get(ctx, s.key)
	if err := res.Err(); err != nil {
		if !errors.Is(err, redis.Nil) {
			return nil, err
		}
		foud = false
	}

	if foud {
		b, err := res.Bytes()
		if err != nil {
			return nil, err
		}

		err = gob.NewDecoder(bytes.NewBuffer(b)).Decode(obj)
		if err != nil {
			return nil, err
		}
		return obj, nil
	}

	var priority = STAMPEDE_PRIMARY_PRIORITY
	err := s.rdb.SetArgs(ctx, s.LockPrefix, lockValue, redis.SetArgs{Mode: "NX", TTL: time.Minute}).Err()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			return nil, fmt.Errorf("failed to accquere lock: %w", err)
		}
		priority = STAMPEDE_SECONDARY_PRIORITY
	}

	switch priority {
	case STAMPEDE_PRIMARY_PRIORITY:
		defer ReleaseLock(ctx, s.rdb, s.LockPrefix, lockValue)
		return s.fetchPrimary(ctx, fetch)
	case STAMPEDE_SECONDARY_PRIORITY:
		return s.fetchSecondary(ctx, fetch)
	}

	return nil, fmt.Errorf("failed to fetch resourse")
}

func (s *RedisStampede[T]) fetchPrimary(ctx context.Context, fetch func(ctx context.Context) (*T, error)) (*T, error) {
	newObj, err := fetch(ctx)
	if err != nil {
		return nil, err
	}

	var objBuff = bytes.NewBuffer([]byte{})

	err = gob.NewEncoder(objBuff).Encode(newObj)
	if err != nil {
		return nil, err
	}

	err = s.rdb.SetArgs(ctx, s.key, objBuff.Bytes(), redis.SetArgs{TTL: s.chacheTTL}).Err()
	if err != nil {
		return nil, err
	}

	return newObj, nil
}

func (s *RedisStampede[T]) fetchSecondary(
	ctx context.Context,
	fetch func(ctx context.Context) (*T, error),
) (*T, error) {

	base := 20 * time.Millisecond
	max := 500 * time.Millisecond
	deadline := time.Now().Add(10 * time.Second)

	attempt := 0

	for {
		if time.Now().After(deadline) {
			newLockValue := uuid.New().String()

			err := s.rdb.SetArgs(ctx, s.LockPrefix, newLockValue,
				redis.SetArgs{Mode: "NX", TTL: time.Minute}).Err()
			if err != nil {
				if errors.Is(err, redis.Nil) {
					return nil, ErrStampedeTimeout
				}
			}

			defer ReleaseLock(ctx, s.rdb, s.LockPrefix, newLockValue)

			return s.fetchPrimary(ctx, fetch)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		res := s.rdb.Get(ctx, s.key)
		if err := res.Err(); err == nil {
			obj := new(T)
			b, _ := res.Bytes()
			if err := gob.NewDecoder(bytes.NewBuffer(b)).Decode(obj); err == nil {
				return obj, nil
			}
		}

		wait := min(base*time.Duration(1<<attempt), max)

		jitter := time.Duration(rand.Int63n(int64(wait / 2)))
		sleep := wait/2 + jitter

		attempt++

		select {
		case <-time.After(sleep):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}
