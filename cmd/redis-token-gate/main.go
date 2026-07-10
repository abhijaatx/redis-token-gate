package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/abhijaatx/redis-token-gate/internal/config"
	"github.com/abhijaatx/redis-token-gate/internal/httpapi"
	"github.com/abhijaatx/redis-token-gate/internal/limiter"
	"github.com/abhijaatx/redis-token-gate/internal/metrics"
	"github.com/abhijaatx/redis-token-gate/internal/server"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	config, err := config.Load()
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	rateLimiter, err := limiter.NewRedisLimiter(config.RedisURL, config.KeyPrefix, config.Policy)
	if err != nil {
		logger.Error("create Redis limiter", "error", err)
		os.Exit(1)
	}
	defer rateLimiter.Close()

	startupContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	err = rateLimiter.Ping(startupContext)
	cancel()
	if err != nil {
		logger.Error("Redis is unavailable", "error", err)
		os.Exit(1)
	}

	meter := &metrics.Metrics{}
	handler := httpapi.New(rateLimiter, config.Policy, config.APIToken, config.RequestTimeout, meter, logger)
	if err := server.Run(config.ListenAddr, handler, logger); err != nil {
		logger.Error("server exited", "error", err)
		os.Exit(1)
	}
}
