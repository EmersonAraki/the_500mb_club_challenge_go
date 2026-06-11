#!/usr/bin/env bash
#
# "Check p99 on the Pi": bring up the full stack (Redis + 3 API instances +
# round-robin nginx LB) under the real 2 CPU / 500 MB budget, wait until it is
# healthy, then run the k6 steady load and print p99 per endpoint.
#
# The stack runs in the native linux/arm64 OrbStack environment -- the Pi-5
# proxy. It is not the real Pi (Apple M-core, not Cortex-A76), so trust the
# ratios and the per-endpoint comparison; absolute p99 on real hardware will be
# ~2.5-4x higher. But the LB, Redis RTT, and resource limits here are all real.
#
# Usage:
#   scripts/pi-bench.sh                 # build, smoke, steady load, tear down
#   scripts/pi-bench.sh --keep          # leave the stack running afterward
#   scripts/pi-bench.sh --no-smoke      # skip the functional smoke gate
#   K6_DIR=/path/to/test scripts/pi-bench.sh
#
# Env:
#   K6_DIR     dir holding smoke.js / test.js / lib (default: sibling challenge repo)
#   BASE_URL   override the target (default: http://localhost:8080)

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
K6_DIR="${K6_DIR:-$REPO_ROOT/../the_500mb_club_challenge/test}"
BASE_URL="${BASE_URL:-http://localhost:8080}"

KEEP=0
SMOKE=1
for arg in "$@"; do
  case "$arg" in
    --keep)     KEEP=1 ;;
    --no-smoke) SMOKE=0 ;;
    *) echo "unknown flag: $arg" >&2; exit 2 ;;
  esac
done

command -v k6 >/dev/null || { echo "k6 not found (brew install k6)" >&2; exit 1; }
[ -f "$K6_DIR/test.js" ] || { echo "k6 scripts not found in $K6_DIR (set K6_DIR)" >&2; exit 1; }

cd "$REPO_ROOT"

cleanup() {
  if [ "$KEEP" -eq 1 ]; then
    echo "stack left running (--keep); tear down with: docker compose down -v"
  else
    echo "tearing down stack..."
    docker compose down -v >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

echo "building + starting stack..."
docker compose down -v >/dev/null 2>&1 || true
docker compose up -d --build

# Wait for the LB to serve a healthy upstream (compose has no healthcheck gate).
echo -n "waiting for $BASE_URL/healthz "
for i in $(seq 1 60); do
  if [ "$(curl -s -o /dev/null -w '%{http_code}' "$BASE_URL/healthz" 2>/dev/null)" = "200" ]; then
    echo "-> ready"
    break
  fi
  if [ "$i" -eq 60 ]; then
    echo "-> TIMEOUT"; docker compose ps; exit 1
  fi
  echo -n "."; sleep 1
done

if [ "$SMOKE" -eq 1 ]; then
  echo "=== smoke (functional gate) ==="
  if ! k6 run --env BASE_URL="$BASE_URL" "$K6_DIR/smoke.js"; then
    echo "smoke failed; aborting before load test" >&2
    exit 1
  fi
fi

# gc_snapshot scrapes /metrics through the LB enough times to land on all three
# instances (round-robin), bucketing the Go runtime gauges by X-Instance-Id. The
# point is tail-latency attribution: gc_pause_seconds = time the world was
# stopped; sched_latency_seconds = time goroutines waited for a CPU slice. A
# rising sched_latency with flat gc_pause means CFS throttling under the 0.45-CPU
# quota, not GC -- that distinction is what these numbers are here to settle.
gc_snapshot() {
  local label="$1" seen="" hdrs body inst tries
  echo "=== Go runtime / GC gauges ($label) ==="
  for tries in $(seq 1 24); do
    hdrs="$(mktemp)"
    body="$(curl -s -D "$hdrs" "$BASE_URL/metrics" 2>/dev/null || true)"
    inst="$(grep -i '^x-instance-id:' "$hdrs" | tr -d '\r' | awk '{print $2}')"
    rm -f "$hdrs"
    [ -z "$inst" ] && continue
    case " $seen " in *" $inst "*) ;; *)
      seen="$seen $inst"
      echo "--- instance $inst ---"
      printf '%s\n' "$body" | grep -E \
        '^pibench_go_(gc_cycles_total|gc_pause_seconds_(total|max)|sched_latency_seconds_(total|max)|goroutines|heap_(objects|goal)_bytes) '
      ;;
    esac
    # Stop once we have all three instances.
    [ "$(echo "$seen" | wc -w)" -ge 3 ] && break
  done
}

echo "=== steady load: p99 per endpoint ==="
k6 run --env BASE_URL="$BASE_URL" "$K6_DIR/test.js"

# Cumulative pause/latency totals reflect the whole run; scrape after load.
gc_snapshot "after load"
