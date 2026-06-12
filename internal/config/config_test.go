package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	c := Load(func(string) string { return "" })
	if c.Addr != ":8080" {
		t.Errorf("Addr default: got %q", c.Addr)
	}
	if c.RedisAddr != "127.0.0.1:6379" {
		t.Errorf("RedisAddr default: got %q", c.RedisAddr)
	}
	if c.DeviceCap != 1024 {
		t.Errorf("DeviceCap default: got %d", c.DeviceCap)
	}
	if c.ReadTimeout != 250*time.Millisecond {
		t.Errorf("ReadTimeout default: got %v want 250ms", c.ReadTimeout)
	}
	if c.InstanceID == "" {
		t.Error("InstanceID should fall back to a non-empty value")
	}
}

func TestLoadReadTimeoutOverride(t *testing.T) {
	c := Load(func(k string) string {
		if k == "READ_TIMEOUT_MS" {
			return "100"
		}
		return ""
	})
	if c.ReadTimeout != 100*time.Millisecond {
		t.Errorf("ReadTimeout override: got %v want 100ms", c.ReadTimeout)
	}
}

func TestLoadOverrides(t *testing.T) {
	env := map[string]string{
		"LISTEN_ADDR": ":9000",
		"REDIS_ADDR":  "redis:6379",
		"INSTANCE_ID": "api-2",
		"DEVICE_CAP":  "256",
		"REDIS_POOL":  "32",
	}
	c := Load(func(k string) string { return env[k] })
	if c.Addr != ":9000" || c.RedisAddr != "redis:6379" || c.InstanceID != "api-2" {
		t.Errorf("override mismatch: %+v", c)
	}
	if c.DeviceCap != 256 {
		t.Errorf("DeviceCap override: got %d want 256", c.DeviceCap)
	}
	if c.PoolSize != 32 {
		t.Errorf("PoolSize override: got %d want 32", c.PoolSize)
	}
}

func TestLoadIgnoresInvalidInts(t *testing.T) {
	c := Load(func(k string) string {
		if k == "DEVICE_CAP" {
			return "notanint"
		}
		return ""
	})
	if c.DeviceCap != 1024 {
		t.Errorf("invalid DEVICE_CAP should fall back to default, got %d", c.DeviceCap)
	}
}
