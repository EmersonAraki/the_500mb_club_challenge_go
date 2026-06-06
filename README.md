# Pi-Bench Telemetry API — 500 MB Club submission

A telemetry ingestion + query service implementing [`openapi.yaml`](./openapi.yaml)
and [`api.md`](./api.md), built for the 500 MB Club benchmark (2 CPU / 500 MB
aggregate, Raspberry Pi 5, ARM64).

## Stack

```
            :8080
        ┌────────────┐
client ─▶│  nginx LB  │  strict round-robin
        └─────┬──────┘
     ┌────────┼────────┐
     ▼        ▼        ▼
  ┌──────┐ ┌──────┐ ┌──────┐
  │ api1 │ │ api2 │ │ api3 │   Go, scratch image (~5.7 MB)
  └──┬───┘ └──┬───┘ └──┬───┘
     └────────┼────────┘
              ▼
          ┌───────┐
          │ redis │   one sorted set per device
          └───────┘
```

- **Language: Go** — saturates the latency / resilience / stability targets while
  scoring strongly on the two dimensions that decide the race (efficiency 32% +
  capacity 27% = 59% of the weight). Ships a tiny static binary on `scratch`.
- **Storage: Redis** — a sorted set per device (`t:{id}`, score = `ts`, member =
  the binary-encoded point). Natively handles out-of-order ingestion, time-window
  range queries, and "last 256 points" for anomaly.
- **Zero third-party dependencies** — the Redis client, RESP codec, and Prometheus
  exposition are hand-rolled on the standard library. Smaller binary, smaller
  image, lower RSS, no supply chain.

## Why these choices score well

- **Idle footprint ~17 MiB** across all 5 containers (each Go instance ~1.6 MiB
  RSS). Container limits sum to **1.95 CPU / 484 MB**, inside the inviolable
  ceiling (see [`docker-compose.yml`](./docker-compose.yml)).
- **Bounded memory under sustained ingestion.** Each device's history is capped to
  the newest `DEVICE_CAP` (default 1024) points via `ZREMRANGEBYRANK`, with a Redis
  `maxmemory` + `allkeys-lru` safety net. The trim is **amortized** (issued ~once per
  16 writes, not every write): Redis is single-threaded, so halving the write-path
  command count directly raises sustainable RPS. A device briefly holds a small slack
  above the cap between trims — memory still does not grow without bound, the decisive
  factor for the efficiency dimension.
- **Low GC pressure on the hot path:** points are stored as a fixed 65-byte binary
  member (not JSON), validation is allocation-light, and metrics are atomic counters
  only (no per-request histograms). Strict round-robin punishes GC pauses, so this
  matters for tail latency.

## Endpoints

All responses carry `X-Instance-Id`. See the contract for full details.

| Method | Path | Notes |
|---|---|---|
| GET | `/healthz` | liveness, no storage access |
| GET | `/readyz` | 200 when Redis reachable, else 503 |
| GET | `/metrics` | Prometheus text exposition |
| POST | `/devices/{id}/telemetry` | single ingest → 202 |
| POST | `/devices/{id}/telemetry/batch` | 1–100 points → 202 `{accepted}` |
| GET | `/devices/{id}/telemetry` | time-window query, cursor pagination |
| GET | `/devices/{id}/anomaly` | z-score over last 256 points (no cache) |

## Contract decisions worth flagging

- **Anomaly threshold uses `|z| > 3`** (absolute value), per `openapi.yaml`, which
  declares itself the binding contract ("conform exactly to this contract").
  `api.md` phrases it as `z_score > 3`; where the two disagree, the OpenAPI wins.
- **Standard deviation is population** (÷ N), i.e. the standard deviation of the
  256-point window, not the sample estimator (÷ N−1).
- **`battery` is optional** and emitted in responses only when present (0.0 is a
  valid value, so it is not elided by omitempty alone — a pointer field is used).
- **Pagination cursors are opaque** (base64url of `ts:skip`); `skip` carries the
  count of already-returned points at the boundary `ts`, so pages remain stable
  even when multiple points share a timestamp.
- **Range `404`** is not used: an empty window returns `200` with `points: []`,
  which the contract explicitly permits.

## Run

```bash
docker compose up --build -d
curl -s localhost:8080/readyz
curl -s -X POST -H 'content-type: application/json' \
  -d '{"ts":1715800000000,"lat":-23.5,"lon":-46.6,"battery":0.8,"ax":0.1,"ay":-0.04,"az":9.81}' \
  localhost:8080/devices/dev1/telemetry
```

## Tests

```bash
go test ./...                       # unit + in-memory store + handler tests
go test -tags=integration ./internal/store/   # against a real Redis (REDIS_ADDR)
```

The in-memory store mirrors the Redis ZSET semantics, so handler behavior is
covered without a broker; the `integration`-tagged tests exercise the real Redis
client (the live stack was also smoke-tested end-to-end through the load balancer).

## Configuration

| Env | Default | Purpose |
|---|---|---|
| `LISTEN_ADDR` | `:8080` | listen address |
| `REDIS_ADDR` | `127.0.0.1:6379` | Redis endpoint |
| `INSTANCE_ID` | hostname | reported in `X-Instance-Id` |
| `DEVICE_CAP` | `1024` | points retained per device |
| `REDIS_POOL` | `64` | connection pool size |
| `GOMEMLIMIT` | `110MiB` (compose) | Go soft memory limit |

## Layout

```
cmd/api              entrypoint: server + graceful SIGTERM drain (10s)
internal/model       telemetry point: validation + 65-byte binary codec
internal/anomaly     z-score over the magnitude window
internal/cursor      opaque keyset pagination cursors
internal/store       Store interface, in-memory + Redis backends, pagination
internal/resp        minimal RESP2 codec (no deps)
internal/metrics     atomic-counter Prometheus exposition
internal/httpapi     routing, middleware, handlers
internal/config      env configuration
deploy/nginx.conf    round-robin load balancer
```
