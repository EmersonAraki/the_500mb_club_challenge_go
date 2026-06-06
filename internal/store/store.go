// Package store persists telemetry points. The production backend is Redis
// (sorted set per device); an in-memory backend mirrors its semantics for tests.
package store

import (
	"context"

	"github.com/araki/pibench/internal/model"
)

// Store is the persistence contract used by the HTTP handlers.
type Store interface {
	// Ping verifies the backend is reachable (drives /readyz).
	Ping(ctx context.Context) error
	// Append persists points for a device and returns how many were accepted.
	Append(ctx context.Context, id string, pts []model.Point) (int, error)
	// Range returns points with from <= ts <= to in ascending ts order, at most
	// limit, plus an opaque next cursor ("" when exhausted).
	Range(ctx context.Context, id string, from, to int64, limit int, cur string) ([]model.Point, string, error)
	// Recent returns up to n newest points, most-recent-first.
	Recent(ctx context.Context, id string, n int) ([]model.Point, error)
	// Close releases backend resources.
	Close() error
}
