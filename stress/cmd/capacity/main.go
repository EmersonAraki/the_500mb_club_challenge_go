// Capacity sweep -- hold a series of fixed arrival rates and report, per rate,
// the achieved throughput, error rate, dropped iterations, and worst per-op p99.
// The knee (highest rate that stays failure-free with no drops) is the capacity
// number. Use it to rank CPU-split or memory variants between runs.
//
// Usage: go run ./stress/cmd/capacity [-rates 200,400,600,800,1000,1200] [-dur 30s] [-url ...]
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/araki/pibench/stress/load"
)

func main() {
	fs := flag.NewFlagSet("capacity", flag.ExitOnError)
	opts := load.RegisterCommon(fs)
	ratesArg := fs.String("rates", "200,400,600,800,1000,1200", "comma-separated req/s steps")
	dur := fs.Duration("dur", 30*time.Second, "hold per step")
	_ = fs.Parse(os.Args[1:])

	rates := parseRates(*ratesArg)
	fmt.Printf("capacity sweep against %s, %s per step: %v\n\n", opts.URL, *dur, rates)

	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "rate\tachieved/s\terror%\tdropped\tworst-p99")
	knee := 0
	for _, rate := range rates {
		cfg := opts.Config([]load.Segment{{Rate: rate, Dur: *dur}})
		res := load.Run(context.Background(), cfg)

		total, fails := 0, 0
		var worst time.Duration
		for _, op := range load.DefaultMix().Names() {
			r := res.Stats.Op(op)
			total += r.Count
			fails += r.Fails
			if r.P99 > worst {
				worst = r.P99
			}
		}
		errPct := 0.0
		if total > 0 {
			errPct = 100 * float64(fails) / float64(total)
		}
		achieved := float64(total) / res.Elapsed.Seconds()
		fmt.Fprintf(tw, "%d\t%.0f\t%.2f\t%d\t%s\n", rate, achieved, errPct, res.Dropped, worst.Round(time.Microsecond))

		if errPct < 0.5 && res.Dropped == 0 {
			knee = rate
		}
	}
	tw.Flush()
	fmt.Printf("\nknee = %d req/s (highest failure-free, drop-free step)\n", knee)
}

func parseRates(s string) []int {
	var out []int
	for _, part := range strings.Split(s, ",") {
		if n, err := strconv.Atoi(strings.TrimSpace(part)); err == nil && n > 0 {
			out = append(out, n)
		}
	}
	return out
}
