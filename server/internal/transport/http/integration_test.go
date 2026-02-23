package http

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

	"github.com/BON4/patterns/server/internal/infra/redisclient"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

type mockUserRepo struct {
	mu     sync.Mutex
	called bool
}

func (m *mockUserRepo) DumpRequestCounts(ctx context.Context, updates []interface{}) error {
	m.mu.Lock()
	m.called = true
	m.mu.Unlock()
	return nil
}

func prepareInputData(t *testing.T, userKv *redisclient.UserKvRepo) {
	// Prepare some dummy data in redis
	for range 5 {
		err := userKv.SaveUserRequest(context.Background(), "127.0.0.1")
		if err != nil {
			t.Fatalf("failed to save user request: %v", err)
		}
	}

	for range 3 {
		err := userKv.SaveUserRequest(context.Background(), "192.168.1.1")
		if err != nil {
			t.Fatalf("failed to save user request: %v", err)
		}
	}
}

func TestDumpRequestsConcurrent(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	userKv := redisclient.NewUserKvRepo(rdb)

	prepareInputData(t, userKv)

	userRepo := &mockUserRepo{}

	// gin engine with handler similar to production route
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/users/dump-requests", func(c *gin.Context) {
		dump, err := userKv.DumpUserRequests(c.Request.Context())
		if err != nil {
			if errors.Is(err, redisclient.ErrLockNotAcquired) {
				c.JSON(http.StatusAccepted, gin.H{"status": "already_processing"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err})
			return
		}

		// convert dummy data to expected mongo shape then call mock
		_ = dump
		_ = userRepo.DumpRequestCounts(c.Request.Context(), nil)

		c.JSON(http.StatusOK, gin.H{"status": "updated"})
	})

	ts := httptest.NewServer(engine)
	defer ts.Close()

	var wg sync.WaitGroup
	wg.Add(2)

	results := make([]*http.Response, 2)

	doPost := func(i int) {
		defer wg.Done()
		resp, err := http.Post(ts.URL+"/users/dump-requests", "application/json", nil)
		if err != nil {
			t.Errorf("post error: %v", err)
			return
		}

		if resp.StatusCode == 500 {
			body, _ := io.ReadAll(resp.Body)
			t.Errorf("unexpected 500 response: %s", body)
			resp.Body.Close()
			return
		}
		results[i] = resp
	}

	go doPost(0)
	// small stagger to increase contention
	time.Sleep(20 * time.Millisecond)
	go doPost(1)

	wg.Wait()

	// parse results
	statuses := map[int]int{}
	bodies := make([]map[string]interface{}, 2)
	for i, resp := range results {
		if resp == nil {
			t.Fatalf("nil response for idx %d", i)
		}
		statuses[i] = resp.StatusCode
		var b map[string]interface{}
		_ = json.NewDecoder(resp.Body).Decode(&b)
		bodies[i] = b
		resp.Body.Close()
	}

	// Expect one 200 and one 202 with already_processing
	var okCount, acceptedCount int
	for i := 0; i < 2; i++ {
		if statuses[i] == http.StatusOK {
			okCount++
		}
		if statuses[i] == http.StatusAccepted {
			acceptedCount++
			if bodies[i]["status"] != "already_processing" {
				t.Fatalf("expected already_processing body, got %v", bodies[i])
			}
		}
	}

	if okCount != 1 || acceptedCount != 1 {
		t.Fatalf("expected one OK and one Accepted; got ok=%d, accepted=%d, statuses=%v", okCount, acceptedCount, statuses)
	}
}
