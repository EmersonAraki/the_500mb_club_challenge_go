# stress — in-process load harness

A dependency-free, pure-Go load generator for the telemetry API. It mirrors the
official k6 scenarios so the inner experiment loop (e.g. the E1 memory ladder)
runs anywhere the Go toolchain exists — including directly on the Pi, with no k6
install.

**This is not the official harness.** Final, comparable numbers must still come
from the challenge's k6 `test.js` + `capture-stats.sh`. This is for fast local
iteration only. It never ships in the image (the Dockerfile copies only `cmd/`
and `internal/`, and `stress` is in `.dockerignore`).

## Scenarios

All share `-url` (or `BASE_URL`), `-devices`, `-workers`, `-batch`, and use the
scoring mix (60% post / 10% batch / 20% range / 10% anomaly). Devices are
pre-seeded so range/anomaly read real data from the first request.

```bash
# constant 200 RPS for 60s — efficiency + tail latency + the gate
go run ./stress/cmd/steady   -rate 200 -dur 60s

# baseline -> 2x burst -> recovery — where a starved LB / GC pause shows up
go run ./stress/cmd/spike    -base 200 -spike 400 -seg 20s

# step through rates and print the knee (highest failure-free, drop-free step)
go run ./stress/cmd/capacity -rates 200,400,600,800,1000,1200 -dur 30s

# long hold — memory drift, GC sawtooth, OOM (validates the E1 ladder)
go run ./stress/cmd/endurance -rate 200 -dur 30m
```

Each prints a per-op table (p50/p95/p99/p99.9/max + error%) plus dropped
iterations. `capacity` additionally reports the knee.

## Layout

```
stress/load   driver, stats/percentiles, weighted mix, payload gen, CLI flags
stress/cmd/*  one thin main per scenario
```
