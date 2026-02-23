package transport_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/BON4/patterns/server/internal/infra"
	"github.com/BON4/patterns/server/internal/repo"
	"github.com/BON4/patterns/server/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// --- Mocks ---

type mockUserMongoRepo struct {
	mu     sync.Mutex
	called bool
}

func (m *mockUserMongoRepo) DumpRequestCounts(ctx context.Context, updates []repo.UserRequestUpdate) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.called = true
	return nil
}

// --- Helpers ---

func newTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Skipf("redis unavailable, skipping: %v", err)
	}
	t.Cleanup(func() { rdb.FlushDB(context.Background()) })
	return rdb
}

func prepareInputData(t *testing.T, kvRepo *repo.UserKvRepo) {
	t.Helper()
	requests := map[string]int{
		"127.0.0.1":   5,
		"192.168.1.1": 3,
	}
	for ip, count := range requests {
		for range count {
			if err := kvRepo.SaveUserRequest(context.Background(), ip); err != nil {
				t.Fatalf("failed to save user request for %s: %v", ip, err)
			}
		}
	}
}

func newTestEngine(userService *service.UserService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/users/dump-requests", func(c *gin.Context) {
		if err := userService.SyncToMongo(c.Request.Context()); err != nil {
			if errors.Is(err, infra.ErrLockNotAcquired) {
				c.JSON(http.StatusAccepted, gin.H{"status": "already_processing"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "updated"})
	})
	return engine
}

// --- Tests ---

func TestDumpRequestsConcurrent(t *testing.T) {
	rdb := newTestRedis(t)

	kvRepo := repo.NewUserKvRepo(rdb)
	mongoRepo := &mockUserMongoRepo{}
	userService := service.NewUserService(kvRepo, mongoRepo)

	prepareInputData(t, kvRepo)

	ts := httptest.NewServer(newTestEngine(userService))
	defer ts.Close()

	doPost := func() (*http.Response, error) {
		return http.Post(ts.URL+"/users/dump-requests", "application/json", nil)
	}

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		results []*http.Response
	)

	for i := range 2 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i == 1 {
				time.Sleep(20 * time.Millisecond)
			}
			resp, err := doPost()
			if err != nil {
				t.Errorf("request %d failed: %v", i, err)
				return
			}
			if resp.StatusCode == http.StatusInternalServerError {
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				t.Errorf("unexpected 500 on request %d: %s", i, body)
				return
			}
			mu.Lock()
			results = append(results, resp)
			mu.Unlock()
		}(i)
	}
	wg.Wait()

	if len(results) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(results))
	}

	var okCount, acceptedCount int
	for _, resp := range results {
		defer resp.Body.Close()

		var body map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&body)

		switch resp.StatusCode {
		case http.StatusOK:
			okCount++
		case http.StatusAccepted:
			acceptedCount++
			if body["status"] != "already_processing" {
				t.Errorf("expected already_processing, got %v", body)
			}
		}
	}

	if okCount != 1 || acceptedCount != 1 {
		t.Fatalf("expected 1 OK and 1 Accepted, got ok=%d accepted=%d", okCount, acceptedCount)
	}

	mongoRepo.mu.Lock()
	defer mongoRepo.mu.Unlock()
	if !mongoRepo.called {
		t.Error("expected DumpRequestCounts to be called")
	}
}
