// Package config loads runtime configuration from the environment with
// frugal, benchmark-appropriate defaults.
package config

import (
	"os"
	"strconv"
	"time"
)

// Config is the resolved application configuration.
type Config struct {
	Addr            string
	RedisAddr       string
	InstanceID      string
	PoolSize        int
	DeviceCap       int
	SingleMaxBytes  int64
	BatchMaxBytes   int64
	ShutdownTimeout time.Duration
}

// Load resolves configuration using getenv (injected for testability).
func Load(getenv func(string) string) Config {
	c := Config{
		Addr:            str(getenv, "LISTEN_ADDR", ":8080"),
		RedisAddr:       str(getenv, "REDIS_ADDR", "127.0.0.1:6379"),
		InstanceID:      getenv("INSTANCE_ID"),
		PoolSize:        intVal(getenv, "REDIS_POOL", 64),
		DeviceCap:       intVal(getenv, "DEVICE_CAP", 1024),
		SingleMaxBytes:  int64(intVal(getenv, "SINGLE_MAX_BYTES", 4096)),
		BatchMaxBytes:   int64(intVal(getenv, "BATCH_MAX_BYTES", 131072)),
		ShutdownTimeout: 10 * time.Second,
	}
	if c.InstanceID == "" {
		if h, err := os.Hostname(); err == nil && h != "" {
			c.InstanceID = h
		} else {
			c.InstanceID = "unknown"
		}
	}
	return c
}

func str(getenv func(string) string, key, def string) string {
	if v := getenv(key); v != "" {
		return v
	}
	return def
}

func intVal(getenv func(string) string, key string, def int) int {
	if v := getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
