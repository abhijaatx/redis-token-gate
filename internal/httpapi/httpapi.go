// Package httpapi exposes the rate-limiter's HTTP contract.
package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/abhijaatx/redis-token-gate/internal/limiter"
	"github.com/abhijaatx/redis-token-gate/internal/metrics"
)

const maxRequestBytes = 1 << 20

// App is the HTTP application and its dependencies.
type App struct {
	limiter        limiter.Service
	policy         limiter.Policy
	apiToken       string
	requestTimeout time.Duration
	metrics        *metrics.Metrics
	logger         *slog.Logger
}

// New constructs a standard-library HTTP handler with safe timeouts,
// request IDs, panic recovery, and structured access logging.
func New(l limiter.Service, policy limiter.Policy, apiToken string, requestTimeout time.Duration, meter *metrics.Metrics, logger *slog.Logger) http.Handler {
	app := &App{
		limiter:        l,
		policy:         policy,
		apiToken:       apiToken,
		requestTimeout: requestTimeout,
		metrics:        meter,
		logger:         logger,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", app.health)
	mux.HandleFunc("GET /readyz", app.ready)
	mux.HandleFunc("GET /metrics", meter.Handler)
	mux.HandleFunc("POST /v1/check", app.check)
	return app.withMiddleware(mux)
}

type checkRequest struct {
	Identity string `json:"identity"`
	Cost     int64  `json:"cost"`
}

type checkResponse struct {
	Allowed      bool   `json:"allowed"`
	Limit        int64  `json:"limit"`
	Remaining    int64  `json:"remaining"`
	RetryAfterMS int64  `json:"retry_after_ms"`
	ResetAfterMS int64  `json:"reset_after_ms"`
	RequestID    string `json:"request_id"`
}

type errorResponse struct {
	Error     string `json:"error"`
	RequestID string `json:"request_id"`
}

func (a *App) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "request_id": requestID(r.Context())})
}

func (a *App) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), a.requestTimeout)
	defer cancel()
	if err := a.limiter.Ping(ctx); err != nil {
		a.metrics.RecordRedisError()
		writeError(w, http.StatusServiceUnavailable, "redis is unavailable", requestID(r.Context()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready", "request_id": requestID(r.Context())})
}

func (a *App) check(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(r) {
		w.Header().Set("WWW-Authenticate", "Bearer")
		writeError(w, http.StatusUnauthorized, "unauthorized", requestID(r.Context()))
		return
	}

	var input checkRequest
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "body must be valid JSON", requestID(r.Context()))
		return
	}
	if err := rejectTrailingJSON(decoder); err != nil {
		writeError(w, http.StatusBadRequest, "body must contain one JSON object", requestID(r.Context()))
		return
	}
	input.Identity = strings.TrimSpace(input.Identity)
	if input.Identity == "" || len(input.Identity) > 128 {
		writeError(w, http.StatusBadRequest, "identity must contain 1 to 128 characters", requestID(r.Context()))
		return
	}
	if input.Cost == 0 {
		input.Cost = 1
	}
	if input.Cost < 1 || input.Cost > a.policy.MaxCost {
		writeError(w, http.StatusBadRequest, "cost is outside the configured range", requestID(r.Context()))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), a.requestTimeout)
	defer cancel()
	decision, err := a.limiter.Allow(ctx, input.Identity, input.Cost)
	if err != nil {
		a.metrics.RecordRedisError()
		a.logger.Error("rate-limit decision failed", "error", err, "request_id", requestID(r.Context()))
		writeError(w, http.StatusServiceUnavailable, "rate limiter is temporarily unavailable", requestID(r.Context()))
		return
	}

	setRateLimitHeaders(w, decision)
	response := checkResponse{
		Allowed:      decision.Allowed,
		Limit:        decision.Limit,
		Remaining:    int64(math.Floor(decision.Remaining)),
		RetryAfterMS: decision.RetryAfter.Milliseconds(),
		ResetAfterMS: decision.ResetAfter.Milliseconds(),
		RequestID:    requestID(r.Context()),
	}
	if decision.Allowed {
		a.metrics.RecordAllowed()
		writeJSON(w, http.StatusOK, response)
		return
	}
	a.metrics.RecordDenied()
	writeJSON(w, http.StatusTooManyRequests, response)
}

func (a *App) authorized(r *http.Request) bool {
	if a.apiToken == "" {
		return true
	}
	const prefix = "Bearer "
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	provided := strings.TrimPrefix(header, prefix)
	return subtle.ConstantTimeCompare([]byte(provided), []byte(a.apiToken)) == 1
}

func setRateLimitHeaders(w http.ResponseWriter, decision limiter.Decision) {
	w.Header().Set("RateLimit-Limit", strconvFormat(decision.Limit))
	w.Header().Set("RateLimit-Remaining", strconvFormat(int64(math.Floor(decision.Remaining))))
	w.Header().Set("RateLimit-Reset", strconvFormat(secondsCeil(decision.ResetAfter)))
	if !decision.Allowed {
		w.Header().Set("Retry-After", strconvFormat(max(1, secondsCeil(decision.RetryAfter))))
	}
}

func secondsCeil(duration time.Duration) int64 {
	return int64(math.Ceil(duration.Seconds()))
}

func strconvFormat(value int64) string {
	return strconv.FormatInt(value, 10)
}

func rejectTrailingJSON(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("found an additional JSON value")
	}
	return err
}

func writeError(w http.ResponseWriter, status int, message, id string) {
	writeJSON(w, status, errorResponse{Error: message, RequestID: id})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

type requestIDKey struct{}

func requestID(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey{}).(string)
	return value
}

func (a *App) withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := newRequestID()
		w.Header().Set("X-Request-ID", id)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		started := time.Now()
		defer func() {
			if recovered := recover(); recovered != nil {
				a.logger.Error("panic recovered", "request_id", id, "panic", recovered)
				writeError(w, http.StatusInternalServerError, "internal server error", id)
			}
		}()
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id)))
		a.logger.Info("request completed", "method", r.Method, "path", r.URL.Path, "duration_ms", time.Since(started).Milliseconds(), "request_id", id)
	})
}

func newRequestID() string {
	bytes := make([]byte, 12)
	if _, err := rand.Read(bytes); err == nil {
		return hex.EncodeToString(bytes)
	}
	return strconvFormat(time.Now().UnixNano())
}
