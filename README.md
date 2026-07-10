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

| Endpoint | Purpose |
| --- | --- |
| `POST /v1/check` | Consume tokens and return an admission decision. |
| `GET /healthz` | Process liveness probe. |
| `GET /readyz` | Redis connectivity probe. |
| `GET /metrics` | Prometheus-compatible counters. |

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
