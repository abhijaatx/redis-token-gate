// Package limiter contains the token-bucket policy and Redis implementation.
package limiter

import (
	"context"
	"fmt"
	"math"
	"time"
)

// Policy controls how a bucket refills. Capacity and cost use whole tokens;
// refill rates may be fractional to support limits such as 30 requests/minute.
type Policy struct {
	Capacity        int64
	RefillPerSecond float64
	MaxCost         int64
}

const maxLuaExactInteger int64 = 1<<53 - 1

// Validate rejects unsafe or nonsensical policies before they reach Redis.
func (p Policy) Validate() error {
	if p.Capacity < 1 {
		return fmt.Errorf("capacity must be at least 1")
	}
	if p.Capacity > maxLuaExactInteger {
		return fmt.Errorf("capacity must be no greater than %d for Redis Lua precision", maxLuaExactInteger)
	}
	if p.RefillPerSecond <= 0 || math.IsInf(p.RefillPerSecond, 0) || math.IsNaN(p.RefillPerSecond) {
		return fmt.Errorf("refill rate must be a finite number greater than zero")
	}
	if p.MaxCost < 1 || p.MaxCost > p.Capacity {
		return fmt.Errorf("max cost must be between 1 and capacity")
	}
	if _, err := p.bucketTTLMilliseconds(); err != nil {
		return err
	}
	return nil
}

// BucketTTL is the time Redis should retain an inactive bucket. A small buffer
// avoids deleting a bucket just before its last fractional token is restored.
func (p Policy) BucketTTL() time.Duration {
	milliseconds, err := p.bucketTTLMilliseconds()
	if err != nil {
		return 0
	}
	return time.Duration(milliseconds) * time.Millisecond
}

func (p Policy) bucketTTLMilliseconds() (int64, error) {
	// Redis expires keys using signed 64-bit millisecond values. Validate the
	// computed TTL before converting from float64 so extreme configuration
	// values cannot wrap a time.Duration or produce an immediate expiry.
	milliseconds := math.Ceil(float64(p.Capacity)/p.RefillPerSecond*1000) + 1000
	maxMilliseconds := float64(int64(1<<63-1) / int64(time.Millisecond))
	if math.IsNaN(milliseconds) || math.IsInf(milliseconds, 0) || milliseconds < 1 || milliseconds > maxMilliseconds {
		return 0, fmt.Errorf("capacity/refill combination produces an unsupported bucket TTL")
	}
	return int64(milliseconds), nil
}

// Decision is the result of consuming tokens from a bucket.
type Decision struct {
	Allowed    bool
	Limit      int64
	Remaining  float64
	RetryAfter time.Duration
	ResetAfter time.Duration
}

// Service is the narrow interface used by the HTTP layer.
type Service interface {
	Allow(ctx context.Context, identity string, cost int64) (Decision, error)
	Ping(ctx context.Context) error
}
