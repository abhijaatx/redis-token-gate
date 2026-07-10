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

// Validate rejects unsafe or nonsensical policies before they reach Redis.
func (p Policy) Validate() error {
	if p.Capacity < 1 {
		return fmt.Errorf("capacity must be at least 1")
	}
	if p.RefillPerSecond <= 0 || math.IsInf(p.RefillPerSecond, 0) || math.IsNaN(p.RefillPerSecond) {
		return fmt.Errorf("refill rate must be a finite number greater than zero")
	}
	if p.MaxCost < 1 || p.MaxCost > p.Capacity {
		return fmt.Errorf("max cost must be between 1 and capacity")
	}
	return nil
}

// BucketTTL is the time Redis should retain an inactive bucket. A small buffer
// avoids deleting a bucket just before its last fractional token is restored.
func (p Policy) BucketTTL() time.Duration {
	return time.Duration(math.Ceil(float64(p.Capacity)/p.RefillPerSecond*1000)+1000) * time.Millisecond
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
