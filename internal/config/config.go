// Package config loads and validates the service's environment configuration.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/abhijaatx/redis-token-gate/internal/limiter"
)

// Config contains all runtime settings. Secrets are only read from the
// environment and never logged by the application.
type Config struct {
	ListenAddr     string
	RedisURL       string
	KeyPrefix      string
	APIToken       string
	RequestTimeout time.Duration
	Policy         limiter.Policy
}

// Load reads documented environment variables and returns actionable errors.
func Load() (Config, error) {
	capacity, err := int64Env("DEFAULT_CAPACITY", 10)
	if err != nil {
		return Config{}, err
	}
	refill, err := floatEnv("REFILL_PER_SECOND", 1)
	if err != nil {
		return Config{}, err
	}
	maxCost, err := int64Env("MAX_COST", capacity)
	if err != nil {
		return Config{}, err
	}
	timeout, err := durationEnv("REQUEST_TIMEOUT", 2*time.Second)
	if err != nil {
		return Config{}, err
	}

	config := Config{
		ListenAddr:     stringEnv("LISTEN_ADDR", ":8080"),
		RedisURL:       stringEnv("REDIS_URL", "redis://localhost:6379/0"),
		KeyPrefix:      stringEnv("KEY_PREFIX", "rtg:bucket:"),
		APIToken:       os.Getenv("API_TOKEN"),
		RequestTimeout: timeout,
		Policy: limiter.Policy{
			Capacity:        capacity,
			RefillPerSecond: refill,
			MaxCost:         maxCost,
		},
	}
	if err := config.Policy.Validate(); err != nil {
		return Config{}, fmt.Errorf("invalid rate-limit policy: %w", err)
	}
	if config.RequestTimeout <= 0 {
		return Config{}, fmt.Errorf("REQUEST_TIMEOUT must be greater than zero")
	}
	if strings.TrimSpace(config.KeyPrefix) == "" {
		return Config{}, fmt.Errorf("KEY_PREFIX cannot be empty")
	}
	return config, nil
}

func stringEnv(name, fallback string) string {
	if value, ok := os.LookupEnv(name); ok {
		return value
	}
	return fallback
}

func int64Env(name string, fallback int64) (int64, error) {
	value, ok := os.LookupEnv(name)
	if !ok {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", name, err)
	}
	return parsed, nil
}

func floatEnv(name string, fallback float64) (float64, error) {
	value, ok := os.LookupEnv(name)
	if !ok {
		return fallback, nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be a number: %w", name, err)
	}
	return parsed, nil
}

func durationEnv(name string, fallback time.Duration) (time.Duration, error) {
	value, ok := os.LookupEnv(name)
	if !ok {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a Go duration such as 2s: %w", name, err)
	}
	return parsed, nil
}
