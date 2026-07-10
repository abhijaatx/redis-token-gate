//go:build integration

package limiter

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"
)

func TestRedisLimiterConsumesAndDeniesAtomically(t *testing.T) {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379/15"
	}
	service, err := NewRedisLimiter(redisURL, "rtg:test:", Policy{Capacity: 10, RefillPerSecond: 1, MaxCost: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	if err := service.Ping(context.Background()); err != nil {
		t.Fatalf("Redis unavailable: %v", err)
	}

	identity := "integration-" + time.Now().UTC().Format("20060102150405.000000000")
	var allowed int
	var lock sync.Mutex
	var waitGroup sync.WaitGroup
	for range 30 {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			decision, err := service.Allow(context.Background(), identity, 1)
			if err != nil {
				t.Errorf("Allow() error = %v", err)
				return
			}
			if decision.Allowed {
				lock.Lock()
				allowed++
				lock.Unlock()
			}
		}()
	}
	waitGroup.Wait()
	if allowed != 10 {
		t.Fatalf("allowed decisions = %d, want exactly 10", allowed)
	}
}
