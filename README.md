# Pi-Bench Telemetry API — 500 MB Club submission

A telemetry ingestion and query service for the
[500 MB Club benchmark](./participating.md): a Raspberry Pi 5 (ARM64) with an
inviolable aggregate ceiling of **2 CPU / 500 MB**. It implements the contract in
[`openapi.yaml`](./openapi.yaml) (see also [`api.md`](./api.md)) with **zero
third-party dependencies** — just Go's standard library on a `scratch` image.

The service ingests device telemetry points (location + acceleration + optional
battery), serves time-window queries with cursor pagination, and flags anomalies
via a z-score over each device's recent history.

## Architecture

```
            :8080
        ┌────────────┐
client ─▶│  nginx LB  │  strict round-robin (no adaptive heuristics)
        └─────┬──────┘
     ┌────────┼────────┐
     ▼        ▼        ▼
  ┌──────┐ ┌──────┐ ┌──────┐
  │ api1 │ │ api2 │ │ api3 │   Go, scratch image
  └──┬───┘ └──┬───┘ └──┬───┘
     └────────┼────────┘
              ▼
          ┌───────┐
          │ redis │   one sorted set per device
          └───────┘
```

Container limits sum within the **2 CPU / 500 MB** ceiling
(see [`docker-compose.yml`](./docker-compose.yml)):

| Component | CPU | Memory |
|---|---:|---:|
| nginx LB | 0.48 | 32 MB |
| api ×3 | 0.37 each (1.11) | 120 MB each (360 MB) |
| redis | 0.40 | 96 MB |
| **total** | **1.99** | **488 MB** |

The split deliberately favors the two **shared funnels** — the single nginx and
the single Redis, through which all traffic passes — over the three API instances,
which parallelize and have the most headroom. Both are single-threaded, so
starving either collapses throughput for everyone.

## Key design decisions

- **Go on a `scratch` image, zero third-party dependencies.** The Redis client,
  RESP2 codec, and Prometheus exposition are all hand-rolled on the standard
  library and shipped as a single static binary on `scratch`. The result is a tiny
  image, low and predictable RSS, no runtime to host, and no supply chain.
- **Redis, one sorted set per device** (`t:{id}`, score = `ts`, member =
  binary-encoded point). A sorted set natively handles out-of-order ingestion,
  time-window range queries, and the "last 256 points" the anomaly check needs.
- **Fixed 65-byte binary member, not JSON.** Each point is encoded into a
  fixed-width binary member — a big-endian `ts` + atomic `seq` prefix (stable tie
  ordering and uniqueness) followed by the float fields. Storing binary instead of
  JSON keeps the hot path allocation-light and the stored size small.
- **Bounded memory under sustained ingestion.** Each device's history is capped to
  the newest `DEVICE_CAP` points (default 1024) via `ZREMRANGEBYRANK`, backed by a
  Redis `maxmemory` + `allkeys-lru` safety net. The trim is **amortized** — issued
  ~once per 16 writes, not on every write. Redis is single-threaded, so halving the
  write-path command count directly raises sustainable throughput, and memory never
  grows without bound.
- **Synchronous writes — a `202` means persisted.** Ingestion writes straight to
  Redis before returning `202`; a Redis failure surfaces as a `503`, never as
  silent data loss. There is no async buffer or coalescing layer, so no
  read-after-write race and no drop-on-overflow window.
- **Fail-fast reads.** Every read wraps its store call in a per-request deadline
  (`READ_TIMEOUT_MS`, default 250 ms): a stalled Redis becomes an immediate `503`
  plus a counter, instead of holding a goroutine and the load-balancer connection
  for the full server write timeout — which under strict round-robin would poison
  every instance's tail.
- **Strict round-robin load balancing.** The nginx LB uses fixed round-robin with
  no adaptive heuristics, by design: it exposes per-instance tail-latency variance
  (e.g. a GC pause on one API) rather than hiding it behind least-conn balancing.
- **One `P` per instance (`GOMAXPROCS=1`).** Each API is pinned to a single
  scheduler thread to match its sub-core CPU quota, avoiding the goroutine-migration
  and GC-assist jitter of an oversubscribed runtime under a fractional-core limit.
- **Low GC pressure on the hot path.** Binary members (not JSON), allocation-light
  validation, and atomic-counter metrics (no per-request histograms) keep
  stop-the-world pauses small — which strict round-robin would otherwise surface
  directly in tail latency.

## Endpoints

All responses carry `X-Instance-Id`. See [`openapi.yaml`](./openapi.yaml) for the
full contract.

| Method | Path | Notes |
|---|---|---|
| GET | `/healthz` | liveness, no storage access (answered locally by the LB) |
| GET | `/readyz` | 200 when Redis reachable within the read deadline, else 503 |
| GET | `/metrics` | Prometheus text exposition |
| POST | `/devices/{id}/telemetry` | single ingest → 202 |
| POST | `/devices/{id}/telemetry/batch` | 1–100 points → 202 `{accepted}` |
| GET | `/devices/{id}/telemetry` | time-window query, cursor pagination |
| GET | `/devices/{id}/anomaly` | z-score over last 256 points (no cache) |

## Contract decisions worth flagging

- **Anomaly threshold uses `|z| > 3`** (absolute value), per `openapi.yaml`, which
  declares itself the binding contract ("conform exactly to this contract").
  `api.md` phrases it as `z_score > 3`; where the two disagree, the OpenAPI wins.
- **Standard deviation is population** (÷ N) — the deviation of the 256-point
  window, not the sample estimator (÷ N−1).
- **`battery` is optional** and emitted only when present. 0.0 is a valid value, so
  it is not elided by `omitempty` alone — a pointer field is used.
- **Pagination cursors are opaque** (base64url of `ts:skip`); `skip` carries the
  count of already-returned points at the boundary `ts`, so pages stay stable even
  when multiple points share a timestamp.
- **No `404` on range queries** — an empty window returns `200` with `points: []`,
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
go test ./...                                  # unit + in-memory store + handler tests
go test -tags=integration ./internal/store/    # against a real Redis (REDIS_ADDR)
```

The in-memory store mirrors the Redis ZSET semantics, so handler behavior is
covered without a broker; the `integration`-tagged tests exercise the real Redis
client.

## Configuration

| Env | Default | Purpose |
|---|---|---|
| `LISTEN_ADDR` | `:8080` | listen address |
| `REDIS_ADDR` | `127.0.0.1:6379` | Redis endpoint |
| `INSTANCE_ID` | hostname | reported in `X-Instance-Id` |
| `DEVICE_CAP` | `1024` | points retained per device |
| `READ_TIMEOUT_MS` | `250` | per-request Redis deadline on read paths; a stall becomes a fast `503` plus `pibench_redis_read_timeout_total`, not a held goroutine |
| `REDIS_POOL` | `64` | connection pool size; pre-warmed at startup so the request path never pays a TCP handshake |
| `GOMEMLIMIT` | `110MiB` (compose) | Go soft memory limit |
| `GOMAXPROCS` | `1` (compose) | one P per instance to avoid oversubscription jitter under the sub-core quota |

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
stress/              in-process Go load harness (dev only, not shipped)
```
