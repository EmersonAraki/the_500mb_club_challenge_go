package httpapi

import (
	"io"
	"net/http"
)

// healthz reports process liveness without touching storage. The body is "ok"
// (the contract smoke test asserts the body contains it).
func (h *Handler) healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	io.WriteString(w, "ok")
}

// readyz reports readiness, contingent on storage being reachable.
func (h *Handler) readyz(w http.ResponseWriter, r *http.Request) {
	if err := h.store.Ping(r.Context()); err != nil {
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
