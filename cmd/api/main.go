// Command api is the Pi-Bench telemetry service: a single instance behind the
// round-robin load balancer.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
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

	reg := metrics.New()
	handler := httpapi.New(st, reg, httpapi.Config{
		InstanceID:     cfg.InstanceID,
		SingleMaxBytes: cfg.SingleMaxBytes,
		BatchMaxBytes:  cfg.BatchMaxBytes,
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

	log.Printf("instance %s listening on %s (redis %s)", cfg.InstanceID, cfg.Addr, cfg.RedisAddr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("listen: %v", err)
	}
	<-idleClosed
}
