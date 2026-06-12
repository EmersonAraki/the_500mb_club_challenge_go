package load

import (
	"bytes"
	"context"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

func bytesReader(b []byte) io.Reader { return bytes.NewReader(b) }

// Segment is a constant arrival rate (requests/second) held for a duration.
type Segment struct {
	Rate int
	Dur  time.Duration
}

// Config parameterizes a load run.
type Config struct {
	BaseURL   string
	Mix       *Mix
	Segments  []Segment
	Devices   int          // device-id space: dev0..dev{N-1}
	BatchSize int          // points per batch op (default 50)
	Workers   int          // max in-flight requests (default 200)
	Warmup    int          // points pre-seeded per device before timing (default 16)
	Seed      int64        // RNG seed for reproducibility
	Client    *http.Client // optional; a keep-alive client is built if nil
}

// RunResult is the outcome of a run.
type RunResult struct {
	Stats   *Stats
	Dropped int64 // jobs the pacer could not hand to a worker (saturation)
	Elapsed time.Duration
}

// Run drives an open-loop, constant-arrival-rate load against the API. The pacer
// emits one job per 1/Rate seconds regardless of response time, so a slow server
// shows up as rising latency and (once Workers saturate) drops -- the capacity
// signal -- rather than as a self-throttled rate.
func Run(ctx context.Context, cfg Config) RunResult {
	cfg = withDefaults(cfg)
	stats := NewStats()
	client := cfg.Client
	if client == nil {
		client = defaultClient(cfg.Workers)
	}

	if cfg.Warmup > 0 {
		seed(ctx, cfg, client)
	}

	jobs := make(chan string, cfg.Workers)
	var dropped int64
	var wg sync.WaitGroup
	for i := 0; i < cfg.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rng := rand.New(rand.NewSource(cfg.Seed + 1))
			for op := range jobs {
				d, ok := fire(ctx, client, cfg, op, rng)
				stats.Record(op, d, ok)
			}
		}()
	}

	start := time.Now()
	pace(ctx, cfg, jobs, &dropped)
	close(jobs)
	wg.Wait()

	return RunResult{Stats: stats, Dropped: atomic.LoadInt64(&dropped), Elapsed: time.Since(start)}
}

// pace emits jobs at each segment's arrival rate, dropping (counting) when no
// worker is free rather than blocking, so the offered load stays constant.
func pace(ctx context.Context, cfg Config, jobs chan<- string, dropped *int64) {
	rng := rand.New(rand.NewSource(cfg.Seed))
	for _, seg := range cfg.Segments {
		if seg.Rate <= 0 {
			continue
		}
		interval := time.Second / time.Duration(seg.Rate)
		t := time.NewTicker(interval)
		deadline := time.After(seg.Dur)
		for {
			select {
			case <-ctx.Done():
				t.Stop()
				return
			case <-deadline:
				t.Stop()
				goto next
			case <-t.C:
				op := cfg.Mix.Pick(rng.Float64())
				select {
				case jobs <- op:
				default:
					atomic.AddInt64(dropped, 1)
				}
			}
		}
	next:
	}
}

// fire issues one request for op and returns its latency and whether it was a
// 2xx/3xx. The device id is drawn from the configured id space.
func fire(ctx context.Context, client *http.Client, cfg Config, op string, rng *rand.Rand) (time.Duration, bool) {
	dev := "dev" + strconv.Itoa(rng.Intn(cfg.Devices))
	method, url, body := request(cfg, op, dev, rng)
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return 0, false
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	start := time.Now()
	resp, err := client.Do(req)
	d := time.Since(start)
	if err != nil {
		return d, false
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	return d, resp.StatusCode < 400
}

func request(cfg Config, op, dev string, rng *rand.Rand) (method, url string, body io.Reader) {
	base := cfg.BaseURL + "/devices/" + dev
	switch op {
	case "batch":
		return "POST", base + "/telemetry/batch", bytesReader(RandomBatch(rng, cfg.BatchSize))
	case "range":
		return "GET", base + "/telemetry?from=0&to=" + strconv.FormatInt(time.Now().UnixMilli()+1, 10) + "&limit=100", nil
	case "anomaly":
		return "GET", base + "/anomaly", nil
	default: // post
		return "POST", base + "/telemetry", bytesReader(RandomPoint(rng))
	}
}

// seed pre-posts Warmup points to every device so range/anomaly read real data.
func seed(ctx context.Context, cfg Config, client *http.Client) {
	rng := rand.New(rand.NewSource(cfg.Seed - 1))
	for d := 0; d < cfg.Devices; d++ {
		dev := "dev" + strconv.Itoa(d)
		body := RandomBatch(rng, cfg.Warmup)
		req, err := http.NewRequestWithContext(ctx, "POST", cfg.BaseURL+"/devices/"+dev+"/telemetry/batch", bytesReader(body))
		if err != nil {
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		if resp, err := client.Do(req); err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
	}
}

func withDefaults(cfg Config) Config {
	if cfg.BatchSize == 0 {
		cfg.BatchSize = 50
	}
	if cfg.Workers == 0 {
		cfg.Workers = 200
	}
	if cfg.Devices == 0 {
		cfg.Devices = 50
	}
	if cfg.Warmup == 0 {
		// Pre-seed enough points that range/anomaly read real data from the first
		// request (anomaly needs >= 8 samples, else it returns 404 -- not a failure,
		// but it pollutes the error rate). -1 disables seeding explicitly.
		cfg.Warmup = 16
	}
	if cfg.Warmup < 0 {
		cfg.Warmup = 0
	}
	if cfg.Mix == nil {
		cfg.Mix = DefaultMix()
	}
	return cfg
}

func defaultClient(workers int) *http.Client {
	tr := &http.Transport{
		MaxIdleConns:        workers * 2,
		MaxIdleConnsPerHost: workers * 2,
		IdleConnTimeout:     90 * time.Second,
	}
	return &http.Client{Transport: tr, Timeout: 10 * time.Second}
}
