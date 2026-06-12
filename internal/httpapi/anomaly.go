package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/araki/pibench/internal/anomaly"
)

// anomalyOut is the z-score response.
type anomalyOut struct {
	ZScore    float64 `json:"z_score"`
	Samples   int     `json:"samples"`
	Anomalous bool    `json:"anomalous"`
	Mean      float64 `json:"mean"`
	Stddev    float64 `json:"stddev"`
}

// anomaly handles GET /devices/{id}/anomaly. It recomputes on every call (no
// cache) over the newest 256 points.
func (h *Handler) anomaly(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validID(id) {
		status(w, http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.cfg.ReadTimeout)
	defer cancel()
	pts, err := h.store.Recent(ctx, id, 256)
	if err != nil {
		status(w, h.readStatus(err))
		return
	}
	res, err := anomaly.Compute(pts)
	if err != nil {
		if errors.Is(err, anomaly.ErrInsufficient) {
			status(w, http.StatusNotFound)
			return
		}
		status(w, http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, anomalyOut{
		ZScore:    res.ZScore,
		Samples:   res.Samples,
		Anomalous: res.Anomalous,
		Mean:      res.Mean,
		Stddev:    res.Stddev,
	})
}
