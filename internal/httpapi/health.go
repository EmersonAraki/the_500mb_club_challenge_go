package httpapi

import (
	"context"
	"io"
	"net/http"
)

// healthz reports process liveness without touching storage. The body is "ok"
// (the contract smoke test asserts the body contains it).
func (h *Handler) healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	io.WriteString(w, "ok")
}

// readyz reports readiness, contingent on storage being reachable. The store
// ping is bounded by the read deadline so a stalled Redis fails the probe fast
// instead of hanging the LB's readiness check.
func (h *Handler) readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.cfg.ReadTimeout)
	defer cancel()
	if err := h.store.Ping(ctx); err != nil {
		status(w, http.StatusServiceUnavailable)
		return
	}
	status(w, http.StatusOK)
}

// metrics exposes the Prometheus text exposition.
func (h *Handler) metrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	h.reg.Render(w)
}
