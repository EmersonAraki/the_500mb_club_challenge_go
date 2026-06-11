package httpapi

import (
	"testing"

	"github.com/araki/pibench/internal/model"
)

func benchPoints(n int) []model.Point {
	pts := make([]model.Point, n)
	for i := range pts {
		pts[i] = model.Point{
			TS: int64(1715800000000 + i), Lat: 12.5, Lon: -40.25,
			Ax: 0.1, Ay: 0.2, Az: 9.81, Battery: 0.75, HasBattery: true,
		}
	}
	return pts
}

// BenchmarkRangeEncode_Fast is the production reflection-free encoder.
func BenchmarkRangeEncode_Fast(b *testing.B) {
	pts := benchPoints(100)
	buf := make([]byte, 0, len(pts)*96+32)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = appendRangeJSON(buf[:0], pts, "Zm9vYmFy")
	}
}

// BenchmarkRangeEncode_Stdlib is the prior reflection path, kept to quantify
// the delta (builds the intermediate structs and marshals them).
func BenchmarkRangeEncode_Stdlib(b *testing.B) {
	pts := benchPoints(100)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = oracleMarshal(pts, "Zm9vYmFy")
	}
}
