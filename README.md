# Redis Token Gate

[![CI](https://github.com/abhijaatx/redis-token-gate/actions/workflows/test.yml/badge.svg)](https://github.com/abhijaatx/redis-token-gate/actions/workflows/test.yml)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

A compact, production-minded HTTP rate-limiter built with **Go**, **Redis**,
and **Lua**. It applies a distributed token bucket atomically in Redis and
returns standard rate-limit headers so any application can make an admission
decision before doing expensive work.

The repository's three implementation languages all have a concrete job:

- **Go** — HTTP API, configuration, observability, graceful shutdown, and tests.
- **Lua** — single-operation Redis token-bucket mutation.
- **JavaScript** — concurrent Node.js load demonstration using native `fetch`.

Operational configuration (Docker/Compose and GitHub Actions) accompanies the
code but does not introduce application-language placeholders.

## Features

- Distributed, atomic token bucket shared by multiple service instances.
- Fractional refill rates, weighted request costs, and automatic inactive-key
  expiry.
- `RateLimit-Limit`, `RateLimit-Remaining`, `RateLimit-Reset`, and `Retry-After`
  response headers.
- Optional Bearer-token protection, hashed Redis bucket keys, bounded JSON
  requests, request IDs, JSON logs, safe HTTP timeouts, and graceful shutdown.
- Liveness, readiness, and Prometheus-compatible metrics endpoints.
- Unit tests, a Redis concurrency integration test, a hardened non-root image,
  Docker Compose setup, and GitHub Actions verification.

See the [architecture overview](docs/architecture.md) for the request path and
failure behavior.

## Why this service?

Use Redis Token Gate when several application instances need one shared
admission policy. The service makes a small, fast decision before downstream
work begins; it does not proxy, queue, or retry application traffic. Keeping
that boundary explicit makes it safe to place beside an API, worker fleet, or
edge proxy without coupling the limiter to application business logic.

## Quick start

Prerequisites: Docker Desktop with Compose v2.

```bash
docker compose up --build
```

In another terminal, make a decision:

```bash
curl -i http://localhost:8080/v1/check \
  --request POST \
  --header 'Content-Type: application/json' \
  --data '{"identity":"account-42","cost":1}'
```

The first request returns `200`. After the default capacity of ten requests is
spent, the service returns `429` with `Retry-After` and a JSON decision body.

Stop the stack with:

```bash
docker compose down
```

## Example load demonstration

Node.js 20 or later is required. With the Compose stack running:

```bash
node examples/load-demo.mjs
```

It concurrently sends 12 requests for the same identity, which demonstrates
that only ten are admitted under the default policy. Override
`REQUEST_COUNT`, `RATE_LIMIT_IDENTITY`, `RATE_LIMITER_URL`, or `API_TOKEN` in
the environment when needed.

## Request lifecycle

For each decision, the service:

1. Validates the identity and weighted cost at the HTTP boundary.
2. Hashes the identity before using it as a Redis key.
3. Runs the token refill and consume operation in one Redis Lua transaction.
4. Returns an admission decision and the timing headers a caller needs to
   decide whether to proceed or wait.

Because the mutation is atomic, concurrent service instances cannot spend the
same token twice during a race.

## API

`POST /v1/check`

```json
{
  "identity": "account-42",
  "cost": 1
}
```

`identity` is required and is stored as a SHA-256-derived Redis key. `cost`
defaults to `1` and must not exceed `MAX_COST`.

Successful response (`200`):

```json
{
  "allowed": true,
  "limit": 10,
  "remaining": 9,
  "retry_after_ms": 0,
  "reset_after_ms": 1000,
  "request_id": "b8b9a9c4a7b49c7bd0d41295"
}
```

When the bucket is empty, the same schema is returned with `allowed: false`
and status `429`. Always honor `Retry-After` before retrying.

If `API_TOKEN` is configured, add `Authorization: Bearer <token>` to decision
requests. The service deliberately returns `503` when Redis is unavailable so
it never silently changes to fail-open behavior.

### Authentication example

Set the token in the service environment and send it only over a trusted,
encrypted connection in production:

```bash
API_TOKEN='change-me' docker compose up --build
curl -i http://localhost:8080/v1/check \
  --request POST \
  --header 'Authorization: Bearer change-me' \
  --header 'Content-Type: application/json' \
  --data '{"identity":"account-42"}'
```

The token comparison is constant-time. Health, readiness, and metrics remain
unauthenticated so orchestrator probes can reach them; protect those routes at
the network boundary when they should not be publicly visible.

| Endpoint | Purpose |
| --- | --- |
| `POST /v1/check` | Consume tokens and return an admission decision. |
| `GET /healthz` | Process liveness probe. |
| `GET /readyz` | Redis connectivity probe. |
| `GET /metrics` | Prometheus-compatible counters. |

### Response semantics

| Status | Meaning | Caller action |
| --- | --- | --- |
| `200` | The requested cost was admitted. | Start the protected work. |
| `400` | The JSON request or policy bounds are invalid. | Fix the request; do not retry unchanged. |
| `401` | A configured API token is missing or incorrect. | Authenticate before retrying. |
| `429` | The bucket cannot afford the requested cost yet. | Wait for `Retry-After` seconds. |
| `503` | Redis could not complete the decision. | Apply the caller's outage policy and retry with backoff. |

Every decision includes `RateLimit-Limit`, `RateLimit-Remaining`, and
`RateLimit-Reset`. Denials additionally include `Retry-After`; all JSON
responses include a request ID for correlating client and server logs.

## Configuration

| Variable | Default | Description |
| --- | --- | --- |
| `LISTEN_ADDR` | `:8080` | HTTP bind address. |
| `REDIS_URL` | `redis://localhost:6379/0` | Redis connection URL. |
| `DEFAULT_CAPACITY` | `10` | Maximum tokens in each bucket. |
| `REFILL_PER_SECOND` | `1` | Token refill rate; fractional values are valid. |
| `MAX_COST` | capacity | Largest admitted request cost. |
| `KEY_PREFIX` | `rtg:bucket:` | Redis key namespace. |
| `REQUEST_TIMEOUT` | `2s` | Per-decision Redis timeout. |
| `API_TOKEN` | unset | Optional Bearer token for the decision endpoint. |

## Operational behavior

Buckets are namespaced by `KEY_PREFIX` and expire after enough idle time for a
full refill plus a safety buffer. That bounds inactive-key growth while
preserving state for active identities. The raw identity is never written to
Redis; only its SHA-256-derived key suffix is stored.

The service fails closed when Redis cannot answer a decision and returns
`503`. This avoids accidentally admitting traffic during an outage. If your
application needs a different outage policy, make that choice explicitly in
the caller rather than relying on an implicit fallback in the limiter.

### Policy examples

The refill rate is expressed in tokens per second, so common policies can be
read directly from environment variables:

```text
10 requests/second       DEFAULT_CAPACITY=10  REFILL_PER_SECOND=10
30 requests/minute       DEFAULT_CAPACITY=30  REFILL_PER_SECOND=0.5
100 tokens, max cost 25  DEFAULT_CAPACITY=100 REFILL_PER_SECOND=2 MAX_COST=25
```

Use a cost greater than one when one operation should consume more budget than
another; the request is denied until the full weighted cost is available.

## Run without Docker

Install Go 1.24+ and Redis 7+, start Redis locally, then run:

```bash
go run ./cmd/redis-token-gate
```

For a protected local endpoint:

```bash
API_TOKEN='change-me' go run ./cmd/redis-token-gate
curl -X POST http://localhost:8080/v1/check \
  -H 'Authorization: Bearer change-me' \
  -H 'Content-Type: application/json' \
  -d '{"identity":"account-42"}'
```

## Test and verify

Fast tests and static analysis do not require Redis:

```bash
go vet ./...
go test -race ./...
```

Run the real Redis atomicity test against the Compose Redis service:

```bash
docker compose up -d redis
REDIS_URL=redis://localhost:6379/15 go test -race -tags=integration ./internal/limiter
docker compose down
```

GitHub Actions runs both suites and builds the container on pushes and pull
requests.

## Project layout

```text
cmd/redis-token-gate/       application entry point
internal/config/            environment parsing and validation
internal/httpapi/           HTTP contract, headers, auth, and middleware
internal/limiter/           Go policy plus embedded atomic Redis Lua script
internal/metrics/           Prometheus exposition endpoint
internal/server/            HTTP hardening and graceful shutdown
examples/load-demo.mjs      concurrent client demonstration
docs/architecture.md        system and failure-mode design
```

## License

Licensed under the [Apache License 2.0](LICENSE).
