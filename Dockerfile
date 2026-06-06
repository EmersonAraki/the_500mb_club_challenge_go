# Build a fully static binary, ship it on scratch for a minimal image and RSS.
FROM golang:1.26-alpine AS build
WORKDIR /src
# No third-party modules: copy go.mod and sources, build offline.
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /api ./cmd/api

FROM scratch
COPY --from=build /api /api
USER 65534:65534
EXPOSE 8080
ENTRYPOINT ["/api"]
