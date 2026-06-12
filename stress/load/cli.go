package load

import (
	"flag"
	"os"
)

// CommonOpts are the flags every scenario shares.
type CommonOpts struct {
	URL     string
	Devices int
	Workers int
	Batch   int
}

// RegisterCommon registers the shared flags on fs and returns the bound opts.
// BASE_URL from the environment overrides the -url default (matches the k6 harness).
func RegisterCommon(fs *flag.FlagSet) *CommonOpts {
	o := &CommonOpts{}
	fs.StringVar(&o.URL, "url", envOr("BASE_URL", "http://localhost:8080"), "target base URL")
	fs.IntVar(&o.Devices, "devices", 50, "device-id space (dev0..devN-1)")
	fs.IntVar(&o.Workers, "workers", 200, "max in-flight requests")
	fs.IntVar(&o.Batch, "batch", 50, "points per batch op")
	return o
}

// Config builds a run Config for the given arrival schedule with the default mix.
func (o *CommonOpts) Config(segments []Segment) Config {
	return Config{
		BaseURL:   o.URL,
		Mix:       DefaultMix(),
		Segments:  segments,
		Devices:   o.Devices,
		Workers:   o.Workers,
		BatchSize: o.Batch,
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
