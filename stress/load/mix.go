package load

// WeightedOp is an operation name and its relative weight in the request mix.
type WeightedOp struct {
	Name   string
	Weight int
}

// Mix selects an operation by a uniform draw in [0,1), preserving the order the
// ops were given so cumulative thresholds are deterministic.
type Mix struct {
	names []string
	cum   []float64 // cumulative upper bound per op, last == 1.0
}

// NewMix builds a mix from ordered (name, weight) pairs.
func NewMix(ops []WeightedOp) *Mix {
	total := 0
	for _, o := range ops {
		total += o.Weight
	}
	m := &Mix{}
	run := 0
	for _, o := range ops {
		run += o.Weight
		m.names = append(m.names, o.Name)
		m.cum = append(m.cum, float64(run)/float64(total))
	}
	return m
}

// DefaultMix is the scoring profile: 60% single ingest, 10% batch, 20% range
// query, 10% anomaly.
func DefaultMix() *Mix {
	return NewMix([]WeightedOp{
		{"post", 60}, {"batch", 10}, {"range", 20}, {"anomaly", 10},
	})
}

// Names returns the op names in mix order.
func (m *Mix) Names() []string {
	return append([]string(nil), m.names...)
}

// Pick returns the op whose cumulative band contains r (clamped to [0,1)).
func (m *Mix) Pick(r float64) string {
	for i, c := range m.cum {
		if r < c {
			return m.names[i]
		}
	}
	return m.names[len(m.names)-1]
}
