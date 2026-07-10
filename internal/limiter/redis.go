package limiter

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

//go:embed rate_limit.lua
var tokenBucketScript string

// RedisLimiter applies one policy atomically for all API instances that share
// the same Redis database.
type RedisLimiter struct {
	client *redis.Client
	policy Policy
	prefix string
	script *redis.Script
	now    func() time.Time
}

// NewRedisLimiter connects a limiter to a Redis URL such as
// redis://localhost:6379/0. The caller should call Ping before serving traffic.
func NewRedisLimiter(redisURL, prefix string, policy Policy) (*RedisLimiter, error) {
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse REDIS_URL: %w", err)
	}
	return &RedisLimiter{
		client: redis.NewClient(opts),
		policy: policy,
		prefix: prefix,
		script: redis.NewScript(tokenBucketScript),
		now:    time.Now,
	}, nil
}

// Allow consumes cost tokens for identity. Identity is SHA-256 hashed before
// storage so raw client identifiers are not persisted in Redis keys.
func (l *RedisLimiter) Allow(ctx context.Context, identity string, cost int64) (Decision, error) {
	if cost < 1 || cost > l.policy.MaxCost {
		return Decision{}, fmt.Errorf("cost must be between 1 and %d", l.policy.MaxCost)
	}

	result, err := l.script.Run(ctx, l.client, []string{l.bucketKey(identity)},
		l.policy.Capacity,
		l.policy.RefillPerSecond,
		cost,
		l.now().UnixMilli(),
		l.policy.BucketTTL().Milliseconds(),
	).Result()
	if err != nil {
		return Decision{}, fmt.Errorf("run token-bucket script: %w", err)
	}

	values, ok := result.([]interface{})
	if !ok || len(values) != 4 {
		return Decision{}, fmt.Errorf("unexpected token-bucket response: %T", result)
	}
	allowed, err := asInt64(values[0])
	if err != nil {
		return Decision{}, fmt.Errorf("decode allowed flag: %w", err)
	}
	remaining, err := asFloat64(values[1])
	if err != nil {
		return Decision{}, fmt.Errorf("decode remaining tokens: %w", err)
	}
	retryAfter, err := asInt64(values[2])
	if err != nil {
		return Decision{}, fmt.Errorf("decode retry delay: %w", err)
	}
	resetAfter, err := asInt64(values[3])
	if err != nil {
		return Decision{}, fmt.Errorf("decode reset delay: %w", err)
	}
	return Decision{
		Allowed:    allowed == 1,
		Limit:      l.policy.Capacity,
		Remaining:  remaining,
		RetryAfter: time.Duration(retryAfter) * time.Millisecond,
		ResetAfter: time.Duration(resetAfter) * time.Millisecond,
	}, nil
}

// Ping verifies that Redis is reachable.
func (l *RedisLimiter) Ping(ctx context.Context) error {
	return l.client.Ping(ctx).Err()
}

// Close releases the Redis client connection pool.
func (l *RedisLimiter) Close() error {
	return l.client.Close()
}

func (l *RedisLimiter) bucketKey(identity string) string {
	digest := sha256.Sum256([]byte(identity))
	return l.prefix + hex.EncodeToString(digest[:])
}

func asInt64(value interface{}) (int64, error) {
	switch v := value.(type) {
	case int64:
		return v, nil
	case string:
		return strconv.ParseInt(v, 10, 64)
	case []byte:
		return strconv.ParseInt(string(v), 10, 64)
	default:
		return 0, fmt.Errorf("unsupported type %T", value)
	}
}

func asFloat64(value interface{}) (float64, error) {
	switch v := value.(type) {
	case string:
		return strconv.ParseFloat(v, 64)
	case []byte:
		return strconv.ParseFloat(string(v), 64)
	case float64:
		return v, nil
	default:
		return 0, fmt.Errorf("unsupported type %T", value)
	}
}
