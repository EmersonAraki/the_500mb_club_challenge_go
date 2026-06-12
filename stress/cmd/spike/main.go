// Spike load test -- a baseline rate, a burst to a higher rate, then recovery
// back to baseline. The load balancer is busiest during the burst, so this is
// where a starved nginx (the E2 concern) or a GC pause shows up as a tail spike.
//
// Usage: go run ./stress/cmd/spike [-base 200] [-spike 400] [-seg 20s] [-url ...]
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
	fs := flag.NewFlagSet("spike", flag.ExitOnError)
	opts := load.RegisterCommon(fs)
	base := fs.Int("base", 200, "baseline arrival rate (req/s)")
	spike := fs.Int("spike", 400, "burst arrival rate (req/s)")
	seg := fs.Duration("seg", 20*time.Second, "duration of each phase (base/spike/base)")
	_ = fs.Parse(os.Args[1:])

	cfg := opts.Config([]load.Segment{
		{Rate: *base, Dur: *seg},
		{Rate: *spike, Dur: *seg},
		{Rate: *base, Dur: *seg},
	})
	fmt.Printf("spike: %d -> %d -> %d req/s, %s each, against %s\n\n", *base, *spike, *base, *seg, cfg.BaseURL)

	res := load.Run(context.Background(), cfg)
	res.Stats.Report(os.Stdout)
	fmt.Printf("\ndropped=%d elapsed=%s\n", res.Dropped, res.Elapsed.Round(time.Millisecond))
}
