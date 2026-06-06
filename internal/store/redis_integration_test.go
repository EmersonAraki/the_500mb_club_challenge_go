//go:build integration

// These tests run against a real Redis. Provide REDIS_ADDR (default
// 127.0.0.1:6379) and run: go test -tags=integration ./internal/store/
package store

import (
	"context"
	"os"
	"testing"
)

func redisStoreForTest(t *testing.T, cap int) Store {
	t.Helper()
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "127.0.0.1:6379"
	}
	s := NewRedis(addr, 4, cap)
	if err := s.Ping(context.Background()); err != nil {
		t.Skipf("redis not reachable at %s: %v", addr, err)
	}
	return s
}

// uniqueID isolates each test's data in the shared instance.
func uniqueID(t *testing.T) string { return "itest-" + t.Name() }

func TestRedisAppendRecentAndRange(t *testing.T) {
	s := redisStoreForTest(t, 1000)
	id := uniqueID(t)
	if _, err := s.Append(ctx(), id, tsPts(3, 1, 2)); err != nil {
		t.Fatal(err)
	}
	recent, err := s.Recent(ctx(), id, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !equalTs(tsOf(recent), []int64{3, 2}) {
		t.Errorf("recent: got %v want [3 2]", tsOf(recent))
	}
	page, _, err := s.Range(ctx(), id, 0, 10, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	if !equalTs(tsOf(page), []int64{1, 2, 3}) {
		t.Errorf("range: got %v want [1 2 3]", tsOf(page))
	}
}

func TestRedisCapBoundsAndKeepsNewest(t *testing.T) {
	// The cap is amortized (trim runs ~1/trimEvery writes), so this asserts the
	// soft contract: history stays bounded well below the insert count and the
	// newest point is always retained.
	s := redisStoreForTest(t, 2)
	id := uniqueID(t)
	for i := int64(1); i <= 100; i++ {
		if _, err := s.Append(ctx(), id, tsPts(i)); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.Recent(ctx(), id, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 || got[0].TS != 100 {
		t.Errorf("newest not retained: got %v", tsOf(got))
	}
	if len(got) > 2+32 { // cap + generous trim slack proves trimming occurs
		t.Errorf("history not bounded: %d points retained for cap 2", len(got))
	}
}

func TestRedisPaginationReconstructsFullSet(t *testing.T) {
	s := redisStoreForTest(t, 1000)
	id := uniqueID(t)
	s.Append(ctx(), id, tsPts(1, 2, 3, 4, 5, 6, 7))
	var all []int64
	cur := ""
	for {
		page, next, err := s.Range(ctx(), id, 0, 100, 2, cur)
		if err != nil {
			t.Fatal(err)
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
