package httpapi

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/araki/pibench/internal/metrics"
	"github.com/araki/pibench/internal/model"
	"github.com/araki/pibench/internal/store"
)

// errStore is a Store whose read methods always fail with a fixed error, used to
// exercise the read-path 400/503 split and the deadline metric without Redis.
type errStore struct{ err error }

func (s errStore) Ping(context.Context) error                                 { return s.err }
func (s errStore) Append(context.Context, string, []model.Point) (int, error) { return 0, s.err }
func (s errStore) Range(context.Context, string, int64, int64, int, string) ([]model.Point, string, error) {
	return nil, "", s.err
}
func (s errStore) Recent(context.Context, string, int) ([]model.Point, error) { return nil, s.err }
func (s errStore) Close() error                                               { return nil }

func handlerWith(s store.Store, reg *metrics.Registry) *Handler {
	return New(s, reg, Config{InstanceID: "i", SingleMaxBytes: 4096, BatchMaxBytes: 131072, ReadTimeout: 250 * time.Millisecond})
}

func TestQueryDeadlineReturns503(t *testing.T) {
	h := handlerWith(errStore{context.DeadlineExceeded}, metrics.New())
	w := do(t, h, "GET", "/devices/d/telemetry?from=0&to=10", "")
	if w.Code != 503 {
		t.Errorf("deadline on range: got %d want 503", w.Code)
	}
}

func TestQueryGenericErrorReturns503(t *testing.T) {
	h := handlerWith(errStore{errors.New("connection refused")}, metrics.New())
	w := do(t, h, "GET", "/devices/d/telemetry?from=0&to=10", "")
	if w.Code != 503 {
		t.Errorf("conn error on range: got %d want 503", w.Code)
	}
}

func TestQueryBadCursorReturns400(t *testing.T) {
	// A malformed cursor is the only user-driven error on the read path.
	h := handlerWith(store.NewMem(10), metrics.New())
	w := do(t, h, "GET", "/devices/d/telemetry?from=0&to=10&cursor=not%20base64%21", "")
	if w.Code != 400 {
		t.Errorf("bad cursor: got %d want 400", w.Code)
	}
}

func TestAnomalyDeadlineReturns503(t *testing.T) {
	h := handlerWith(errStore{context.DeadlineExceeded}, metrics.New())
	w := do(t, h, "GET", "/devices/d/anomaly", "")
	if w.Code != 503 {
		t.Errorf("deadline on anomaly: got %d want 503", w.Code)
	}
}

func TestReadTimeoutMetricCountsOnlyDeadlines(t *testing.T) {
	reg := metrics.New()
	h := handlerWith(errStore{context.DeadlineExceeded}, reg)
	do(t, h, "GET", "/devices/d/telemetry?from=0&to=10", "")
	do(t, h, "GET", "/devices/d/anomaly", "")
	if got := scrapeMetric(t, h, "pibench_redis_read_timeout_total"); got != "2" {
		t.Errorf("read timeout metric after 2 deadlines: got %q want 2", got)
	}

	// A non-deadline error must not bump the timeout counter.
	reg2 := metrics.New()
	h2 := handlerWith(errStore{errors.New("conn refused")}, reg2)
	do(t, h2, "GET", "/devices/d/telemetry?from=0&to=10", "")
	if got := scrapeMetric(t, h2, "pibench_redis_read_timeout_total"); got != "0" {
		t.Errorf("read timeout metric after conn error: got %q want 0", got)
	}
}

func scrapeMetric(t *testing.T, h *Handler, name string) string {
	t.Helper()
	w := do(t, h, "GET", "/metrics", "")
	for _, line := range strings.Split(w.Body.String(), "\n") {
		if strings.HasPrefix(line, name+" ") {
			return strings.TrimSpace(strings.TrimPrefix(line, name))
		}
	}
	t.Fatalf("metric %q not found in exposition", name)
	return ""
}
