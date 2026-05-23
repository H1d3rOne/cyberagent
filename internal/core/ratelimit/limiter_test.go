package ratelimit

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestLimiterBasicWait(t *testing.T) {
	l := New(Config{GlobalRPS: 100, GlobalBurst: 10, PerHostRPS: 50, PerHostBurst: 5, MaxConcurrencyAll: 10})
	ctx := context.Background()

	err := l.Wait(ctx, "localhost")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	l.Release()
}

func TestLimiterConcurrencyLimit(t *testing.T) {
	l := New(Config{GlobalRPS: 1000, GlobalBurst: 100, PerHostRPS: 1000, PerHostBurst: 100, MaxConcurrencyAll: 2})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_ = l.Wait(ctx, "a")
	_ = l.Wait(ctx, "b")

	if l.ActiveCount() != 2 {
		t.Errorf("expected 2 active, got %d", l.ActiveCount())
	}

	done := make(chan struct{})
	go func() {
		_ = l.Wait(ctx, "c")
		close(done)
	}()

	select {
	case <-done:
		t.Error("third request should block")
	case <-time.After(50 * time.Millisecond):
	}

	l.Release()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Error("third request should proceed after release")
	}
	l.Release()
	l.Release()
}

func TestLimiterContextCancel(t *testing.T) {
	l := New(Config{GlobalRPS: 1000, GlobalBurst: 100, PerHostRPS: 1000, PerHostBurst: 100, MaxConcurrencyAll: 1})
	ctx := context.Background()
	_ = l.Wait(ctx, "host")

	ctx2, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := l.Wait(ctx2, "host")
	if err == nil {
		t.Error("expected context deadline error")
		l.Release()
	}
	l.Release()
}

func TestLimiterPerHostThrottle(t *testing.T) {
	l := New(Config{GlobalRPS: 1000, GlobalBurst: 100, PerHostRPS: 2, PerHostBurst: 2, MaxConcurrencyAll: 100})
	ctx := context.Background()

	start := time.Now()
	for i := 0; i < 4; i++ {
		if err := l.Wait(ctx, "slow-host"); err != nil {
			t.Fatal(err)
		}
		l.Release()
	}
	elapsed := time.Since(start)

	if elapsed < 400*time.Millisecond {
		t.Errorf("expected throttling to slow requests, elapsed: %v", elapsed)
	}
}

func TestLimiterConcurrentSafety(t *testing.T) {
	l := New(DefaultConfig())
	ctx := context.Background()
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := l.Wait(ctx, "host"); err != nil {
				return
			}
			time.Sleep(time.Millisecond)
			l.Release()
		}()
	}
	wg.Wait()
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.GlobalRPS <= 0 || cfg.PerHostRPS <= 0 || cfg.MaxConcurrencyAll <= 0 {
		t.Error("default config should have positive values")
	}
}
