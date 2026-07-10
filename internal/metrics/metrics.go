// Package metrics exposes a deliberately small Prometheus text endpoint without
// requiring a metrics SDK for this single-purpose service.
package metrics

import (
	"fmt"
	"net/http"
	"sync/atomic"
)

// Metrics holds concurrency-safe counters for the decision endpoint.
type Metrics struct {
	allowed     atomic.Uint64
	denied      atomic.Uint64
	redisErrors atomic.Uint64
}

func (m *Metrics) RecordAllowed()    { m.allowed.Add(1) }
func (m *Metrics) RecordDenied()     { m.denied.Add(1) }
func (m *Metrics) RecordRedisError() { m.redisErrors.Add(1) }

// Handler emits the Prometheus exposition format.
func (m *Metrics) Handler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	fmt.Fprintf(w, "# HELP redis_token_gate_decisions_total Rate-limit decisions by outcome.\n")
	fmt.Fprintf(w, "# TYPE redis_token_gate_decisions_total counter\n")
	fmt.Fprintf(w, "redis_token_gate_decisions_total{outcome=\"allowed\"} %d\n", m.allowed.Load())
	fmt.Fprintf(w, "redis_token_gate_decisions_total{outcome=\"denied\"} %d\n", m.denied.Load())
	fmt.Fprintf(w, "# HELP redis_token_gate_redis_errors_total Redis errors while deciding a request.\n")
	fmt.Fprintf(w, "# TYPE redis_token_gate_redis_errors_total counter\n")
	fmt.Fprintf(w, "redis_token_gate_redis_errors_total %d\n", m.redisErrors.Load())
	fmt.Fprintf(w, "# HELP redis_token_gate_up Process health indicator.\n")
	fmt.Fprintf(w, "# TYPE redis_token_gate_up gauge\n")
	fmt.Fprint(w, "redis_token_gate_up 1\n")
}
