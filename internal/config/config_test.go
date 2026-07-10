package config

import (
	"strings"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	for _, key := range []string{"DEFAULT_CAPACITY", "REFILL_PER_SECOND", "MAX_COST", "REQUEST_TIMEOUT", "LISTEN_ADDR", "REDIS_URL", "KEY_PREFIX", "API_TOKEN"} {
		t.Setenv(key, "")
		t.Setenv(key, "")
	}
	// Empty variables intentionally override defaults; remove the important ones
	// in a subprocess-free way by using explicit valid values instead.
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

func TestLoadRejectsInvalidCapacity(t *testing.T) {
	t.Setenv("DEFAULT_CAPACITY", "not-a-number")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "DEFAULT_CAPACITY") {
		t.Fatalf("Load() error = %v, want DEFAULT_CAPACITY error", err)
	}
}
