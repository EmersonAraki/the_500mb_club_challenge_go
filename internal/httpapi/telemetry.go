package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/araki/pibench/internal/model"
)

// ingestSingle handles POST /devices/{id}/telemetry.
func (h *Handler) ingestSingle(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validID(id) {
		status(w, http.StatusBadRequest)
		return
	}
	body, code := readBody(w, r, h.cfg.SingleMaxBytes)
	if code != 0 {
		status(w, code)
		return
	}
	p, err := model.ParsePoint(body)
	if err != nil {
		status(w, http.StatusBadRequest)
		return
	}
	if _, err := h.store.Append(r.Context(), id, []model.Point{p}); err != nil {
		status(w, http.StatusServiceUnavailable)
		return
	}
	h.points.Inc()
	status(w, http.StatusAccepted)
}

type batchIn struct {
	Points []json.RawMessage `json:"points"`
}

// ingestBatch handles POST /devices/{id}/telemetry/batch.
func (h *Handler) ingestBatch(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validID(id) {
		status(w, http.StatusBadRequest)
		return
	}
	body, code := readBody(w, r, h.cfg.BatchMaxBytes)
	if code != 0 {
		status(w, code)
		return
	}
	var in batchIn
	if err := json.Unmarshal(body, &in); err != nil {
		status(w, http.StatusBadRequest)
		return
	}
	if len(in.Points) == 0 {
		status(w, http.StatusBadRequest)
		return
	}
	if len(in.Points) > 100 {
		status(w, http.StatusRequestEntityTooLarge)
		return
	}
	pts := make([]model.Point, len(in.Points))
	for i, raw := range in.Points {
		p, err := model.ParsePoint(raw)
		if err != nil {
			status(w, http.StatusBadRequest)
			return
		}
		pts[i] = p
	}
	n, err := h.store.Append(r.Context(), id, pts)
	if err != nil {
		status(w, http.StatusServiceUnavailable)
		return
	}
	h.points.Add(int64(n))
	writeJSON(w, http.StatusAccepted, map[string]int{"accepted": n})
}

// readBody reads the request body under a size cap. It returns the body and a
// zero status on success, or an HTTP status code to send on failure.
func readBody(w http.ResponseWriter, r *http.Request, max int64) ([]byte, int) {
	r.Body = http.MaxBytesReader(w, r.Body, max)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return nil, http.StatusRequestEntityTooLarge
		}
		return nil, http.StatusBadRequest
	}
	return body, 0
}
