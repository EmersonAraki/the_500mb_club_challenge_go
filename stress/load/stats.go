// Package load is a dependency-free, in-process load generator for the telemetry
// API. It mirrors the official k6 scenarios (steady/spike/capacity/endurance) so
// the E1/E2 experiment loop can run anywhere the Go toolchain exists -- including
// directly on the Pi -- without a k6 install. It is NOT the official harness:
// final, comparable numbers must still come from the challenge's k6 suite. This
// code never ships in the image (the Dockerfile copies only cmd/ and internal/).
package load

import (
	"io"
	"math"
	"sort"
	"sync"
	"text/tabwriter"
	"time"
)

// OpResult is the latency summary for one operation.
type OpResult struct {
	P50, P95, P99, P999 time.Duration
	Min, Max            time.Duration
	Count               int
	Fails               int
}

// Stats accumulates per-operation latencies and outcomes. Safe for concurrent
// use by the driver's workers.
type Stats struct {
	mu    sync.Mutex
	ops   map[string]*opSamples
	order []string
}

type opSamples struct {
	durs  []time.Duration
	fails int
}

// NewStats returns an empty Stats.
func NewStats() *Stats {
	return &Stats{ops: make(map[string]*opSamples)}
}

// Record stores one sample for op: its latency and whether it succeeded.
func (s *Stats) Record(op string, d time.Duration, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o := s.ops[op]
	if o == nil {
		o = &opSamples{}
		s.ops[op] = o
		s.order = append(s.order, op)
	}
	o.durs = append(o.durs, d)
	if !ok {
		o.fails++
	}
}

// Op returns the latency summary for one operation (zero value if unseen).
func (s *Stats) Op(op string) OpResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	return summarize(s.ops[op])
}

func summarize(o *opSamples) OpResult {
	if o == nil || len(o.durs) == 0 {
		return OpResult{}
	}
	sorted := append([]time.Duration(nil), o.durs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	return OpResult{
		P50:   percentile(sorted, 50),
		P95:   percentile(sorted, 95),
		P99:   percentile(sorted, 99),
		P999:  percentile(sorted, 99.9),
		Min:   sorted[0],
		Max:   sorted[len(sorted)-1],
		Count: len(sorted),
		Fails: o.fails,
	}
}

// percentile uses the nearest-rank method on an ascending slice: rank =
// ceil(p/100 * n), 1-indexed. Deterministic and dependency-free; k6 interpolates,
// so treat these as the inner-loop proxy, not the official figure.
func percentile(sorted []time.Duration, p float64) time.Duration {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	rank := int(math.Ceil(p / 100 * float64(n)))
	if rank < 1 {
		rank = 1
	}
	if rank > n {
		rank = n
	}
	return sorted[rank-1]
}

// Report writes a per-op table (p50/p95/p99/p99.9 + error rate) plus a totals
// row, mirroring the scoring-table format in the README.
func (s *Stats) Report(w io.Writer) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	io.WriteString(tw, "op\tcount\tp50\tp95\tp99\tp99.9\tmax\terror%\n")
	var totalCount, totalFails int
	for _, op := range s.order {
		r := summarize(s.ops[op])
		totalCount += r.Count
		totalFails += r.Fails
		writeRow(tw, op, r)
	}
	if len(s.order) > 0 {
		errPct := 0.0
		if totalCount > 0 {
			errPct = 100 * float64(totalFails) / float64(totalCount)
		}
		io.WriteString(tw, "TOTAL\t"+itoa(totalCount)+"\t\t\t\t\t\t"+ftoa(errPct)+"\n")
	}
	tw.Flush()
}

func writeRow(tw io.Writer, op string, r OpResult) {
	errPct := 0.0
	if r.Count > 0 {
		errPct = 100 * float64(r.Fails) / float64(r.Count)
	}
	io.WriteString(tw, op+"\t"+itoa(r.Count)+"\t"+
		ms(r.P50)+"\t"+ms(r.P95)+"\t"+ms(r.P99)+"\t"+ms(r.P999)+"\t"+ms(r.Max)+"\t"+ftoa(errPct)+"\n")
}
