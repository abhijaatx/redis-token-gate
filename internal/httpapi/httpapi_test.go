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

func TestCheckAcceptsBearerToken(t *testing.T) {
	fake := &fakeLimiter{decision: limiter.Decision{Allowed: true, Limit: 10, Remaining: 9}}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/check", bytes.NewBufferString(`{"identity":"account-42"}`))
	request.Header.Set("Authorization", "Bearer secret")
	testHandler(fake, "secret").ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || fake.identity != "account-42" || fake.cost != 1 {
		t.Fatalf("status = %d, identity = %q, cost = %d", recorder.Code, fake.identity, fake.cost)
	}
}

func TestCheckRejectsMalformedOrUnknownJSON(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "malformed", body: `{"identity":`},
		{name: "unknown field", body: `{"identity":"account-42","extra":true}`},
		{name: "trailing value", body: `{"identity":"account-42"}{}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fake := &fakeLimiter{}
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/v1/check", bytes.NewBufferString(test.body))
			testHandler(fake, "").ServeHTTP(recorder, request)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", recorder.Code)
			}
			if fake.identity != "" {
				t.Fatalf("limiter should not run for invalid JSON, got identity %q", fake.identity)
			}
		})
	}
}

func TestCheckRejectsInvalidInput(t *testing.T) {
	tests := []string{
		`{"identity":""}`,
		`{"identity":"account-42","cost":0,"extra":true}`,
		`{"identity":"account-42","cost":6}`,
	}
	for _, body := range tests {
		t.Run(body, func(t *testing.T) {
			fake := &fakeLimiter{}
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/v1/check", bytes.NewBufferString(body))
			testHandler(fake, "").ServeHTTP(recorder, request)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", recorder.Code)
			}
			if fake.identity != "" {
				t.Fatalf("limiter should not run for invalid input, got identity %q", fake.identity)
			}
		})
	}
}

func TestHealthAndMetricsExposeProbeResponses(t *testing.T) {
	fake := &fakeLimiter{}
	handler := testHandler(fake, "")
	for _, path := range []string{"/healthz", "/metrics"} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, path, nil)
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200", path, recorder.Code)
		}
		if recorder.Header().Get("X-Request-ID") == "" {
			t.Fatalf("%s did not include request ID", path)
		}
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
