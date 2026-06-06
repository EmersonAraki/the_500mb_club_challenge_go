// Package metrics provides a minimal, dependency-free Prometheus exposition:
// atomic counters only (no histograms), to keep hot-path allocation and the
// binary footprint low.
package metrics

import (
	"io"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
)

// Registry holds the registered metric families.
type Registry struct {
	mu         sync.RWMutex
	counters   []*Counter
	counterVec []*CounterVec
}

// New returns an empty Registry.
func New() *Registry { return &Registry{} }

// Counter is a monotonically increasing value.
type Counter struct {
	name string
	help string
	v    atomic.Int64
}

// Inc adds one. Add adds n.
func (c *Counter) Inc()        { c.v.Add(1) }
func (c *Counter) Add(n int64) { c.v.Add(n) }

// NewCounter registers and returns a counter.
func (r *Registry) NewCounter(name, help string) *Counter {
	c := &Counter{name: name, help: help}
	r.mu.Lock()
	r.counters = append(r.counters, c)
	r.mu.Unlock()
	return c
}

// Sample is a single counter cell within a CounterVec.
type Sample struct{ v atomic.Int64 }

// Inc adds one. Add adds n.
func (s *Sample) Inc()        { s.v.Add(1) }
func (s *Sample) Add(n int64) { s.v.Add(n) }

// CounterVec is a counter family partitioned by a single label.
type CounterVec struct {
	name   string
	help   string
	label  string
	mu     sync.RWMutex
	series map[string]*Sample
}

// NewCounterVec registers and returns a labeled counter family.
func (r *Registry) NewCounterVec(name, help, label string) *CounterVec {
	cv := &CounterVec{name: name, help: help, label: label, series: map[string]*Sample{}}
	r.mu.Lock()
	r.counterVec = append(r.counterVec, cv)
	r.mu.Unlock()
	return cv
}

// With returns the counter for the given label value, creating it on first use.
func (cv *CounterVec) With(value string) *Sample {
	cv.mu.RLock()
	c := cv.series[value]
	cv.mu.RUnlock()
	if c != nil {
		return c
	}
	cv.mu.Lock()
	defer cv.mu.Unlock()
	if c = cv.series[value]; c == nil {
		c = &Sample{}
		cv.series[value] = c
	}
	return c
}

// Render writes all metrics in Prometheus text exposition format.
func (r *Registry) Render(w io.Writer) {
	r.mu.RLock()
	counters := append([]*Counter(nil), r.counters...)
	vecs := append([]*CounterVec(nil), r.counterVec...)
	r.mu.RUnlock()

	for _, c := range counters {
		writeHelpType(w, c.name, c.help)
		io.WriteString(w, c.name+" "+strconv.FormatInt(c.v.Load(), 10)+"\n")
	}
	for _, cv := range vecs {
		writeHelpType(w, cv.name, cv.help)
		cv.mu.RLock()
		keys := make([]string, 0, len(cv.series))
		for k := range cv.series {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			io.WriteString(w, cv.name+"{"+cv.label+`="`+k+`"} `+
				strconv.FormatInt(cv.series[k].v.Load(), 10)+"\n")
		}
		cv.mu.RUnlock()
	}
}

func writeHelpType(w io.Writer, name, help string) {
	io.WriteString(w, "# HELP "+name+" "+help+"\n")
	io.WriteString(w, "# TYPE "+name+" counter\n")
}
