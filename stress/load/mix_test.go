package load

import "testing"

func TestDefaultMixBoundaries(t *testing.T) {
	m := DefaultMix() // post 60 / batch 10 / range 20 / anomaly 10
	cases := []struct {
		r    float64
		want string
	}{
		{0.0, "post"},
		{0.59, "post"},
		{0.60, "batch"},
		{0.69, "batch"},
		{0.70, "range"},
		{0.89, "range"},
		{0.90, "anomaly"},
		{0.999, "anomaly"},
		{1.0, "anomaly"}, // clamp at the top
	}
	for _, c := range cases {
		if got := m.Pick(c.r); got != c.want {
			t.Errorf("Pick(%v): got %q want %q", c.r, got, c.want)
		}
	}
}

func TestMixNames(t *testing.T) {
	got := DefaultMix().Names()
	want := []string{"post", "batch", "range", "anomaly"}
	if len(got) != len(want) {
		t.Fatalf("names len: got %d want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("names[%d]: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestMixProportions(t *testing.T) {
	m := NewMix([]WeightedOp{{"a", 1}, {"b", 3}}) // a:25% b:75%
	counts := map[string]int{}
	for i := 0; i < 10000; i++ {
		counts[m.Pick(float64(i)/10000)]++
	}
	// a covers [0,0.25): ~2500; b covers [0.25,1): ~7500.
	if counts["a"] < 2400 || counts["a"] > 2600 {
		t.Errorf("a count out of band: %d (want ~2500)", counts["a"])
	}
	if counts["b"] < 7400 || counts["b"] > 7600 {
		t.Errorf("b count out of band: %d (want ~7500)", counts["b"])
	}
}
