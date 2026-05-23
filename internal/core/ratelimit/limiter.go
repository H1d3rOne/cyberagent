package ratelimit

import (
	"context"
	"sync"
	"time"
)

type Limiter struct {
	mu             sync.Mutex
	perHost        map[string]*hostState
	globalRPS      float64
	globalBurst    int
	perHostRPS     float64
	perHostBurst   int
	maxConcurrency int
	activeSem      chan struct{}
	lastGlobalTick time.Time
	globalTokens   float64
}

type hostState struct {
	lastTick time.Time
	tokens   float64
}

type Config struct {
	GlobalRPS         float64
	GlobalBurst       int
	PerHostRPS        float64
	PerHostBurst      int
	MaxConcurrencyAll int
}

func DefaultConfig() Config {
	return Config{
		GlobalRPS:         20,
		GlobalBurst:       5,
		PerHostRPS:        10,
		PerHostBurst:      3,
		MaxConcurrencyAll: 10,
	}
}

func New(cfg Config) *Limiter {
	if cfg.GlobalRPS <= 0 {
		cfg.GlobalRPS = 20
	}
	if cfg.GlobalBurst <= 0 {
		cfg.GlobalBurst = 5
	}
	if cfg.PerHostRPS <= 0 {
		cfg.PerHostRPS = 10
	}
	if cfg.PerHostBurst <= 0 {
		cfg.PerHostBurst = 3
	}
	if cfg.MaxConcurrencyAll <= 0 {
		cfg.MaxConcurrencyAll = 10
	}
	return &Limiter{
		perHost:        make(map[string]*hostState),
		globalRPS:      cfg.GlobalRPS,
		globalBurst:    cfg.GlobalBurst,
		perHostRPS:     cfg.PerHostRPS,
		perHostBurst:   cfg.PerHostBurst,
		maxConcurrency: cfg.MaxConcurrencyAll,
		activeSem:      make(chan struct{}, cfg.MaxConcurrencyAll),
		lastGlobalTick: time.Now(),
		globalTokens:   float64(cfg.GlobalBurst),
	}
}

func (l *Limiter) Wait(ctx context.Context, host string) error {
	select {
	case l.activeSem <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}

	for {
		wait := l.reserve(host)
		if wait <= 0 {
			return nil
		}
		timer := time.NewTimer(wait)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			l.Release()
			return ctx.Err()
		}
	}
}

func (l *Limiter) Release() {
	<-l.activeSem
}

func (l *Limiter) reserve(host string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()

	elapsed := now.Sub(l.lastGlobalTick).Seconds()
	l.globalTokens += elapsed * l.globalRPS
	if l.globalTokens > float64(l.globalBurst) {
		l.globalTokens = float64(l.globalBurst)
	}
	l.lastGlobalTick = now

	hs := l.perHost[host]
	if hs == nil {
		hs = &hostState{lastTick: now, tokens: float64(l.perHostBurst)}
		l.perHost[host] = hs
	}
	hostElapsed := now.Sub(hs.lastTick).Seconds()
	hs.tokens += hostElapsed * l.perHostRPS
	if hs.tokens > float64(l.perHostBurst) {
		hs.tokens = float64(l.perHostBurst)
	}
	hs.lastTick = now

	if l.globalTokens >= 1 && hs.tokens >= 1 {
		l.globalTokens--
		hs.tokens--
		return 0
	}

	var globalWait, hostWait time.Duration
	if l.globalTokens < 1 {
		globalWait = time.Duration((1 - l.globalTokens) / l.globalRPS * float64(time.Second))
	}
	if hs.tokens < 1 {
		hostWait = time.Duration((1 - hs.tokens) / l.perHostRPS * float64(time.Second))
	}
	if hostWait > globalWait {
		return hostWait
	}
	return globalWait
}

func (l *Limiter) ActiveCount() int {
	return len(l.activeSem)
}
