#!/usr/bin/env bash
#
# Source-of-truth runner for codec / serialization micro-benchmarks.
#
# The real Raspberry Pi 5 cannot run arbitrary Go benchmarks (it only runs the
# merged image's k6 bench via the GitHub test/<id> issue flow). The agreed proxy
# is a native linux/arm64 container: real ARM64 Linux + the Linux Go runtime,
# the same OS/arch the submission deploys to.
#
# Caveats: CPU is an Apple M-core, not the Pi's Cortex-A76 (~2.5-4x faster in
# absolute terms; trust ratios, not absolutes). The VM clock quantizes to ~41ns,
# so sub-42ns ops read as 42n; use the mean column for true sub-floor cost.
#
# Usage:
#   scripts/codec-bench.sh            # run the p99 codec table + layout guards
#   scripts/codec-bench.sh -run TestCodecP99   # pass extra `go test` flags

set -euo pipefail

# Pin the toolchain image so results are comparable across runs.
IMAGE="golang:1.26-alpine"
PLATFORM="linux/arm64"

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Default: both the percentile table and the byte-layout regression guards.
ARGS=("$@")
if [ ${#ARGS[@]} -eq 0 ]; then
  ARGS=(-run "TestCodecP99|TestLayout" -v)
fi

exec docker run --rm \
  --platform "$PLATFORM" \
  -v "$REPO_ROOT":/src \
  -w /src \
  -e GOFLAGS=-count=1 \
  "$IMAGE" \
  sh -c 'echo "host: $(uname -m)  cpus: $(nproc)  $(go version | awk "{print \$3}")"; exec go test ./internal/model '"$(printf '%q ' "${ARGS[@]}")"
