package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/araki/pibench/internal/model"
)

// pointOut serializes a telemetry point. Battery is a pointer so it is emitted
// only when present (battery is optional and 0.0 is a valid value).
type pointOut struct {
	TS      int64    `json:"ts"`
	Lat     float64  `json:"lat"`
	Lon     float64  `json:"lon"`
	Battery *float64 `json:"battery,omitempty"`
	Ax      float64  `json:"ax"`
	Ay      float64  `json:"ay"`
	Az      float64  `json:"az"`
}

func toOut(p model.Point) pointOut {
	o := pointOut{TS: p.TS, Lat: p.Lat, Lon: p.Lon, Ax: p.Ax, Ay: p.Ay, Az: p.Az}
	if p.HasBattery {
		b := p.Battery
		o.Battery = &b
	}
	return o
}

func toOuts(pts []model.Point) []pointOut {
	out := make([]pointOut, len(pts))
	for i, p := range pts {
		out[i] = toOut(p)
	}
	return out
}

// writeJSON encodes v as the body with the given status code.
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// status writes a bare status code with no body.
func status(w http.ResponseWriter, code int) { w.WriteHeader(code) }
