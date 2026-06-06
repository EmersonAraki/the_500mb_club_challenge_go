package store

import (
	"testing"

	"github.com/araki/pibench/internal/cursor"
	"github.com/araki/pibench/internal/model"
)

func tsPts(tss ...int64) []model.Point {
	out := make([]model.Point, len(tss))
	for i, ts := range tss {
		out[i] = model.Point{TS: ts}
	}
	return out
}

func TestBuildPageLastPageNoCursor(t *testing.T) {
	// fetched <= limit -> everything fits, no next cursor.
	page, next := BuildPage(tsPts(1, 2, 3), 5, 1, 0)
	if len(page) != 3 || next != "" {
		t.Errorf("got len=%d next=%q want 3,\"\"", len(page), next)
	}
}

func TestBuildPageTruncatesAndEmitsCursor(t *testing.T) {
	// fetched limit+1 -> return limit, cursor points past last returned ts.
	page, next := BuildPage(tsPts(10, 20, 30, 40), 3, 10, 0)
	if len(page) != 3 {
		t.Fatalf("page len: got %d want 3", len(page))
	}
	if next == "" {
		t.Fatal("expected a next cursor")
	}
	ts, skip, err := cursor.Decode(next)
	if err != nil {
		t.Fatal(err)
	}
	if ts != 30 || skip != 1 {
		t.Errorf("cursor: got ts=%d skip=%d want 30,1", ts, skip)
	}
}

func TestBuildPageAccumulatesSkipForSameTs(t *testing.T) {
	// All returned items share the cursor's ts; skip accumulates across pages.
	page, next := BuildPage(tsPts(30, 30, 30, 30), 3, 30, 2)
	if len(page) != 3 {
		t.Fatalf("page len: got %d want 3", len(page))
	}
	ts, skip, err := cursor.Decode(next)
	if err != nil {
		t.Fatal(err)
	}
	// previous skip 2 + 3 emitted at ts 30 = 5
	if ts != 30 || skip != 5 {
		t.Errorf("cursor: got ts=%d skip=%d want 30,5", ts, skip)
	}
}

func TestBuildPageSkipCountsOnlyTrailingTsRun(t *testing.T) {
	// last ts (40) appears twice at the tail; skip resets to that run length.
	page, next := BuildPage(tsPts(20, 40, 40, 50), 3, 20, 0)
	if len(page) != 3 {
		t.Fatalf("page len: got %d want 3", len(page))
	}
	ts, skip, err := cursor.Decode(next)
	if err != nil {
		t.Fatal(err)
	}
	if ts != 40 || skip != 2 {
		t.Errorf("cursor: got ts=%d skip=%d want 40,2", ts, skip)
	}
}
