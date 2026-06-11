package metrics

import (
	"runtime"
	rmetrics "runtime/metrics"
	"strings"
	"testing"
)

func TestHistStats(t *testing.T) {
	// Three finite buckets: [0,1] empty, [1,2] x2, [2,3] x3.
	h := &rmetrics.Float64Histogram{
		Counts:  []uint64{0, 2, 3},
		Buckets: []float64{0, 1, 2, 3},
	}
	sum, max, count := histStats(h)
	// sum uses bucket midpoints: 2*1.5 + 3*2.5 = 3.0 + 7.5 = 10.5
	if sum != 10.5 {
		t.Errorf("sum = %v, want 10.5", sum)
	}
	// max is the upper bound of the highest non-empty bucket.
	if max != 3 {
		t.Errorf("max = %v, want 3", max)
	}
	if count != 5 {
		t.Errorf("count = %v, want 5", count)
	}
}

func TestHistStatsHandlesInfiniteEdges(t *testing.T) {
	inf := func(neg bool) float64 {
		z := 0.0
		if neg {
			return -1 / z
		}
		return 1 / z
	}
	// [-Inf,1] x4, [1,+Inf] x0: a value lands only in the lowest bucket. The
	// -Inf edge must not produce NaN/Inf; clamp to the finite bound.
	h := &rmetrics.Float64Histogram{
		Counts:  []uint64{4, 0},
		Buckets: []float64{inf(true), 1, inf(false)},
	}
	sum, max, count := histStats(h)
	if sum != 4 { // 4 * clamp([-Inf,1]) == 4 * 1
		t.Errorf("sum = %v, want 4", sum)
	}
	if max != 1 {
		t.Errorf("max = %v, want 1", max)
	}
	if count != 4 {
		t.Errorf("count = %v, want 4", count)
	}
}

func TestRuntimeCollectorEmitsGCGauges(t *testing.T) {
	runtime.GC() // guarantee at least one completed cycle

	var b strings.Builder
	RuntimeCollector(&b)
	out := b.String()

	for _, name := range []string{
		"pibench_go_goroutines",
		"pibench_go_gomaxprocs",
		"pibench_go_gc_cycles_total",
		"pibench_go_gc_pause_seconds_total",
		"pibench_go_gc_pause_seconds_max",
		"pibench_go_sched_latency_seconds_total",
		"pibench_go_sched_latency_seconds_max",
		"pibench_go_heap_objects_bytes",
		"pibench_go_heap_goal_bytes",
	} {
		if !strings.Contains(out, "\n"+name+" ") && !strings.HasPrefix(out, name+" ") {
			t.Errorf("missing metric %q in:\n%s", name, out)
		}
		if !strings.Contains(out, "# TYPE "+name+" ") {
			t.Errorf("missing TYPE for %q in:\n%s", name, out)
		}
	}
	if !strings.Contains(out, "# TYPE pibench_go_gc_cycles_total counter") {
		t.Errorf("gc cycles should be a counter:\n%s", out)
	}
	if !strings.Contains(out, "# TYPE pibench_go_goroutines gauge") {
		t.Errorf("goroutines should be a gauge:\n%s", out)
	}
}
