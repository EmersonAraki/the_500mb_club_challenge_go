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
  │ api1 │ │ api2 │ │ api3 │   Go, scratch image (~5.7 MB)
  └──┬───┘ └──┬───┘ └──┬───┘
     └────────┼────────┘
              ▼
          ┌───────┐
          │ redis │   one sorted set per device
          └───────┘
```

Container limits sum to exactly **2.00 CPU / 488 MB**, inside the ceiling
(see [`docker-compose.yml`](./docker-compose.yml)):

| Component | CPU | Memory |
|---|---:|---:|
| nginx LB | 0.35 | 32 MB |
| api ×3 | 0.45 each (1.35) | 120 MB each (360 MB) |
| redis | 0.30 | 96 MB |
| **total** | **2.00** | **488 MB** |

## Design choices and why they score well

The score weights five dimensions; efficiency (32%) and capacity (27%) together
decide **59%** of the race. Every choice below optimizes for those two:

- **Go on `scratch`.** A tiny static binary (~5.7 MB image) with low, predictable
  RSS and no runtime to host. Saturates the latency / resilience / stability
  targets while leading on efficiency and capacity.
- **Redis, one sorted set per device** (`t:{id}`, score = `ts`, member =
  binary-encoded point). Natively handles out-of-order ingestion, time-window
  range queries, and the "last 256 points" needed for anomaly detection.
- **Bounded memory under sustained ingestion.** Each device's history is capped to
  the newest `DEVICE_CAP` points (default 1024) via `ZREMRANGEBYRANK`, with a Redis
  `maxmemory` + `allkeys-lru` safety net. The trim is **amortized** — issued ~once
  per 16 writes, not every write. Redis is single-threaded, so halving the
  write-path command count directly raises sustainable RPS. Memory never grows
  without bound, which is decisive for efficiency.
- **Low GC pressure on the hot path.** Points are stored as a fixed 65-byte binary
  member (not JSON), validation is allocation-light, and metrics are atomic
  counters only (no per-request histograms). Strict round-robin punishes GC pauses,
  so keeping them small protects tail latency.
- **No third-party dependencies.** The Redis client, RESP2 codec, and Prometheus
  exposition are hand-rolled on the standard library. Smaller binary, smaller
  image, lower RSS, no supply chain.

## Benchmark

Run with the challenge's own harness — the k6 `steady` scenario (`test/test.js`)
and `capture-stats.sh` from [the challenge
repo](https://github.com/araki/the_500mb_club_challenge) — against the full
`docker compose` stack through the nginx load balancer. The steady scenario is a
`constant-arrival-rate` of **100 RPS for 1 minute** over **50 pre-seeded devices**,
with the realistic operation mix **60% single ingest / 10% batch / 20% range query
/ 10% anomaly**. Targets below are the scoring profile from
[`scoring.md`](./scoring.md).

> **Environment caveat.** These runs were on **Docker Desktop / macOS / Apple
> Silicon (ARM64)**, *not* the Raspberry Pi 5 target, and at the `test.js` rate of
> 100 RPS (the Pi harness gates efficiency at ~200 RPS). Container CPU and memory
> limits are enforced identically (2.00 CPU / 488 MB aggregate), so the figures
> validate correctness, the zero-error gate, latency under the SLOs, and footprint
> under the budget — but absolute throughput on a Pi 5 will be lower, and the
> capacity / spike / endurance scenarios run only on the Pi-Bench daemon.

### Latency (k6 steady, per operation)

| Operation | p95 | p99 | p99.9 | p99 target | Result |
|---|---:|---:|---:|---:|:--|
| `post` single ingest | 1.90 ms | 2.39 ms | 5.46 ms | 8 ms | ✅ 3.3× under |
| `batch` ingest | 4.38 ms | 5.56 ms | 23.4 ms | 25 ms | ✅ 4.5× under |
| `range` query | 2.61 ms | 3.00 ms | 3.73 ms | 15 ms | ✅ 5.0× under |
| `anomaly` z-score | 2.23 ms | 2.99 ms | 4.44 ms | 25 ms | ✅ 8.4× under |

- **`http_req_failed`: 0.00%** (0 / 6,050 requests) — clears the gate's < 0.5%
  error budget.
- Every operation sits well inside its "excellent" p99 SLO; the contract
  [`smoke.js`](https://github.com/araki/the_500mb_club_challenge) passed all 45
  assertions first.

### Efficiency / footprint (measured during the steady run)

| Metric | Observed | Target / budget | Result |
|---|---:|---:|:--|
| Aggregate RSS (p95) | **46.6 MiB** | 50–500 MB band; budget 500 MB | ✅ below the 50 MB top-clip floor |
| Aggregate CPU (mean) | **8.2%** | 40% (half the 2-core ceiling) | ✅ ~1/5 of target |
| API image size | **~5.7 MB** | — (informational) | scratch, single static binary |
| Cold start (`up` → stable `/readyz`) | **7.65 s** | — (informational) | — |

The aggregate is the sum across all five containers (3× api + nginx + redis). At
steady load the whole stack uses **under 10% of its memory budget** and **~4% of
its CPU budget** — the efficiency dimension (32% of the score) is where this
submission is built to win.

### Reproduce

```bash
# from this repo
docker compose -p bench up --build -d

# from a clone of the challenge repo (k6 + capture-stats.sh live there)
k6 run --env BASE_URL=http://localhost:8080 test/smoke.js   # contract check
scripts/capture-stats.sh -p bench -d 75 -o steady-stats.csv &
k6 run --env BASE_URL=http://localhost:8080 test/test.js    # steady 100 RPS, 1m
```

## Endpoints

All responses carry `X-Instance-Id`. See [`openapi.yaml`](./openapi.yaml) for the
full contract.

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
client. The live stack was also smoke-tested end-to-end through the load balancer.

## Configuration

| Env | Default | Purpose |
|---|---|---|
| `LISTEN_ADDR` | `:8080` | listen address |
| `REDIS_ADDR` | `127.0.0.1:6379` | Redis endpoint |
| `INSTANCE_ID` | hostname | reported in `X-Instance-Id` |
| `DEVICE_CAP` | `1024` | points retained per device |
| `REDIS_POOL` | `64` | connection pool size; pre-warmed at startup so the request path never pays a TCP handshake |
| `GOMEMLIMIT` | `110MiB` (compose) | Go soft memory limit |
| `GOMAXPROCS` | `1` (compose) | pinned to the ~0.45-core share to avoid oversubscription jitter |

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
