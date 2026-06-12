package load

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// stubAPI answers like the telemetry contract: 202 for ingest, 200 for reads.
func stubAPI() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /devices/{id}/telemetry", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(202) })
	mux.HandleFunc("POST /devices/{id}/telemetry/batch", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(202) })
	mux.HandleFunc("GET /devices/{id}/telemetry", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("GET /devices/{id}/anomaly", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	return mux
}

func TestDriverIssuesMixedRequestsAndRecords(t *testing.T) {
	srv := httptest.NewServer(stubAPI())
	defer srv.Close()

	res := Run(context.Background(), Config{
		BaseURL:   srv.URL,
		Mix:       DefaultMix(),
		Segments:  []Segment{{Rate: 200, Dur: 300 * time.Millisecond}},
		Devices:   5,
		BatchSize: 4,
		Workers:   50,
		Seed:      7,
	})

	if res.Stats.Op("post").Count == 0 {
		t.Error("expected some post requests recorded")
	}
	// Over ~60 requests with a 60% share, post should dominate the mix.
	total := 0
	var fails int
	for _, op := range []string{"post", "batch", "range", "anomaly"} {
		r := res.Stats.Op(op)
		total += r.Count
		fails += r.Fails
	}
	if total < 10 {
		t.Errorf("too few requests issued: %d", total)
	}
	if fails != 0 {
		t.Errorf("stub API should never fail, got %d fails", fails)
	}
}

func TestDriverCountsNon2xxAsFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(503)
	}))
	defer srv.Close()

	res := Run(context.Background(), Config{
		BaseURL:  srv.URL,
		Mix:      NewMix([]WeightedOp{{"post", 1}}),
		Segments: []Segment{{Rate: 100, Dur: 150 * time.Millisecond}},
		Devices:  2,
		Workers:  20,
		Seed:     1,
	})
	r := res.Stats.Op("post")
	if r.Count == 0 || r.Fails != r.Count {
		t.Errorf("all requests should fail on 503: count=%d fails=%d", r.Count, r.Fails)
	}
}

func TestReportRendersDriverStats(t *testing.T) {
	s := NewStats()
	s.Record("post", time.Millisecond, true)
	var b strings.Builder
	s.Report(&b)
	if !strings.Contains(b.String(), "TOTAL") {
		t.Error("report should include a TOTAL row")
	}
}
