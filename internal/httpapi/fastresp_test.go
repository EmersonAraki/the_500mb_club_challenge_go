package httpapi

import (
	"encoding/json"
	"testing"

	"github.com/araki/pibench/internal/model"
)

// The oracle structs mirror the wire contract; appendRangeJSON must produce
// byte-identical output to json.Marshal over them.
type oraclePoint struct {
	TS      int64    `json:"ts"`
	Lat     float64  `json:"lat"`
	Lon     float64  `json:"lon"`
	Battery *float64 `json:"battery,omitempty"`
	Ax      float64  `json:"ax"`
	Ay      float64  `json:"ay"`
	Az      float64  `json:"az"`
}

type oracleRange struct {
	Points     []oraclePoint `json:"points"`
	NextCursor *string       `json:"next_cursor"`
}

func oracleMarshal(pts []model.Point, next string) []byte {
	outs := make([]oraclePoint, len(pts))
	for i, p := range pts {
		o := oraclePoint{TS: p.TS, Lat: p.Lat, Lon: p.Lon, Ax: p.Ax, Ay: p.Ay, Az: p.Az}
		if p.HasBattery {
			b := p.Battery
			o.Battery = &b
		}
		outs[i] = o
	}
	r := oracleRange{Points: outs}
	if next != "" {
		r.NextCursor = &next
	}
	b, _ := json.Marshal(r)
	return b
}

func TestAppendRangeJSONMatchesStdlib(t *testing.T) {
	bat := func(v float64) model.Point {
		return model.Point{TS: 1, Lat: 1, Lon: 2, Ax: 3, Ay: 4, Az: 5, Battery: v, HasBattery: true}
	}
	cases := []struct {
		name string
		pts  []model.Point
		next string
	}{
		{"empty no cursor", nil, ""},
		{"empty with cursor", nil, "Zm9vYmFy"},
		{"one no battery", []model.Point{{TS: 1715800000000, Lat: 12.5, Lon: -40.25, Ax: 0.1, Ay: 0.2, Az: 9.81}}, ""},
		{"one with battery", []model.Point{bat(0.75)}, "abc-_DEF"},
		{"battery zero", []model.Point{bat(0)}, ""},
		{"multi", []model.Point{bat(0.5), {TS: 2, Lat: -90, Lon: 180, Ax: -1, Ay: 0, Az: 1}}, "next"},
		// Float formatting branches: stdlib switches to 'e' below 1e-6 and at/above 1e21.
		{"tiny exp", []model.Point{{TS: 1, Lat: 1e-7, Lon: 9.999e-7, Ax: 1e-6, Ay: 0, Az: -0.0}}, ""},
		{"large exp", []model.Point{{TS: 1, Lat: 1e21, Lon: 1e20, Ax: -1e21, Ay: 123456789.123, Az: 0.1}}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			want := oracleMarshal(c.pts, c.next)
			got := appendRangeJSON(nil, c.pts, c.next)
			if string(got) != string(want) {
				t.Errorf("mismatch\n want=%s\n got =%s", want, got)
			}
		})
	}
}
