package model

// Explores alternative byte layouts for the sorted-set member to see which, if
// any, beats the current 65-byte Encode. Measures tail latency (p99), allocs/op,
// and member size, because member size feeds the RAM-efficiency score. These are
// experiments only; production code is unchanged.
//
//	go test ./internal/model -run TestEncodeVariants -v
//
// Variants:
//   current      : production Encode  (make[65], BE ts/seq, LE floats, flag byte)
//   native-endian: floats via NativeEndian (proves endianness is free on arm64)
//   into-buffer  : write into a caller-owned []byte, zero allocation
//   unsafe       : single 65-byte store via *[65]byte, zero allocation
//   64B-nan      : drop the flag byte; absent battery = NaN sentinel (8-aligned)
//   64B-into     : 64B-nan written into a caller buffer, zero allocation

import (
	"encoding/binary"
	"fmt"
	"math"
	"runtime"
	"sort"
	"testing"
	"time"
	"unsafe"
)

const variantIters = 300_000

// --- variant encoders -------------------------------------------------------

// v_nativeEndian: identical bytes to current on a little-endian host; isolates
// whether the LittleEndian helper costs anything vs NativeEndian.
func v_nativeEndian(p Point, seq uint64) []byte {
	b := make([]byte, 65)
	binary.BigEndian.PutUint64(b[0:8], uint64(p.TS))
	binary.BigEndian.PutUint64(b[8:16], seq)
	binary.NativeEndian.PutUint64(b[16:24], math.Float64bits(p.Lat))
	binary.NativeEndian.PutUint64(b[24:32], math.Float64bits(p.Lon))
	binary.NativeEndian.PutUint64(b[32:40], math.Float64bits(p.Ax))
	binary.NativeEndian.PutUint64(b[40:48], math.Float64bits(p.Ay))
	binary.NativeEndian.PutUint64(b[48:56], math.Float64bits(p.Az))
	binary.NativeEndian.PutUint64(b[56:64], math.Float64bits(p.Battery))
	if p.HasBattery {
		b[64] = 1
	}
	return b
}

// v_into: same 65-byte layout but the caller owns the buffer (no per-call alloc).
func v_into(dst []byte, p Point, seq uint64) {
	binary.BigEndian.PutUint64(dst[0:8], uint64(p.TS))
	binary.BigEndian.PutUint64(dst[8:16], seq)
	binary.LittleEndian.PutUint64(dst[16:24], math.Float64bits(p.Lat))
	binary.LittleEndian.PutUint64(dst[24:32], math.Float64bits(p.Lon))
	binary.LittleEndian.PutUint64(dst[32:40], math.Float64bits(p.Ax))
	binary.LittleEndian.PutUint64(dst[40:48], math.Float64bits(p.Ay))
	binary.LittleEndian.PutUint64(dst[48:56], math.Float64bits(p.Az))
	binary.LittleEndian.PutUint64(dst[56:64], math.Float64bits(p.Battery))
	dst[64] = 0
	if p.HasBattery {
		dst[64] = 1
	}
}

// v_unsafe: writes the eight 64-bit words through a fixed-size array view, then
// the flag. One bounds check, no per-field reslice. Little-endian host only.
func v_unsafe(p Point, seq uint64) []byte {
	b := make([]byte, 65)
	w := (*[8]uint64)(unsafe.Pointer(&b[0]))
	w[0] = bswap64(uint64(p.TS)) // keep ts big-endian for sort
	w[1] = bswap64(seq)
	w[2] = math.Float64bits(p.Lat)
	w[3] = math.Float64bits(p.Lon)
	w[4] = math.Float64bits(p.Ax)
	w[5] = math.Float64bits(p.Ay)
	w[6] = math.Float64bits(p.Az)
	w[7] = math.Float64bits(p.Battery)
	if p.HasBattery {
		b[64] = 1
	}
	return b
}

// v_nan64: 64-byte layout, naturally 8-byte aligned. Absent battery encoded as a
// NaN sentinel, removing the trailing flag byte entirely.
func v_nan64(p Point, seq uint64) []byte {
	b := make([]byte, 64)
	binary.BigEndian.PutUint64(b[0:8], uint64(p.TS))
	binary.BigEndian.PutUint64(b[8:16], seq)
	binary.LittleEndian.PutUint64(b[16:24], math.Float64bits(p.Lat))
	binary.LittleEndian.PutUint64(b[24:32], math.Float64bits(p.Lon))
	binary.LittleEndian.PutUint64(b[32:40], math.Float64bits(p.Ax))
	binary.LittleEndian.PutUint64(b[40:48], math.Float64bits(p.Ay))
	binary.LittleEndian.PutUint64(b[48:56], math.Float64bits(p.Az))
	bat := math.Float64bits(p.Battery)
	if !p.HasBattery {
		bat = absentNaN
	}
	binary.LittleEndian.PutUint64(b[56:64], bat)
	return b
}

func v_nan64Into(dst []byte, p Point, seq uint64) {
	binary.BigEndian.PutUint64(dst[0:8], uint64(p.TS))
	binary.BigEndian.PutUint64(dst[8:16], seq)
	binary.LittleEndian.PutUint64(dst[16:24], math.Float64bits(p.Lat))
	binary.LittleEndian.PutUint64(dst[24:32], math.Float64bits(p.Lon))
	binary.LittleEndian.PutUint64(dst[32:40], math.Float64bits(p.Ax))
	binary.LittleEndian.PutUint64(dst[40:48], math.Float64bits(p.Ay))
	binary.LittleEndian.PutUint64(dst[48:56], math.Float64bits(p.Az))
	bat := math.Float64bits(p.Battery)
	if !p.HasBattery {
		bat = absentNaN
	}
	binary.LittleEndian.PutUint64(dst[56:64], bat)
}

