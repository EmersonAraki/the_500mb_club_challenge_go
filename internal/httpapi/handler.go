// Package httpapi wires the telemetry HTTP contract to a store, adding the
// instance header, panic recovery, and request metrics.
package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/araki/pibench/internal/cursor"
	"github.com/araki/pibench/internal/metrics"
	"github.com/araki/pibench/internal/store"
)

// Handler implements the full API as an http.Handler.
type Handler struct {
	store        store.Store
	cfg          Config
	mux          *http.ServeMux
	reg          *metrics.Registry
	reqs         *metrics.CounterVec
	points       *metrics.Counter
	readTimeouts *metrics.Counter
	statuses     map[int]*metrics.Sample
}

// statusCodes are the responses this API emits; their counter cells are cached
// at startup so the request path avoids a strconv + map lookup under lock.
var statusCodes = []int{200, 202, 400, 404, 413, 500, 503}

// New builds the API handler over the given store and metrics registry.
func New(s store.Store, reg *metrics.Registry, cfg Config) *Handler {
	h := &Handler{
		store:        s,
		cfg:          cfg,
		mux:          http.NewServeMux(),
		reg:          reg,
		reqs:         reg.NewCounterVec("pibench_http_requests_total", "HTTP requests by status code", "code"),
		points:       reg.NewCounter("pibench_points_ingested_total", "Telemetry points ingested"),
		readTimeouts: reg.NewCounter("pibench_redis_read_timeout_total", "Read requests aborted by the per-request Redis deadline"),
	}
	h.statuses = make(map[int]*metrics.Sample, len(statusCodes))
	for _, code := range statusCodes {
		h.statuses[code] = h.reqs.With(strconv.Itoa(code))
	}
	h.routes()
	return h
}

// readStatus maps a read-path store error to an HTTP status. A malformed cursor
// is the only user-driven error (400); every backend failure -- a hit read
// deadline, a connection error -- is a 503. A deadline hit also bumps the
// read-timeout counter so a stalled Redis is visible on /metrics.
func (h *Handler) readStatus(err error) int {
	if errors.Is(err, cursor.ErrInvalid) {
		return http.StatusBadRequest
	}
	if errors.Is(err, context.DeadlineExceeded) {
		h.readTimeouts.Inc()
	}
	return http.StatusServiceUnavailable
}

// countStatus records the response status, using the cached cell when possible.
func (h *Handler) countStatus(code int) {
	if s := h.statuses[code]; s != nil {
		s.Inc()
		return
	}
	h.reqs.With(strconv.Itoa(code)).Inc()
}

func (h *Handler) routes() {
	h.mux.HandleFunc("GET /healthz", h.healthz)
	h.mux.HandleFunc("GET /readyz", h.readyz)
	h.mux.HandleFunc("GET /metrics", h.metrics)
	h.mux.HandleFunc("POST /devices/{id}/telemetry", h.ingestSingle)
	h.mux.HandleFunc("GET /devices/{id}/telemetry", h.query)
	h.mux.HandleFunc("POST /devices/{id}/telemetry/batch", h.ingestBatch)
	h.mux.HandleFunc("GET /devices/{id}/anomaly", h.anomaly)
}

// ServeHTTP adds the instance header, recovers panics, and records the status.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Instance-Id", h.cfg.InstanceID)
	sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
	defer func() {
		if rec := recover(); rec != nil && !sw.wrote {
			sw.WriteHeader(http.StatusInternalServerError)
		}
		h.countStatus(sw.status)
	}()
	h.mux.ServeHTTP(sw, r)
}

// statusWriter captures the response status for metrics.
type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (s *statusWriter) WriteHeader(code int) {
	if !s.wrote {
		s.status = code
		s.wrote = true
		s.ResponseWriter.WriteHeader(code)
	}
}

func (s *statusWriter) Write(b []byte) (int, error) {
	if !s.wrote {
		s.WriteHeader(http.StatusOK)
	}
	return s.ResponseWriter.Write(b)
}
