package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/abhijaatx/redis-token-gate/internal/limiter"
	"github.com/abhijaatx/redis-token-gate/internal/metrics"
)

type fakeLimiter struct {
	decision limiter.Decision
	err      error
	pingErr  error
	identity string
	cost     int64
}

func (f *fakeLimiter) Allow(_ context.Context, identity string, cost int64) (limiter.Decision, error) {
	f.identity, f.cost = identity, cost
	return f.decision, f.err
}

func (f *fakeLimiter) Ping(context.Context) error { return f.pingErr }

func testHandler(fake *fakeLimiter, token string) http.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(fake, limiter.Policy{Capacity: 10, RefillPerSecond: 1, MaxCost: 5}, token, time.Second, &metrics.Metrics{}, logger)
}

func TestCheckAllowsAndSetsStandardHeaders(t *testing.T) {
	fake := &fakeLimiter{decision: limiter.Decision{Allowed: true, Limit: 10, Remaining: 4.8, ResetAfter: 5200 * time.Millisecond}}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/check", bytes.NewBufferString(`{"identity":"account-42","cost":2}`))
	testHandler(fake, "").ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if fake.identity != "account-42" || fake.cost != 2 {
		t.Fatalf("limiter called with %q, %d", fake.identity, fake.cost)
	}
	if got := recorder.Header().Get("RateLimit-Remaining"); got != "4" {
		t.Fatalf("RateLimit-Remaining = %q, want 4", got)
	}
	var response checkResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.Allowed || response.RequestID == "" {
		t.Fatalf("unexpected response: %#v", response)
	}
}

func TestCheckDenialIncludesRetryAfter(t *testing.T) {
	fake := &fakeLimiter{decision: limiter.Decision{Allowed: false, Limit: 2, Remaining: 0, RetryAfter: 200 * time.Millisecond, ResetAfter: 2 * time.Second}}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/check", bytes.NewBufferString(`{"identity":"account-42"}`))
	testHandler(fake, "").ServeHTTP(recorder, request)

	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", recorder.Code)
	}
	if got := recorder.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q, want 1", got)
	}
}

func TestCheckRejectsUnauthorizedRequest(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/check", bytes.NewBufferString(`{"identity":"account-42"}`))
	testHandler(&fakeLimiter{}, "secret").ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", recorder.Code)
	}
}

func TestCheckReturnsServiceUnavailableForLimiterFailure(t *testing.T) {
	fake := &fakeLimiter{err: errors.New("Redis timeout")}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/check", bytes.NewBufferString(`{"identity":"account-42"}`))
	testHandler(fake, "").ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
}

func TestReadyReflectsRedisStatus(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	testHandler(&fakeLimiter{pingErr: errors.New("down")}, "").ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
}
