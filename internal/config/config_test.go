package config

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/abhijaatx/redis-token-gate/internal/limiter"
)

func TestLoadDefaults(t *testing.T) {
	unsetEnv(t, "DEFAULT_CAPACITY", "REFILL_PER_SECOND", "MAX_COST", "REQUEST_TIMEOUT", "LISTEN_ADDR", "REDIS_URL", "KEY_PREFIX", "API_TOKEN")

	config, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if config.ListenAddr != ":8080" || config.RedisURL != "redis://localhost:6379/0" || config.KeyPrefix != "rtg:bucket:" {
		t.Fatalf("unexpected connection defaults: %#v", config)
	}
	if config.Policy != (limiter.Policy{Capacity: 10, RefillPerSecond: 1, MaxCost: 10}) {
		t.Fatalf("unexpected policy defaults: %#v", config.Policy)
	}
	if config.RequestTimeout != 2*time.Second || config.APIToken != "" {
		t.Fatalf("unexpected timeout/token defaults: %#v", config)
	}
}

func TestLoadConfiguredValues(t *testing.T) {
	t.Setenv("DEFAULT_CAPACITY", "10")
	t.Setenv("REFILL_PER_SECOND", "1")
	t.Setenv("MAX_COST", "10")
	t.Setenv("REQUEST_TIMEOUT", "2s")
	t.Setenv("LISTEN_ADDR", ":8080")
	t.Setenv("REDIS_URL", "redis://localhost:6379/0")
	t.Setenv("KEY_PREFIX", "rtg:bucket:")
	config, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if config.Policy.Capacity != 10 || config.Policy.MaxCost != 10 {
		t.Fatalf("unexpected policy: %#v", config.Policy)
	}
}

func unsetEnv(t *testing.T, keys ...string) {
	t.Helper()
	for _, key := range keys {
		key := key
		old, wasSet := os.LookupEnv(key)
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("Unsetenv(%q): %v", key, err)
		}
		t.Cleanup(func() {
			if wasSet {
				_ = os.Setenv(key, old)
				return
			}
			_ = os.Unsetenv(key)
		})
	}
}

func TestLoadRejectsInvalidCapacity(t *testing.T) {
	t.Setenv("DEFAULT_CAPACITY", "not-a-number")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "DEFAULT_CAPACITY") {
		t.Fatalf("Load() error = %v, want DEFAULT_CAPACITY error", err)
	}
}
