package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/araki/pibench/internal/metrics"
	"github.com/araki/pibench/internal/store"
)

func newTestHandler(t *testing.T) http.Handler {
	t.Helper()
	cfg := Config{InstanceID: "inst-1", SingleMaxBytes: 4096, BatchMaxBytes: 131072}
	return New(store.NewMem(1000), metrics.New(), cfg)
}

func do(t *testing.T, h http.Handler, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, target, nil)
	} else {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

const validPoint = `{"ts":1715800000000,"lat":-23.5,"lon":-46.6,"battery":0.8,"ax":0.1,"ay":-0.04,"az":9.81}`

func TestHealthz(t *testing.T) {
	w := do(t, newTestHandler(t), "GET", "/healthz", "")
	if w.Code != 200 {
		t.Errorf("healthz: got %d want 200", w.Code)
	}
	// The contract smoke test asserts the healthz body contains "ok".
	if !strings.Contains(w.Body.String(), "ok") {
		t.Errorf("healthz body: got %q, want it to contain \"ok\"", w.Body.String())
	}
}

func TestReadyz(t *testing.T) {
	w := do(t, newTestHandler(t), "GET", "/readyz", "")
	if w.Code != 200 {
		t.Errorf("readyz: got %d want 200", w.Code)
	}
}

func TestMetricsEndpoint(t *testing.T) {
	w := do(t, newTestHandler(t), "GET", "/metrics", "")
	if w.Code != 200 {
		t.Fatalf("metrics: got %d want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("metrics content-type: got %q", ct)
	}
}

func TestInstanceHeaderOnEveryResponse(t *testing.T) {
	h := newTestHandler(t)
	for _, target := range []string{"/healthz", "/readyz", "/metrics"} {
		w := do(t, h, "GET", target, "")
		if got := w.Header().Get("X-Instance-Id"); got != "inst-1" {
			t.Errorf("%s X-Instance-Id: got %q want inst-1", target, got)
		}
	}
}

func TestIngestSingleAccepted(t *testing.T) {
	w := do(t, newTestHandler(t), "POST", "/devices/dev1/telemetry", validPoint)
	if w.Code != 202 {
		t.Errorf("ingest: got %d want 202", w.Code)
	}
	if w.Body.Len() != 0 {
		t.Errorf("ingest body: got %q want empty", w.Body.String())
	}
}

func TestIngestInvalidDeviceID(t *testing.T) {
	w := do(t, newTestHandler(t), "POST", "/devices/bad*id/telemetry", validPoint)
	if w.Code != 400 {
		t.Errorf("bad id: got %d want 400", w.Code)
	}
}

func TestIngestInvalidPayload(t *testing.T) {
	w := do(t, newTestHandler(t), "POST", "/devices/dev1/telemetry", `{"lat":1}`)
	if w.Code != 400 {
		t.Errorf("bad payload: got %d want 400", w.Code)
	}
}

func TestIngestPayloadTooLarge(t *testing.T) {
	cfg := Config{InstanceID: "i", SingleMaxBytes: 16, BatchMaxBytes: 32}
	h := New(store.NewMem(10), metrics.New(), cfg)
	w := do(t, h, "POST", "/devices/dev1/telemetry", validPoint)
	if w.Code != 413 {
		t.Errorf("too large: got %d want 413", w.Code)
	}
}

func TestBatchAccepted(t *testing.T) {
	body := `{"points":[` + validPoint + `,` + validPoint + `]}`
	w := do(t, newTestHandler(t), "POST", "/devices/dev1/telemetry/batch", body)
	if w.Code != 202 {
		t.Fatalf("batch: got %d want 202", w.Code)
	}
	var resp struct {
		Accepted int `json:"accepted"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Accepted != 2 {
		t.Errorf("accepted: got %d want 2", resp.Accepted)
	}
}

func TestBatchEmptyRejected(t *testing.T) {
	w := do(t, newTestHandler(t), "POST", "/devices/dev1/telemetry/batch", `{"points":[]}`)
	if w.Code != 400 {
		t.Errorf("empty batch: got %d want 400", w.Code)
	}
}

func TestBatchOverLimitRejected(t *testing.T) {
	pts := make([]string, 101)
	for i := range pts {
		pts[i] = validPoint
	}
	body := `{"points":[` + strings.Join(pts, ",") + `]}`
	w := do(t, newTestHandler(t), "POST", "/devices/dev1/telemetry/batch", body)
	if w.Code != 413 {
		t.Errorf("over-limit batch: got %d want 413", w.Code)
	}
}

func TestBatchInvalidPointRejected(t *testing.T) {
	body := `{"points":[` + validPoint + `,{"ts":1,"lat":999,"lon":0,"ax":0,"ay":0,"az":0}]}`
	w := do(t, newTestHandler(t), "POST", "/devices/dev1/telemetry/batch", body)
	if w.Code != 400 {
		t.Errorf("invalid point in batch: got %d want 400", w.Code)
	}
}

func TestRangeReturnsAscending(t *testing.T) {
	h := newTestHandler(t)
	for _, ts := range []int64{3, 1, 2} {
		p := strings.Replace(validPoint, "1715800000000", itoa(ts), 1)
		do(t, h, "POST", "/devices/d/telemetry", p)
	}
	w := do(t, h, "GET", "/devices/d/telemetry?from=0&to=10", "")
	if w.Code != 200 {
		t.Fatalf("range: got %d want 200", w.Code)
	}
	var resp struct {
		Points []struct {
			TS int64 `json:"ts"`
		} `json:"points"`
		NextCursor *string `json:"next_cursor"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Points) != 3 || resp.Points[0].TS != 1 || resp.Points[2].TS != 3 {
		t.Errorf("range order wrong: %+v", resp.Points)
	}
}

func TestRangeMissingParams(t *testing.T) {
	w := do(t, newTestHandler(t), "GET", "/devices/d/telemetry?from=0", "")
	if w.Code != 400 {
		t.Errorf("missing to: got %d want 400", w.Code)
	}
}

func TestRangeFromGreaterThanTo(t *testing.T) {
	w := do(t, newTestHandler(t), "GET", "/devices/d/telemetry?from=10&to=5", "")
	if w.Code != 400 {
		t.Errorf("from>to: got %d want 400", w.Code)
	}
}

func TestRangeInvalidLimit(t *testing.T) {
	w := do(t, newTestHandler(t), "GET", "/devices/d/telemetry?from=0&to=10&limit=999", "")
	if w.Code != 400 {
		t.Errorf("limit out of range: got %d want 400", w.Code)
	}
}

func TestAnomalyInsufficient(t *testing.T) {
	h := newTestHandler(t)
	do(t, h, "POST", "/devices/d/telemetry", validPoint)
	w := do(t, h, "GET", "/devices/d/anomaly", "")
	if w.Code != 404 {
		t.Errorf("anomaly insufficient: got %d want 404", w.Code)
	}
}

func TestAnomalySufficient(t *testing.T) {
	h := newTestHandler(t)
	for i := 0; i < 10; i++ {
		p := strings.Replace(validPoint, "1715800000000", itoa(int64(1715800000000+i)), 1)
		do(t, h, "POST", "/devices/d/telemetry", p)
	}
	w := do(t, h, "GET", "/devices/d/anomaly", "")
	if w.Code != 200 {
		t.Fatalf("anomaly: got %d want 200", w.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"z_score", "samples", "anomalous"} {
		if _, ok := resp[k]; !ok {
			t.Errorf("anomaly response missing %q", k)
		}
	}
}

func itoa(v int64) string { return strconv.FormatInt(v, 10) }
