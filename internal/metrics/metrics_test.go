package metrics

import (
	"io"
	"strconv"
	"strings"
	"testing"
)

func render(r *Registry) string {
	var b strings.Builder
	r.Render(&b)
	return b.String()
}

func TestCounterExposition(t *testing.T) {
	r := New()
	c := r.NewCounter("pibench_points_total", "Total points ingested")
	c.Inc()
	c.Add(2)
	out := render(r)
	for _, want := range []string{
		"# HELP pibench_points_total Total points ingested",
		"# TYPE pibench_points_total counter",
		"pibench_points_total 3",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("exposition missing %q in:\n%s", want, out)
		}
	}
}

func TestCounterVecPerLabel(t *testing.T) {
	r := New()
	v := r.NewCounterVec("pibench_http_requests_total", "Requests by status", "code")
	v.With("200").Inc()
	v.With("200").Inc()
	v.With("400").Inc()
	out := render(r)
	if !strings.Contains(out, `pibench_http_requests_total{code="200"} 2`) {
		t.Errorf("missing 200 series in:\n%s", out)
	}
	if !strings.Contains(out, `pibench_http_requests_total{code="400"} 1`) {
		t.Errorf("missing 400 series in:\n%s", out)
	}
	// HELP/TYPE emitted once for the family.
	if n := strings.Count(out, "# TYPE pibench_http_requests_total counter"); n != 1 {
		t.Errorf("TYPE emitted %d times, want 1", n)
	}
}

func TestCollectorRendersAtScrapeTime(t *testing.T) {
	r := New()
	n := 0
	r.AddCollector(func(w io.Writer) {
		n++
		io.WriteString(w, "# HELP pibench_go_goroutines Current goroutines\n")
		io.WriteString(w, "# TYPE pibench_go_goroutines gauge\n")
		io.WriteString(w, "pibench_go_goroutines "+strconv.Itoa(n)+"\n")
	})
	// Each scrape re-invokes the collector, so the value reflects scrape time.
	if got := render(r); !strings.Contains(got, "pibench_go_goroutines 1") {
		t.Errorf("first scrape missing collector output:\n%s", got)
	}
	if got := render(r); !strings.Contains(got, "pibench_go_goroutines 2") {
		t.Errorf("second scrape did not re-invoke collector:\n%s", got)
	}
}

func TestCounterVecConcurrentSafe(t *testing.T) {
	r := New()
	v := r.NewCounterVec("c", "h", "l")
	done := make(chan struct{})
	for i := 0; i < 8; i++ {
		go func() {
			for j := 0; j < 1000; j++ {
				v.With("x").Inc()
			}
			done <- struct{}{}
		}()
	}
	for i := 0; i < 8; i++ {
		<-done
	}
	if !strings.Contains(render(r), `c{l="x"} 8000`) {
		t.Errorf("expected 8000 after concurrent increments:\n%s", render(r))
	}
}
