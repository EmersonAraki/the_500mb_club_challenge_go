package httpapi

import (
	"net/http"
	"strconv"
)

const (
	defaultLimit = 100
	maxLimit     = 500
)

// rangeOut is the response for a time-window query. next_cursor is always
// present, null when there is no further page.
type rangeOut struct {
	Points     []pointOut `json:"points"`
	NextCursor *string    `json:"next_cursor"`
}

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
	out := rangeOut{Points: toOuts(pts)}
	if next != "" {
		out.NextCursor = &next
	}
	writeJSON(w, http.StatusOK, out)
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
