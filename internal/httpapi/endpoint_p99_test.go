package httpapi

// Per-endpoint p99 latency, driving the real handler in-process against the
// in-memory store. Run in the Pi-5 proxy container:
//
//	./scripts/endpoint-bench.sh           (or)
//	go test ./internal/httpapi -run TestEndpointP99 -v
//
// Scope: this measures handler + in-mem-store CPU per call, sequentially. It
// EXCLUDES the Redis round-trip (production uses redisStore) and concurrency, so
// treat it as a per-endpoint cost comparison and a latency floor, not the k6
// full-stack number.

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/araki/pibench/internal/metrics"
	"github.com/araki/pibench/internal/model"
	"github.com/araki/pibench/internal/store"
)

const (
	epIters    = 50_000
	epWarmup   = 5_000
	seededPts  = 300
	baseTS     = 1_700_000_000_000
	deviceID   = "dev-bench-1"
	batchCount = 100
)

func newBenchHandler(t *testing.T) (*Handler, store.Store) {
	t.Helper()
	st := store.NewMem(1024)
	pts := make([]model.Point, seededPts)
	for i := range pts {
		pts[i] = model.Point{
			TS:  int64(baseTS + i),
			Lat: -23.5, Lon: -46.6, Ax: 0.1 + float64(i%7)*0.01, Ay: 0.2, Az: 9.8,
			HasBattery: true, Battery: 0.5,
		}
	}
	if _, err := st.Append(context.Background(), deviceID, pts); err != nil {
		t.Fatalf("seed: %v", err)
	}
	h := New(st, metrics.New(), Config{
		InstanceID: "bench", SingleMaxBytes: 4096, BatchMaxBytes: 1 << 20,
	})
	return h, st
}

func singleBody() []byte {
	return []byte(`{"ts":1700000999999,"lat":-23.55,"lon":-46.63,"battery":0.7,"ax":0.05,"ay":-0.02,"az":9.79}`)
}

func batchBody() []byte {
	var b bytes.Buffer
	b.WriteString(`{"points":[`)
	for i := 0; i < batchCount; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"ts":%d,"lat":-23.55,"lon":-46.63,"battery":0.7,"ax":0.05,"ay":-0.02,"az":9.79}`,
			baseTS+1000+i)
	}
	b.WriteString(`]}`)
	return b.Bytes()
}

func TestEndpointP99(t *testing.T) {
	h, _ := newBenchHandler(t)

	rangeURL := fmt.Sprintf("/devices/%s/telemetry?from=%d&to=%d&limit=100",
		deviceID, baseTS, baseTS+seededPts)
	single := singleBody()
	batch := batchBody()

	type ep struct {
		name string
		want int
		req  func() *http.Request
	}
	eps := []ep{
		{"GET  /healthz", 200, func() *http.Request {
			return httptest.NewRequest("GET", "/healthz", nil)
		}},
		{"GET  /metrics", 200, func() *http.Request {
			return httptest.NewRequest("GET", "/metrics", nil)
		}},
		{"POST /telemetry (single)", 202, func() *http.Request {
			return httptest.NewRequest("POST", "/devices/"+deviceID+"/telemetry", bytes.NewReader(single))
		}},
		{"POST /telemetry/batch (100)", 202, func() *http.Request {
			return httptest.NewRequest("POST", "/devices/"+deviceID+"/telemetry/batch", bytes.NewReader(batch))
		}},
		{"GET  /telemetry (range 100)", 200, func() *http.Request {
			return httptest.NewRequest("GET", rangeURL, nil)
		}},
		{"GET  /anomaly (256-pt)", 200, func() *http.Request {
			return httptest.NewRequest("GET", "/devices/"+deviceID+"/anomaly", nil)
		}},
	}

	fmt.Printf("\n%-30s %5s %8s %8s %8s %9s %9s\n",
		"endpoint", "code", "p50", "p90", "p99", "p99.9", "mean")
	fmt.Println("------------------------------------------------------------------------------------")
	for _, e := range eps {
		lat := measureEndpoint(t, h, e.req, e.want)
		reportEndpoint(e.name, e.want, lat)
	}
	fmt.Printf("\n(%d req/endpoint after %d warmup; handler+mem-store CPU, no Redis RTT, sequential)\n",
		epIters, epWarmup)
}

func measureEndpoint(t *testing.T, h *Handler, mk func() *http.Request, wantCode int) []float64 {
	t.Helper()
	for i := 0; i < epWarmup; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, mk())
		if i == 0 && rec.Code != wantCode {
			t.Fatalf("warmup status = %d, want %d (body: %s)", rec.Code, wantCode, rec.Body.String())
		}
	}
	lat := make([]float64, epIters)
	for i := 0; i < epIters; i++ {
		req := mk()
		rec := httptest.NewRecorder()
		start := time.Now()
		h.ServeHTTP(rec, req)
		lat[i] = float64(time.Since(start).Nanoseconds())
	}
	return lat
}

func reportEndpoint(name string, code int, lat []float64) {
	sort.Float64s(lat)
	var sum float64
	for _, v := range lat {
		sum += v
	}
	fmt.Printf("%-30s %5s %7.0fn %7.0fn %7.0fn %8.0fn %8.0fn\n",
		name, strconv.Itoa(code),
		epPct(lat, 0.50), epPct(lat, 0.90), epPct(lat, 0.99), epPct(lat, 0.999),
		sum/float64(len(lat)))
}

func epPct(sorted []float64, p float64) float64 {
	idx := int(p * float64(len(sorted)))
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
