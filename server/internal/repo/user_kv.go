package repo

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/BON4/patterns/server/internal/infra"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type UserKvRepo struct {
	rClient             *redis.Client
	LockPrefix          string
	KeyPrefix           string
	CurrentKeyPrefix    string
	ProcessingKeyPrefix string
}

func NewUserKvRepo(rClient *redis.Client) *UserKvRepo {
	return &UserKvRepo{
		rClient:             rClient,
		LockPrefix:          "requests-lock",
		KeyPrefix:           "requests",
		CurrentKeyPrefix:    "current",
		ProcessingKeyPrefix: "processing",
	}
}

func (r *UserKvRepo) SaveUserRequest(ctx context.Context, ip string) error {
	res := r.rClient.HIncrBy(ctx, r.KeyPrefix+r.CurrentKeyPrefix, ip, 1)

	return res.Err()
}

type UserRequestsDump struct {
	IP    string
	Count int64
}

// 1. Get lock
// 2. Rename requests:current set to requests:processing so that new requests still will be writen to requests:current but old ones will be safe to process.
// 3. Dump all requests
// 4. Release lock
// No need for lock auto-renewing or fecing token implementation becouse operation is fast
func (r *UserKvRepo) DumpUserRequests(ctx context.Context) ([]UserRequestsDump, error) {
	var (
		currLockName = r.LockPrefix + ":" + "DumpUserRequests"
		lockValue    = uuid.New().String()
	)
	err := r.rClient.SetArgs(ctx, currLockName, lockValue, redis.SetArgs{Mode: "NX", TTL: time.Minute}).Err()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, infra.ErrLockNotAcquired
		}
		return nil, fmt.Errorf("failed to accquere lock: %w", err)
	}

	defer infra.ReleaseLock(ctx, r.rClient, currLockName, lockValue)

	err = r.rClient.Rename(ctx, r.KeyPrefix+r.CurrentKeyPrefix, r.KeyPrefix+r.ProcessingKeyPrefix).Err()
	if err != nil {
		if strings.Contains(err.Error(), "no such key") {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to rename hash: %w", err)
	}

	resp := r.rClient.HGetAll(ctx, r.KeyPrefix+r.ProcessingKeyPrefix)
	if err := resp.Err(); err != nil {
		return nil, fmt.Errorf("failed to get requests: %w", err)
	}

	if resp.Val() == nil {
		return nil, nil
	}

	// Hold the lock for a short while to simulate a long-running job
	time.Sleep(time.Second * 5)

	dumps := make([]UserRequestsDump, 0, len(resp.Val()))
	for ip, countStr := range resp.Val() {
		count, err := strconv.ParseInt(countStr, 10, 64)
		if err != nil {
			return nil, err
		}
		dumps = append(dumps, UserRequestsDump{
			IP:    ip,
			Count: count,
		})
	}
	return dumps, nil
}
