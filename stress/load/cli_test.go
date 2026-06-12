package load

import (
	"testing"
	"time"
)

func TestCommonOptsConfigMapsFields(t *testing.T) {
	o := &CommonOpts{URL: "http://x:8080", Devices: 7, Workers: 33, Batch: 9}
	segs := []Segment{{Rate: 100, Dur: time.Second}}
	cfg := o.Config(segs)
	if cfg.BaseURL != "http://x:8080" || cfg.Devices != 7 || cfg.Workers != 33 || cfg.BatchSize != 9 {
		t.Errorf("config mapping wrong: %+v", cfg)
	}
	if cfg.Mix == nil || len(cfg.Segments) != 1 {
		t.Errorf("config missing mix/segments: %+v", cfg)
	}
}

func TestEnvOrFallsBack(t *testing.T) {
	t.Setenv("STRESS_TEST_KEY", "")
	if got := envOr("STRESS_TEST_KEY", "def"); got != "def" {
		t.Errorf("envOr empty: got %q want def", got)
	}
	t.Setenv("STRESS_TEST_KEY", "set")
	if got := envOr("STRESS_TEST_KEY", "def"); got != "set" {
		t.Errorf("envOr set: got %q want set", got)
	}
}
