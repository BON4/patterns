package service

import (
	"context"
	"fmt"

	"github.com/BON4/patterns/server/internal/repo"
)

type UserKvRepo interface {
	SaveUserRequest(ctx context.Context, ip string) error
	DumpUserRequests(ctx context.Context) ([]repo.UserRequestsDump, error)
}

type UserMongoRepo interface {
	DumpRequestCounts(ctx context.Context, updates []repo.UserRequestUpdate) error
}

type UserService struct {
	kv    UserKvRepo
	store UserMongoRepo
}

func NewUserService(kv UserKvRepo, store UserMongoRepo) *UserService {
	return &UserService{kv: kv, store: store}
}

func (s *UserService) TrackRequest(ctx context.Context, ip string) error {
	return s.kv.SaveUserRequest(ctx, ip)
}

func (s *UserService) SyncToMongo(ctx context.Context) error {
	dumps, err := s.kv.DumpUserRequests(ctx)
	if err != nil {
		return fmt.Errorf("dump from redis: %w", err)
	}
	if len(dumps) == 0 {
		return nil
	}

	updates := make([]repo.UserRequestUpdate, 0, len(dumps))
	for _, d := range dumps {
		updates = append(updates, repo.UserRequestUpdate{
			IP:           d.IP,
			RequestCount: d.Count,
		})
	}

	return s.store.DumpRequestCounts(ctx, updates)
}
