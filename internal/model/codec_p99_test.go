package model

// Percentile micro-benchmark comparing serialization strategies for a single
// telemetry point. Standard testing.B reports only mean ns/op; this harness
// records every per-op latency and reports the tail (p99/p99.9), which is what
// the challenge scores. Run with:
//
//	go test ./internal/model -run TestCodecP99 -v
//
// It is a plain test (not a Benchmark) so it always emits the table.

import (
	"encoding/binary"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"testing"
	"time"
)

const (
	p99Iters  = 300_000
	p99Warmup = 50_000
)

func samplePoint() Point {
	return Point{
		TS:         1_717_900_000_000,
		Lat:        -23.55052,
		Lon:        -46.633308,
		Battery:    0.87,
		HasBattery: true,
		Ax:         0.012, Ay: -0.034, Az: 9.81,
	}
}

func sampleJSON() []byte {
	return []byte(`{"ts":1717900000000,"lat":-23.55052,"lon":-46.633308,"battery":0.87,"ax":0.012,"ay":-0.034,"az":9.81}`)
}

// sink vars defeat dead-code elimination.
var (
	sinkBytes []byte
	sinkPoint Point
	sinkErr   error
)

func TestCodecP99(t *testing.T) {
	pt := samplePoint()
	js := sampleJSON()
	bin := pt.Encode(1)

	// Pre-encode a gob blob and prepare reusable gob coders for a fair decode.
	gobBlob := gobEncode(pt)

	type bench struct {
		name string
		fn   func()
	}
	benches := []bench{
		{"json  decode (ParsePoint)", func() { sinkPoint, sinkErr = ParsePoint(js) }},
		{"json  decode (Unmarshal raw)", func() {
			var pj pointJSON
			sinkErr = json.Unmarshal(js, &pj)
		}},
		{"bin   decode (Decode)", func() { sinkPoint, sinkErr = Decode(bin) }},
		{"gob   decode", func() { sinkPoint = gobDecode(gobBlob) }},

		{"json  encode (Marshal)", func() { sinkBytes, sinkErr = json.Marshal(pt) }},
		{"bin   encode (Encode)", func() { sinkBytes = pt.Encode(1) }},
		{"gob   encode", func() { sinkBytes = gobEncode(pt) }},

		{"bin   manual put (PutUint64x6)", func() { sinkBytes = manualPut(pt) }},
	}

	fmt.Printf("\n%-32s %8s %8s %8s %9s %9s %9s\n",
		"codec", "p50", "p90", "p99", "p99.9", "max", "mean")
	fmt.Println("--------------------------------------------------------------------------------------")
	for _, b := range benches {
		report(b.name, measure(b.fn))
	}
	fmt.Printf("\n(%d iters/codec after %d warmup, go1.26 arm64, single-thread)\n", p99Iters, p99Warmup)
}

// measure runs fn p99Iters times, returning per-op latencies in nanoseconds.
func measure(fn func()) []float64 {
	for i := 0; i < p99Warmup; i++ {
		fn()
	}
	lat := make([]float64, p99Iters)
	for i := 0; i < p99Iters; i++ {
		start := time.Now()
		fn()
		lat[i] = float64(time.Since(start).Nanoseconds())
	}
	return lat
}

func report(name string, lat []float64) {
	sort.Float64s(lat)
	var sum float64
	for _, v := range lat {
		sum += v
	}
	mean := sum / float64(len(lat))
	fmt.Printf("%-32s %7.0fn %7.0fn %7.0fn %8.0fn %8.0fn %8.1fn\n",
		name,
		pct(lat, 0.50), pct(lat, 0.90), pct(lat, 0.99),
		pct(lat, 0.999), lat[len(lat)-1], mean)
}

func pct(sorted []float64, p float64) float64 {
	idx := int(p * float64(len(sorted)))
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func gobEncode(p Point) []byte {
	var buf bytesBuffer
	_ = gob.NewEncoder(&buf).Encode(p)
	return buf.b
}

func gobDecode(b []byte) Point {
	var p Point
	_ = gob.NewDecoder(&bytesReader{b: b}).Decode(&p)
	return p
}

// manualPut mirrors Encode but inlined, to isolate make+PutUint64 cost.
func manualPut(p Point) []byte {
	b := make([]byte, EncodedLen)
	binary.BigEndian.PutUint64(b[0:8], uint64(p.TS))
	binary.BigEndian.PutUint64(b[8:16], 1)
	binary.LittleEndian.PutUint64(b[16:24], math.Float64bits(p.Lat))
	binary.LittleEndian.PutUint64(b[24:32], math.Float64bits(p.Lon))
	binary.LittleEndian.PutUint64(b[32:40], math.Float64bits(p.Ax))
	binary.LittleEndian.PutUint64(b[40:48], math.Float64bits(p.Ay))
	binary.LittleEndian.PutUint64(b[48:56], math.Float64bits(p.Az))
	binary.LittleEndian.PutUint64(b[56:64], math.Float64bits(p.Battery))
	if p.HasBattery {
		b[64] = 1
	}
	return b
}

// minimal io.Writer/Reader to avoid importing bytes in this test-only file.
type bytesBuffer struct{ b []byte }

func (w *bytesBuffer) Write(p []byte) (int, error) { w.b = append(w.b, p...); return len(p), nil }

type bytesReader struct {
	b   []byte
	off int
}

func (r *bytesReader) Read(p []byte) (int, error) {
	if r.off >= len(r.b) {
		return 0, fmt.Errorf("EOF")
	}
	n := copy(p, r.b[r.off:])
	r.off += n
	return n, nil
}
