// Package anomaly computes the z-score of the most recent acceleration
// magnitude against the window of recent points. No caching: every call
// recomputes from the supplied points.
package anomaly

import (
	"errors"
	"math"

	"github.com/araki/pibench/internal/model"
)

// window is the maximum number of points considered.
const window = 256

// minSamples is the minimum points required to compute a z-score.
const minSamples = 8

// ErrInsufficient indicates fewer than minSamples points were available.
var ErrInsufficient = errors.New("insufficient samples")

// Result is the anomaly computation output.
type Result struct {
	ZScore    float64
	Mean      float64
	Stddev    float64
	Samples   int
	Anomalous bool
}

// Compute expects points ordered most-recent-first (index 0 is newest). It
// considers at most the newest 256 and returns ErrInsufficient if fewer than 8.
func Compute(points []model.Point) (Result, error) {
	if len(points) < minSamples {
		return Result{}, ErrInsufficient
	}
	if len(points) > window {
		points = points[:window]
	}
	// Single-pass Welford: running mean + sum-of-squared-deviations (m2), no
	// intermediate magnitudes slice. Mean and variance are order-independent, so
	// the newest-first orientation does not matter for them; we capture the newest
	// magnitude (index 0) separately for the z-score numerator.
	var mean, m2, newestMag float64
	for i, p := range points {
		mag := math.Sqrt(p.Ax*p.Ax + p.Ay*p.Ay + p.Az*p.Az)
		if i == 0 {
			newestMag = mag
		}
		delta := mag - mean
		mean += delta / float64(i+1)
		m2 += delta * (mag - mean)
	}
	// Population variance (÷N), per the contract; sample variance (÷N-1) is wrong.
	stddev := math.Sqrt(m2 / float64(len(points)))

	var z float64
	if stddev != 0 {
		z = (newestMag - mean) / stddev
	}

	return Result{
		ZScore:    z,
		Mean:      mean,
		Stddev:    stddev,
		Samples:   len(points),
		Anomalous: math.Abs(z) > 3,
	}, nil
}
