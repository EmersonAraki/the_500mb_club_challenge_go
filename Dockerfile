# syntax=docker/dockerfile:1
# Build a fully static binary, ship it on scratch for a minimal image and RSS.
#
# The builder runs natively on the BUILDPLATFORM and cross-compiles to TARGETARCH
# via the Go toolchain (CGO-free, so GOARCH=arm64 just works). This produces the
# Pi 5 arm64 image at native speed instead of ~10x slower QEMU emulation; only
# the final scratch layer is platform-tagged.
ARG BUILDPLATFORM
FROM --platform=${BUILDPLATFORM:-linux/amd64} golang:1.26-alpine AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
# No third-party modules: copy go.mod and sources, build offline (no go.sum).
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-arm64} \
    go build -trimpath -buildvcs=false -ldflags "-s -w" -o /api ./cmd/api

FROM scratch
COPY --from=build /api /api
USER 65534:65534
EXPOSE 8080
ENTRYPOINT ["/api"]
