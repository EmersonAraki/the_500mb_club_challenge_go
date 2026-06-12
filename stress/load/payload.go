package load

import (
	"math/rand"
	"strconv"
	"time"
)

// RandomPoint builds a single contract-valid telemetry point as JSON: ts > 0,
// lat in [-90,90], lon in [-180,180], finite accelerations, optional battery in
// [0,1]. Built by hand (no encoding/json) to keep the generator allocation-light.
func RandomPoint(rng *rand.Rand) []byte {
	return appendPoint(make([]byte, 0, 160), rng)
}

// RandomBatch builds a {"points":[...]} body of n contract-valid points.
func RandomBatch(rng *rand.Rand, n int) []byte {
	b := make([]byte, 0, 16+n*160)
	b = append(b, `{"points":[`...)
	for i := 0; i < n; i++ {
		if i > 0 {
			b = append(b, ',')
		}
		b = appendPoint(b, rng)
	}
	return append(b, ']', '}')
}

func appendPoint(b []byte, rng *rand.Rand) []byte {
	ts := time.Now().UnixMilli() + rng.Int63n(1000)
	lat := rng.Float64()*180 - 90
	lon := rng.Float64()*360 - 180
	ax := rng.NormFloat64()
	ay := rng.NormFloat64()
	az := 9.81 + rng.NormFloat64()

	b = append(b, `{"ts":`...)
	b = strconv.AppendInt(b, ts, 10)
	b = appendField(b, "lat", lat)
	b = appendField(b, "lon", lon)
	b = appendField(b, "ax", ax)
	b = appendField(b, "ay", ay)
	b = appendField(b, "az", az)
	// Battery is optional; include it ~half the time, always in range.
	if rng.Intn(2) == 0 {
		b = appendField(b, "battery", rng.Float64())
	}
	return append(b, '}')
}

func appendField(b []byte, name string, v float64) []byte {
	b = append(b, ',', '"')
	b = append(b, name...)
	b = append(b, '"', ':')
	return strconv.AppendFloat(b, v, 'f', 6, 64)
}
