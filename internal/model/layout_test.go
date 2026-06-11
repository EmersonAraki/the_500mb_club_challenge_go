package model

// Byte-for-byte verification of the documented 65-byte member layout:
//
//	bytes  0–7  : big-endian    TS       (members sort by ts on score ties)
//	bytes  8–15 : big-endian    seq      (uniqueness across concurrent writers)
//	bytes 16–63 : little-endian float64  lat,lon,ax,ay,az,battery (6 x 8B)
//	byte  64    : battery-present flag
//
// Each claim is asserted independently so a failure pinpoints which field's
// offset or endianness drifted.

import (
	"bytes"
	"encoding/binary"
	"math"
	"sort"
	"testing"
)

func layoutPoint() Point {
	return Point{
		TS:         0x0102030405060708, // distinct bytes -> catches endianness
		Lat:        -23.55052,
		Lon:        -46.633308,
		Battery:    0.87,
		HasBattery: true,
		Ax:         0.012, Ay: -0.034, Az: 9.81,
	}
}

func TestLayout_TotalSize(t *testing.T) {
	if got := len(layoutPoint().Encode(1)); got != 65 {
		t.Fatalf("member size = %d, want 65", got)
	}
	if EncodedLen != 65 {
		t.Fatalf("EncodedLen = %d, want 65", EncodedLen)
	}
}

func TestLayout_Bytes0to7_BigEndianTS(t *testing.T) {
	const seq = 0x1122334455667788
	b := layoutPoint().Encode(seq)
	got := binary.BigEndian.Uint64(b[0:8])
	if got != uint64(layoutPoint().TS) {
		t.Fatalf("ts (big-endian) = %#x, want %#x", got, uint64(layoutPoint().TS))
	}
	// Prove it is big-endian, not little: first byte is the most-significant.
	if b[0] != 0x01 || b[7] != 0x08 {
		t.Fatalf("ts not big-endian: b[0]=%#x b[7]=%#x, want 0x01..0x08", b[0], b[7])
	}
}

func TestLayout_Bytes8to15_BigEndianSeq(t *testing.T) {
	const seq = 0x1122334455667788
	b := layoutPoint().Encode(seq)
	if got := binary.BigEndian.Uint64(b[8:16]); got != seq {
		t.Fatalf("seq (big-endian) = %#x, want %#x", got, uint64(seq))
	}
	if b[8] != 0x11 || b[15] != 0x88 {
		t.Fatalf("seq not big-endian: b[8]=%#x b[15]=%#x, want 0x11..0x88", b[8], b[15])
	}
}

func TestLayout_Bytes16to63_LittleEndianFloats(t *testing.T) {
	p := layoutPoint()
	b := p.Encode(1)
	fields := []struct {
		name   string
		lo, hi int
		want   float64
	}{
		{"lat", 16, 24, p.Lat},
		{"lon", 24, 32, p.Lon},
		{"ax", 32, 40, p.Ax},
		{"ay", 40, 48, p.Ay},
		{"az", 48, 56, p.Az},
		{"battery", 56, 64, p.Battery},
	}
	for _, f := range fields {
		bits := binary.LittleEndian.Uint64(b[f.lo:f.hi])
		if got := math.Float64frombits(bits); got != f.want {
			t.Errorf("%s @ [%d:%d] little-endian = %v, want %v", f.name, f.lo, f.hi, got, f.want)
		}
		// Confirm little-endian: big-endian read of the same range must differ
		// (true for every non-palindromic bit pattern, which all our values are).
		if binary.BigEndian.Uint64(b[f.lo:f.hi]) == bits {
			t.Errorf("%s @ [%d:%d] ambiguous endianness", f.name, f.lo, f.hi)
		}
	}
}

func TestLayout_Byte64_BatteryFlag(t *testing.T) {
	withBat := Point{TS: 1, HasBattery: true, Battery: 0.5}
	if b := withBat.Encode(1); b[64] != 1 {
		t.Fatalf("battery present: b[64] = %d, want 1", b[64])
	}
	noBat := Point{TS: 1, HasBattery: false}
	if b := noBat.Encode(1); b[64] != 0 {
		t.Fatalf("battery absent: b[64] = %d, want 0", b[64])
	}
}

// TestLayout_RoundTrip verifies Encode then Decode reconstructs the point,
// including the battery flag (but note seq is intentionally not recovered).
func TestLayout_RoundTrip(t *testing.T) {
	p := layoutPoint()
	got, err := Decode(p.Encode(42))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got != p {
		t.Fatalf("round-trip mismatch:\n got %+v\nwant %+v", got, p)
	}
}

// TestLayout_SortByTimestamp is the load-bearing property: because TS is the
// big-endian prefix, raw byte (memcmp) ordering of members equals numeric
// ordering of timestamps. This is what makes ZSET score ties resolve by ts.
func TestLayout_SortByTimestamp(t *testing.T) {
	tss := []int64{5, 1, 1_000_000, 0xFF, 256, 2}
	enc := make([][]byte, len(tss))
	for i, ts := range tss {
		enc[i] = Point{TS: ts}.Encode(1)
	}
	sort.Slice(enc, func(i, j int) bool { return bytes.Compare(enc[i], enc[j]) < 0 })

	sortedTS := append([]int64(nil), tss...)
	sort.Slice(sortedTS, func(i, j int) bool { return sortedTS[i] < sortedTS[j] })

	for i, want := range sortedTS {
		if got := int64(binary.BigEndian.Uint64(enc[i][0:8])); got != want {
			t.Fatalf("byte-order position %d = ts %d, want %d (memcmp != numeric order)", i, got, want)
		}
	}
}

// TestLayout_SeqTiebreak verifies that for an identical timestamp, the seq
// prefix gives a stable, unique secondary ordering across concurrent writers.
func TestLayout_SeqTiebreak(t *testing.T) {
	const ts = 777
	a := Point{TS: ts}.Encode(1)
	b := Point{TS: ts}.Encode(2)
	if bytes.Equal(a, b) {
		t.Fatal("same ts + different seq produced identical members (collision)")
	}
	if bytes.Compare(a, b) >= 0 {
		t.Fatal("seq=1 should sort before seq=2 on equal ts")
	}
	// And differing seq must not perturb ts ordering.
	low := Point{TS: ts - 1}.Encode(9_999_999)
	if bytes.Compare(low, a) >= 0 {
		t.Fatal("lower ts must sort first regardless of higher seq")
	}
}
