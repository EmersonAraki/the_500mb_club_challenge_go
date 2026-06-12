// Endurance load test -- constant arrival rate held for a long duration to
// expose memory drift, GC sawtooth, and OOM kills under sustained ingestion.
// This is the scenario that validates the E1 memory-tightening ladder: watch
// the error rate and (alongside `docker stats`) RSS over the full hold.
//
// Usage: go run ./stress/cmd/endurance [-rate 200] [-dur 30m] [-url ...]
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
	fs := flag.NewFlagSet("endurance", flag.ExitOnError)
	opts := load.RegisterCommon(fs)
	rate := fs.Int("rate", 200, "constant arrival rate (req/s)")
	dur := fs.Duration("dur", 30*time.Minute, "hold duration")
	_ = fs.Parse(os.Args[1:])

	cfg := opts.Config([]load.Segment{{Rate: *rate, Dur: *dur}})
	fmt.Printf("endurance: %d req/s for %s against %s (%d devices)\n\n", *rate, *dur, cfg.BaseURL, cfg.Devices)

	res := load.Run(context.Background(), cfg)
	res.Stats.Report(os.Stdout)
	fmt.Printf("\ndropped=%d elapsed=%s\n", res.Dropped, res.Elapsed.Round(time.Millisecond))
}
