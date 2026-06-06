package store

import (
	"context"
	"testing"

	"github.com/araki/pibench/internal/model"
)

func ctx() context.Context { return context.Background() }

func TestMemAppendAndRecentMostRecentFirst(t *testing.T) {
	s := NewMem(100)
	n, err := s.Append(ctx(), "dev1", tsPts(1, 2, 3))
	if err != nil || n != 3 {
		t.Fatalf("append: n=%d err=%v want 3,nil", n, err)
	}
	got, err := s.Recent(ctx(), "dev1", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].TS != 3 || got[1].TS != 2 {
		t.Errorf("recent: got %v want [3,2]", tsOf(got))
	}
}

func TestMemHandlesOutOfOrderInsertion(t *testing.T) {
	s := NewMem(100)
	s.Append(ctx(), "d", tsPts(3))
	s.Append(ctx(), "d", tsPts(1))
	s.Append(ctx(), "d", tsPts(2))
	page, _, err := s.Range(ctx(), "d", 0, 10, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := tsOf(page); !equalTs(got, []int64{1, 2, 3}) {
		t.Errorf("range ascending: got %v want [1 2 3]", got)
	}
}

func TestMemCapKeepsNewest(t *testing.T) {
	s := NewMem(2)
	s.Append(ctx(), "d", tsPts(1, 2, 3, 4))
	got, _ := s.Recent(ctx(), "d", 10)
	if !equalTs(tsOf(got), []int64{4, 3}) {
		t.Errorf("cap: got %v want newest [4 3]", tsOf(got))
	}
}

func TestMemRangeWindow(t *testing.T) {
	s := NewMem(100)
	s.Append(ctx(), "d", tsPts(1, 5, 10, 15, 20))
	page, _, _ := s.Range(ctx(), "d", 5, 15, 100, "")
	if !equalTs(tsOf(page), []int64{5, 10, 15}) {
		t.Errorf("window: got %v want [5 10 15]", tsOf(page))
	}
}

func TestMemRangePaginationReconstructsFullSet(t *testing.T) {
	s := NewMem(100)
	s.Append(ctx(), "d", tsPts(1, 2, 3, 4, 5, 6, 7))
	var all []int64
	cur := ""
	for {
		page, next, err := s.Range(ctx(), "d", 0, 100, 2, cur)
		if err != nil {
			t.Fatal(err)
		}
		if len(page) > 2 {
			t.Fatalf("page exceeded limit: %d", len(page))
		}
		all = append(all, tsOf(page)...)
		if next == "" {
			break
		}
		cur = next
	}
	if !equalTs(all, []int64{1, 2, 3, 4, 5, 6, 7}) {
		t.Errorf("paginated set: got %v", all)
	}
}

func TestMemRangePaginationWithDuplicateTimestamps(t *testing.T) {
	s := NewMem(100)
	// five points all at the same ts must paginate without loss or repeat.
	s.Append(ctx(), "d", tsPts(7, 7, 7, 7, 7))
	count := 0
	cur := ""
	for {
		page, next, err := s.Range(ctx(), "d", 0, 100, 2, cur)
		if err != nil {
			t.Fatal(err)
		}
		count += len(page)
		if next == "" {
			break
		}
		cur = next
	}
	if count != 5 {
		t.Errorf("duplicate-ts pagination: got %d points want 5", count)
	}
}

func TestMemRangeInvalidCursorErrors(t *testing.T) {
	s := NewMem(100)
	s.Append(ctx(), "d", tsPts(1))
	if _, _, err := s.Range(ctx(), "d", 0, 10, 10, "!!bad!!"); err == nil {
		t.Error("expected error for invalid cursor")
	}
}

func tsOf(pts []model.Point) []int64 {
	out := make([]int64, len(pts))
	for i, p := range pts {
		out[i] = p.TS
	}
	return out
}

func equalTs(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
