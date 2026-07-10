package limiter

import (
	"strings"
	"testing"
	"time"
)

func TestPolicyValidate(t *testing.T) {
	tests := []struct {
		name    string
		policy  Policy
		wantErr string
	}{
		{"valid", Policy{Capacity: 10, RefillPerSecond: 1, MaxCost: 10}, ""},
		{"zero capacity", Policy{Capacity: 0, RefillPerSecond: 1, MaxCost: 1}, "capacity"},
		{"zero rate", Policy{Capacity: 1, RefillPerSecond: 0, MaxCost: 1}, "refill"},
		{"cost above capacity", Policy{Capacity: 1, RefillPerSecond: 1, MaxCost: 2}, "max cost"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.policy.Validate()
			if test.wantErr == "" && err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if test.wantErr != "" && (err == nil || !strings.Contains(err.Error(), test.wantErr)) {
				t.Fatalf("Validate() error = %v, want substring %q", err, test.wantErr)
			}
		})
	}
}

func TestBucketTTLIncludesRefillTimeAndBuffer(t *testing.T) {
	policy := Policy{Capacity: 10, RefillPerSecond: 2, MaxCost: 10}
	if got, want := policy.BucketTTL(), 6*time.Second; got != want {
		t.Fatalf("BucketTTL() = %s, want %s", got, want)
	}
}

func TestDecoders(t *testing.T) {
	if got, err := asInt64("42"); err != nil || got != 42 {
		t.Fatalf("asInt64() = %d, %v", got, err)
	}
	if got, err := asFloat64("3.25"); err != nil || got != 3.25 {
		t.Fatalf("asFloat64() = %f, %v", got, err)
	}
}
