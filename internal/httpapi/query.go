package httpapi

import (
	"net/http"
	"strconv"
)

const (
	defaultLimit = 100
	maxLimit     = 500
)

// query handles GET /devices/{id}/telemetry.
func (h *Handler) query(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validID(id) {
		status(w, http.StatusBadRequest)
		return
	}
	q := r.URL.Query()
	from, ok1 := parseInt64(q.Get("from"))
	to, ok2 := parseInt64(q.Get("to"))
	if !ok1 || !ok2 || from > to {
		status(w, http.StatusBadRequest)
		return
	}
	limit := defaultLimit
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > maxLimit {
			status(w, http.StatusBadRequest)
			return
		}
		limit = n
	}

	pts, next, err := h.store.Range(r.Context(), id, from, to, limit, q.Get("cursor"))
	if err != nil {
		// An invalid cursor is the only user-driven error surfaced here.
		status(w, http.StatusBadRequest)
		return
	}
	// Encode straight to bytes (reflection-free) and write once. ~96 bytes/point
	// covers the fixed fields plus battery without regrowing the buffer.
	buf := appendRangeJSON(make([]byte, 0, len(pts)*96+32), pts, next)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf)
}

func parseInt64(s string) (int64, bool) {
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}
