package store

import (
	"bytes"
	"context"
	"encoding/binary"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/araki/pibench/internal/cursor"
	"github.com/araki/pibench/internal/model"
)

// memStore is an in-memory Store mirroring the Redis ZSET layout: per device, a
// slice of encoded members kept sorted by (ts, seq) via raw byte order, capped
// to the newest `cap` entries. Used for handler tests where TCP is unavailable.
type memStore struct {
	mu  sync.RWMutex
	cap int
	seq atomic.Uint64
	dev map[string][][]byte
}

// NewMem returns an in-memory store retaining at most cap points per device.
func NewMem(cap int) Store {
	return &memStore{cap: cap, dev: make(map[string][][]byte)}
}

func (m *memStore) Ping(context.Context) error { return nil }
func (m *memStore) Close() error               { return nil }

func memTS(member []byte) int64 {
	return int64(binary.BigEndian.Uint64(member[0:8]))
}

func (m *memStore) Append(_ context.Context, id string, pts []model.Point) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	members := m.dev[id]
	for _, p := range pts {
		enc := p.Encode(m.seq.Add(1))
		i := sort.Search(len(members), func(i int) bool {
			return bytes.Compare(members[i], enc) >= 0
		})
		members = append(members, nil)
		copy(members[i+1:], members[i:])
		members[i] = enc
	}
	if len(members) > m.cap {
		members = append([][]byte(nil), members[len(members)-m.cap:]...)
	}
	m.dev[id] = members
	return len(pts), nil
}

func (m *memStore) Range(_ context.Context, id string, from, to int64, limit int, cur string) ([]model.Point, string, error) {
	curTs, curSkip := from, 0
	if cur != "" {
		ts, skip, err := cursor.Decode(cur)
		if err != nil {
			return nil, "", err
		}
		curTs, curSkip = ts, skip
	}

	m.mu.RLock()
	members := m.dev[id]
	var window [][]byte
	for _, mem := range members {
		ts := memTS(mem)
		if ts >= curTs && ts <= to {
			window = append(window, mem)
		}
	}
	m.mu.RUnlock()

	fetched := make([]model.Point, 0, limit+1)
	for i := curSkip; i < len(window) && len(fetched) < limit+1; i++ {
		p, err := model.Decode(window[i])
		if err != nil {
			return nil, "", err
		}
		fetched = append(fetched, p)
	}
	page, next := BuildPage(fetched, limit, curTs, curSkip)
	return page, next, nil
}

func (m *memStore) Recent(_ context.Context, id string, n int) ([]model.Point, error) {
	m.mu.RLock()
	members := m.dev[id]
	start := len(members) - n
	if start < 0 {
		start = 0
	}
	tail := append([][]byte(nil), members[start:]...)
	m.mu.RUnlock()

	out := make([]model.Point, 0, len(tail))
	for i := len(tail) - 1; i >= 0; i-- { // most-recent-first
		p, err := model.Decode(tail[i])
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}
