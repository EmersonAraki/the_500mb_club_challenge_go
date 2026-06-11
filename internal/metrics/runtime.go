package metrics

import (
	"io"
	"math"
	"runtime"
	rmetrics "runtime/metrics"
	"strconv"
)

// Runtime metric names sampled from the Go runtime. These diagnose tail-latency
// stalls: GC stop-the-world pauses vs. CFS scheduling starvation. The two pause
// histograms are the signal -- gc_pause shows time the world was stopped,
// sched_latency shows time goroutines waited for a CPU slice (CFS throttling
// under the 0.45-CPU cgroup limit surfaces here, not in GC).
const (
	mGCCycles     = "/gc/cycles/total:gc-cycles"
	mGCPauses     = "/gc/pauses:seconds"
	mSchedLatency = "/sched/latencies:seconds"
	mHeapObjects  = "/memory/classes/heap/objects:bytes"
	mHeapGoal     = "/gc/heap/goal:bytes"
	mGoroutines   = "/sched/goroutines:goroutines"
)

// runtimeSamples is the fixed read set, ordered to match the constants above.
var runtimeSamples = []rmetrics.Sample{
	{Name: mGCCycles},
	{Name: mGCPauses},
	{Name: mSchedLatency},
	{Name: mHeapObjects},
	{Name: mHeapGoal},
	{Name: mGoroutines},
}

// RuntimeCollector writes Go runtime gauges in Prometheus exposition format. It
// reads runtime/metrics (which, unlike runtime.ReadMemStats, does not stop the
// world) so scraping never perturbs the latency it is measuring.
func RuntimeCollector(w io.Writer) {
	samples := make([]rmetrics.Sample, len(runtimeSamples))
	copy(samples, runtimeSamples)
	rmetrics.Read(samples)
	v := func(name string) rmetrics.Value {
		for i := range samples {
			if samples[i].Name == name {
				return samples[i].Value
			}
		}
		return rmetrics.Value{}
	}

	gauge(w, "pibench_go_goroutines", "Current goroutine count",
		strconv.FormatUint(v(mGoroutines).Uint64(), 10))
	gauge(w, "pibench_go_gomaxprocs", "GOMAXPROCS setting",
		strconv.Itoa(runtime.GOMAXPROCS(0)))

	writeHelp(w, "pibench_go_gc_cycles_total", "Completed GC cycles", "counter")
	line(w, "pibench_go_gc_cycles_total", strconv.FormatUint(v(mGCCycles).Uint64(), 10))

	pauseSum, pauseMax, _ := histStats(v(mGCPauses).Float64Histogram())
	gauge(w, "pibench_go_gc_pause_seconds_total", "Cumulative GC stop-the-world pause time", fsec(pauseSum))
	gauge(w, "pibench_go_gc_pause_seconds_max", "Upper bound of the worst observed GC pause", fsec(pauseMax))

	schedSum, schedMax, _ := histStats(v(mSchedLatency).Float64Histogram())
	gauge(w, "pibench_go_sched_latency_seconds_total", "Cumulative goroutine scheduling wait time", fsec(schedSum))
	gauge(w, "pibench_go_sched_latency_seconds_max", "Upper bound of the worst observed scheduling wait", fsec(schedMax))

	gauge(w, "pibench_go_heap_objects_bytes", "Live heap object bytes",
		strconv.FormatUint(v(mHeapObjects).Uint64(), 10))
	gauge(w, "pibench_go_heap_goal_bytes", "Heap size target for the next GC",
		strconv.FormatUint(v(mHeapGoal).Uint64(), 10))
}

// histStats reduces a runtime histogram to a cumulative sum (via bucket
// midpoints), the upper bound of the highest non-empty bucket, and the total
// count. Infinite edge bounds are clamped to their finite neighbour so the sum
// stays finite.
func histStats(h *rmetrics.Float64Histogram) (sum, max float64, count uint64) {
	if h == nil {
		return 0, 0, 0
	}
	for i, c := range h.Counts {
		if c == 0 {
			continue
		}
		lo, hi := h.Buckets[i], h.Buckets[i+1]
		if math.IsInf(lo, -1) {
			lo = hi
		}
		if math.IsInf(hi, 1) {
			hi = lo
		}
		sum += float64(c) * (lo + hi) / 2
		if hi > max {
			max = hi
		}
		count += c
	}
	return sum, max, count
}

func gauge(w io.Writer, name, help, value string) {
	writeHelp(w, name, help, "gauge")
	line(w, name, value)
}

func writeHelp(w io.Writer, name, help, typ string) {
	io.WriteString(w, "# HELP "+name+" "+help+"\n")
	io.WriteString(w, "# TYPE "+name+" "+typ+"\n")
}

func line(w io.Writer, name, value string) {
	io.WriteString(w, name+" "+value+"\n")
}

func fsec(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }
