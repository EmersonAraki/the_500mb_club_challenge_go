package anomaly

import (
	"errors"
	"math"
	"testing"

	"github.com/araki/pibench/internal/model"
)

// pts builds points (most-recent-first) from az magnitudes, with ax=ay=0 so
// magnitude == |az|.
func pts(azs ...float64) []model.Point {
	out := make([]model.Point, len(azs))
	for i, az := range azs {
		out[i] = model.Point{Az: az}
	}
	return out
}

func TestComputeInsufficientSamples(t *testing.T) {
	_, err := Compute(pts(1, 1, 1, 1, 1, 1, 1)) // 7 < 8
	if !errors.Is(err, ErrInsufficient) {
		t.Fatalf("expected ErrInsufficient, got %v", err)
	}
}

func TestComputeKnownStatistics(t *testing.T) {
	// magnitudes [2,2,2,2,4,4,4,4] -> mean 3, population stddev 1.
	// most recent (index 0) magnitude is 2 -> z = (2-3)/1 = -1.
	r, err := Compute(pts(2, 2, 2, 2, 4, 4, 4, 4))
	if err != nil {
		t.Fatal(err)
	}
	if r.Samples != 8 {
		t.Errorf("samples: got %d want 8", r.Samples)
	}
	if !approx(r.Mean, 3) {
		t.Errorf("mean: got %v want 3", r.Mean)
	}
	if !approx(r.Stddev, 1) {
		t.Errorf("stddev: got %v want 1", r.Stddev)
	}
	if !approx(r.ZScore, -1) {
		t.Errorf("z_score: got %v want -1", r.ZScore)
	}
	if r.Anomalous {
		t.Errorf("expected not anomalous for |z|=1")
	}
}

func TestComputeAnomalousAboveThreshold(t *testing.T) {
	// one value 100, sixteen 0 -> mean 100/17, stddev 400/17, z = 4.0 exactly.
	azs := make([]float64, 17)
	azs[0] = 100 // most recent is the outlier
	r, err := Compute(pts(azs...))
	if err != nil {
		t.Fatal(err)
	}
	if !approx(r.ZScore, 4) {
		t.Errorf("z_score: got %v want 4", r.ZScore)
	}
	if !r.Anomalous {
		t.Errorf("expected anomalous for |z|=4 (> 3)")
	}
}

func TestComputeNegativeAnomalyUsesAbsoluteValue(t *testing.T) {
	// outlier is small relative to cluster -> z = -4 -> |z| > 3 -> anomalous.
	azs := make([]float64, 17)
	for i := range azs {
		azs[i] = 100
	}
	azs[0] = 0 // most recent drops to 0
	r, err := Compute(pts(azs...))
	if err != nil {
		t.Fatal(err)
	}
	if r.ZScore >= 0 {
		t.Errorf("expected negative z, got %v", r.ZScore)
	}
	if !r.Anomalous {
		t.Errorf("expected anomalous for negative z beyond -3")
	}
}

func TestComputeZeroStddev(t *testing.T) {
	r, err := Compute(pts(5, 5, 5, 5, 5, 5, 5, 5))
	if err != nil {
		t.Fatal(err)
	}
	if r.Stddev != 0 || r.ZScore != 0 || r.Anomalous {
		t.Errorf("flat data: got stddev=%v z=%v anomalous=%v want 0,0,false",
			r.Stddev, r.ZScore, r.Anomalous)
	}
}

func TestComputeZeroStddevYieldsFiniteFields(t *testing.T) {
	// Flat data gives stddev 0; an unguarded z = (mag-mean)/0 would be NaN, which
	// encoding/json refuses to marshal -> a broken 200. Every field must be finite.
	r, err := Compute(pts(5, 5, 5, 5, 5, 5, 5, 5))
	if err != nil {
		t.Fatal(err)
	}
	for name, v := range map[string]float64{"z": r.ZScore, "mean": r.Mean, "stddev": r.Stddev} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Errorf("%s is not finite: %v", name, v)
		}
	}
}

func TestComputeCapsAt256(t *testing.T) {
	azs := make([]float64, 300)
	r, err := Compute(pts(azs...))
	if err != nil {
		t.Fatal(err)
	}
	if r.Samples != 256 {
		t.Errorf("samples: got %d want 256 (capped)", r.Samples)
	}
}

func approx(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}
