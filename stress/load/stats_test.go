package load

import (
	"strings"
	"testing"
	"time"
)

func TestPercentilesNearestRank(t *testing.T) {
	s := NewStats()
	// 100 samples of 1ms..100ms for "post", all ok.
	for i := 1; i <= 100; i++ {
		s.Record("post", time.Duration(i)*time.Millisecond, true)
	}
	r := s.Op("post")
	if r.Count != 100 || r.Fails != 0 {
		t.Fatalf("count/fails: got %d/%d want 100/0", r.Count, r.Fails)
	}
	// nearest-rank: p50 -> rank 50 -> 50ms, p95 -> 95ms, p99 -> 99ms, p99.9 -> 100ms.
	cases := []struct {
		name string
		got  time.Duration
		want time.Duration
	}{
		{"p50", r.P50, 50 * time.Millisecond},
		{"p95", r.P95, 95 * time.Millisecond},
		{"p99", r.P99, 99 * time.Millisecond},
		{"p999", r.P999, 100 * time.Millisecond},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: got %v want %v", c.name, c.got, c.want)
		}
	}
}

func TestRecordCountsFailures(t *testing.T) {
	s := NewStats()
	s.Record("range", time.Millisecond, true)
	s.Record("range", time.Millisecond, false)
	s.Record("range", time.Millisecond, false)
	r := s.Op("range")
	if r.Count != 3 || r.Fails != 2 {
		t.Errorf("count/fails: got %d/%d want 3/2", r.Count, r.Fails)
	}
}

func TestOpUnknownIsZero(t *testing.T) {
	s := NewStats()
	if r := s.Op("nope"); r.Count != 0 {
		t.Errorf("unknown op count: got %d want 0", r.Count)
	}
}

func TestReportIncludesEveryOpAndErrorRate(t *testing.T) {
	s := NewStats()
	s.Record("post", 2*time.Millisecond, true)
	s.Record("anomaly", 3*time.Millisecond, false)
	var b strings.Builder
	s.Report(&b)
	out := b.String()
	for _, want := range []string{"post", "anomaly", "p99", "error"} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q in:\n%s", want, out)
		}
	}
}