// absentNaN is a quiet NaN payload reserved to mean "battery not reported".
const absentNaN = 0x7FF8000000000001

func bswap64(v uint64) uint64 {
	return (v&0x00000000000000FF)<<56 | (v&0x000000000000FF00)<<40 |
		(v&0x0000000000FF0000)<<24 | (v&0x00000000FF000000)<<8 |
		(v&0x000000FF00000000)>>8 | (v&0x0000FF0000000000)>>24 |
		(v&0x00FF000000000000)>>40 | (v&0xFF00000000000000)>>56
}

// --- harness ---------------------------------------------------------------

func TestEncodeVariants(t *testing.T) {
	p := samplePoint()
	buf65 := make([]byte, 65)
	buf64 := make([]byte, 64)

	type variant struct {
		name string
		size int
		fn   func()
	}
	vs := []variant{
		{"current (Encode)", 65, func() { sinkBytes = p.Encode(1) }},
		{"native-endian", 65, func() { sinkBytes = v_nativeEndian(p, 1) }},
		{"into-buffer (no alloc)", 65, func() { v_into(buf65, p, 1) }},
		{"unsafe array store", 65, func() { sinkBytes = v_unsafe(p, 1) }},
		{"64B nan-sentinel", 64, func() { sinkBytes = v_nan64(p, 1) }},
		{"64B into-buffer", 64, func() { v_nan64Into(buf64, p, 1) }},
	}

	// Correctness: every variant that produces a slice must still decode back to
	// the same point (current-layout variants only; 64B uses its own decode).
	verifyVariants(t, p)

	fmt.Printf("\n%-26s %5s %8s %8s %8s %9s %9s %8s\n",
		"variant", "bytes", "p50", "p90", "p99", "p99.9", "mean", "allocs")
	fmt.Println("-------------------------------------------------------------------------------------------")
	for _, v := range vs {
		lat, allocs := measureWithAllocs(v.fn)
		reportVariant(v.name, v.size, lat, allocs)
	}
	fmt.Printf("\n(%d iters/variant, native linux/arm64 OrbStack)\n", variantIters)
}

func verifyVariants(t *testing.T, p Point) {
	t.Helper()
	// 65-byte variants share Decode.
	for name, enc := range map[string]func(Point, uint64) []byte{
		"native-endian": v_nativeEndian,
		"unsafe":        v_unsafe,
	} {
		got, err := Decode(enc(p, 1))
		if err != nil || got != p {
			t.Fatalf("%s: round-trip mismatch: got %+v err %v", name, got, err)
		}
	}
	// into-buffer
	buf := make([]byte, 65)
	v_into(buf, p, 1)
	if got, err := Decode(buf); err != nil || got != p {
		t.Fatalf("into-buffer: round-trip mismatch: got %+v err %v", got, err)
	}
	// 64B nan-sentinel decode (local, since production Decode expects 65).
	if got := decodeNan64(v_nan64(p, 1)); got != p {
		t.Fatalf("64B nan: round-trip mismatch: got %+v want %+v", got, p)
	}
	// Absent battery survives the NaN sentinel.
	noBat := Point{TS: 10, Lat: 1, Lon: 2, Ax: 3, Ay: 4, Az: 5}
	if got := decodeNan64(v_nan64(noBat, 1)); got != noBat {
		t.Fatalf("64B nan absent-battery mismatch: got %+v want %+v", got, noBat)
	}
}

func decodeNan64(b []byte) Point {
	p := Point{
		TS:  int64(binary.BigEndian.Uint64(b[0:8])),
		Lat: math.Float64frombits(binary.LittleEndian.Uint64(b[16:24])),
		Lon: math.Float64frombits(binary.LittleEndian.Uint64(b[24:32])),
		Ax:  math.Float64frombits(binary.LittleEndian.Uint64(b[32:40])),
		Ay:  math.Float64frombits(binary.LittleEndian.Uint64(b[40:48])),
		Az:  math.Float64frombits(binary.LittleEndian.Uint64(b[48:56])),
	}
	bits := binary.LittleEndian.Uint64(b[56:64])
	if bits != absentNaN {
		p.Battery = math.Float64frombits(bits)
		p.HasBattery = true
	}
	return p
}

func measureWithAllocs(fn func()) ([]float64, float64) {
	for i := 0; i < 50_000; i++ {
		fn()
	}
	var ms0 runtime.MemStats
	runtime.ReadMemStats(&ms0)
	lat := make([]float64, variantIters)
	for i := 0; i < variantIters; i++ {
		start := time.Now()
		fn()
		lat[i] = float64(time.Since(start).Nanoseconds())
	}
	var ms1 runtime.MemStats
	runtime.ReadMemStats(&ms1)
	allocs := float64(ms1.Mallocs-ms0.Mallocs) / float64(variantIters)
	return lat, allocs
}

func reportVariant(name string, size int, lat []float64, allocs float64) {
	sort.Float64s(lat)
	var sum float64
	for _, v := range lat {
		sum += v
	}
	fmt.Printf("%-26s %5d %7.0fn %7.0fn %7.0fn %8.0fn %8.1fn %7.2f\n",
		name, size,
		pct(lat, 0.50), pct(lat, 0.90), pct(lat, 0.99), pct(lat, 0.999),
		sum/float64(len(lat)), allocs)
}
