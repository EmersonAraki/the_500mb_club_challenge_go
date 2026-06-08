package store

import (
	"context"
	"net"
	"testing"
	"time"
)

// fakeRedisPong starts a TCP server that replies "+PONG\r\n" to every command it
// reads. It lets Warm's PING verification succeed without a real Redis, keeping
// the test dependency-free and in line with the project's zero-broker unit tests.
func fakeRedisPong(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 256)
				for {
					if _, err := c.Read(buf); err != nil {
						return
					}
					if _, err := c.Write([]byte("+PONG\r\n")); err != nil {
						return
					}
				}
			}(c)
		}
	}()
	return ln.Addr().String()
}

func TestRedisWarmFillsPool(t *testing.T) {
	s := NewRedis(fakeRedisPong(t), 4, 1024).(*redisStore)

	if err := s.Warm(context.Background()); err != nil {
		t.Fatalf("warm: %v", err)
	}
	if got := len(s.pool); got != 4 {
		t.Fatalf("pooled conns after warm: got %d want 4", got)
	}

	// A subsequent op must reuse a pooled connection and return it, leaving the
	// pool full -- proving the warmed connections are live and usable.
	if err := s.Ping(context.Background()); err != nil {
		t.Fatalf("ping after warm: %v", err)
	}
	if got := len(s.pool); got != 4 {
		t.Errorf("pool after ping: got %d want 4 (conn should be returned)", got)
	}
}

func TestRedisWarmErrorsWhenUnreachable(t *testing.T) {
	// Reserve then immediately release a port so the dial is actively refused.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	s := NewRedis(addr, 4, 1024).(*redisStore)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := s.Warm(ctx); err == nil {
		t.Fatal("warm: expected error when redis unreachable, got nil")
	}
	if got := len(s.pool); got != 0 {
		t.Errorf("pool after failed warm: got %d want 0", got)
	}
}
