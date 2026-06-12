// Steady-state load test -- matches the benchmark's "steady" scenario:
// constant arrival rate with the realistic mix (60% post, 10% batch, 20% range,
// 10% anomaly). Feeds efficiency (RSS+CPU), tail latency (p99 per op), the gate.
//
// Usage: go run ./stress/cmd/steady [-rate 200] [-dur 60s] [-url http://localhost:8080]
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/araki/pibench/stress/load"
)

func main() {
	fs := flag.NewFlagSet("steady", flag.ExitOnError)
	opts := load.RegisterCommon(fs)
	rate := fs.Int("rate", 200, "constant arrival rate (req/s)")
	dur := fs.Duration("dur", 60*time.Second, "hold duration")
	_ = fs.Parse(os.Args[1:])

	cfg := opts.Config([]load.Segment{{Rate: *rate, Dur: *dur}})
	fmt.Printf("steady: %d req/s for %s against %s (%d devices)\n\n", *rate, *dur, cfg.BaseURL, cfg.Devices)

	res := load.Run(context.Background(), cfg)
	res.Stats.Report(os.Stdout)
	fmt.Printf("\ndropped=%d elapsed=%s\n", res.Dropped, res.Elapsed.Round(time.Millisecond))
}
