// Command api is the Pi-Bench telemetry service: a single instance behind the
// round-robin load balancer.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/araki/pibench/internal/config"
	"github.com/araki/pibench/internal/httpapi"
	"github.com/araki/pibench/internal/metrics"
	"github.com/araki/pibench/internal/store"
)

func main() {
	cfg := config.Load(os.Getenv)

	st := store.NewRedis(cfg.RedisAddr, cfg.PoolSize, cfg.DeviceCap)
	defer st.Close()

	// Pre-warm the connection pool in the background so request-path latency
	// never includes a TCP handshake. Best-effort and non-blocking: Redis may not
	// be ready yet (depends_on does not wait for readiness), so retry briefly and
	// fall back to lazy dialing if it never comes up.
	if w, ok := st.(store.Warmer); ok {
		go warmPool(w, cfg.PoolSize)
	}

	reg := metrics.New()
	// Expose Go runtime GC/scheduler gauges so /metrics can attribute tail
	// latency to stop-the-world pauses vs. CFS scheduling starvation.
	reg.AddCollector(metrics.RuntimeCollector)
	handler := httpapi.New(st, reg, httpapi.Config{
		InstanceID:     cfg.InstanceID,
		SingleMaxBytes: cfg.SingleMaxBytes,
		BatchMaxBytes:  cfg.BatchMaxBytes,
		ReadTimeout:    cfg.ReadTimeout,
	})

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	idleClosed := make(chan struct{})
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
		<-sig

		// Drain in-flight requests within the contract's 10s window.
		ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("graceful shutdown error: %v", err)
		}
		close(idleClosed)
	}()

	log.Printf("instance %s listening on %s (redis %s, GOMAXPROCS=%d)",
		cfg.InstanceID, cfg.Addr, cfg.RedisAddr, runtime.GOMAXPROCS(0))
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("listen: %v", err)
	}
	<-idleClosed
}

// warmPool fills the Redis connection pool, retrying while Redis comes up. It
// gives up after a bounded window rather than blocking forever; the request path
// dials lazily as a fallback, so a failed warm-up degrades latency, not function.
func warmPool(w store.Warmer, poolSize int) {
	for attempt := 1; attempt <= 20; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		err := w.Warm(ctx)
		cancel()
		if err == nil {
			log.Printf("redis pool warmed (%d connections)", poolSize)
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	log.Printf("redis pool warm-up gave up; connections will be dialed lazily")
}
