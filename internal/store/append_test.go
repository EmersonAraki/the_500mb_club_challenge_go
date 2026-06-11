package store

import (
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/araki/pibench/internal/model"
)

// benchmark the ZADD-args build for a full 100-point batch (the contract max),
// old per-point Encode + 2-alloc itoa vs the new shared-backing encodeMembers.
// Run: ./scripts/store-bench.sh  (or: go test ./internal/store -bench EncodeBatch -benchmem)

func BenchmarkEncodeBatch_New(b *testing.B) {
	pts := samplePoints(100)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var seq atomic.Uint64
		_ = encodeMembers(make([][]byte, 0, 2+2*len(pts)), pts, &seq)
	}
}

func BenchmarkEncodeBatch_Old(b *testing.B) {
	pts := samplePoints(100)
	oldItoa := func(v int64) []byte { return []byte(strconv.FormatInt(v, 10)) }
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var seq atomic.Uint64
		args := make([][]byte, 0, 2+2*len(pts))
		for _, p := range pts {
			args = append(args, oldItoa(p.TS), p.Encode(seq.Add(1)))
		}
		_ = args
	}
}

func samplePoints(n int) []model.Point {
	pts := make([]model.Point, n)
	for i := range pts {
		pts[i] = model.Point{
			TS:  int64(1_700_000_000_000 + i),
			Lat: -23.5, Lon: -46.6, Ax: 0.1, Ay: 0.2, Az: 9.8,
			HasBattery: true, Battery: 0.5,
		}
	}
	return pts
}

// encodeMembers must interleave score (ts) and binary member for each point so
// the result decodes back to the originals in order.
func TestEncodeMembersRoundTrips(t *testing.T) {
	pts := samplePoints(3)
	var seq atomic.Uint64
	args := encodeMembers(nil, pts, &seq)

	if len(args) != 2*len(pts) {
		t.Fatalf("got %d args, want %d (score+member per point)", len(args), 2*len(pts))
	}
	for i, p := range pts {
		member := args[2*i+1]
		got, err := model.Decode(member)
		if err != nil {
			t.Fatalf("point %d: decode: %v", i, err)
		}
		if got != p {
			t.Fatalf("point %d: got %+v want %+v", i, got, p)
		}
	}
}

// All member bytes for a batch must come from a single backing array, so a
// batch of N points costs one member allocation instead of N. Asserted via the
// allocation counter: backing(1) + one itoa per point for the score, and no
// per-point member allocation.
func TestEncodeMembersSharesOneBacking(t *testing.T) {
	const n = 4
	pts := samplePoints(n)
	got := testing.AllocsPerRun(100, func() {
		var seq atomic.Uint64
		_ = encodeMembers(make([][]byte, 0, 2*n), pts, &seq)
	})
	want := float64(n + 1) // 1 backing + n score itoas; if members alloc'd it'd be ~2n
	if got > want {
		t.Fatalf("encodeMembers allocs/op = %v, want <= %v (members must share one backing array)", got, want)
	}
}
