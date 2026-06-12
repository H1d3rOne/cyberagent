package daemon

import (
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/core/services"
)

type daemonRateLimitBucket struct {
	tokens float64
	last   time.Time
}

type daemonRateLimiter struct {
	mu                sync.Mutex
	requestsPerMinute int
	burst             int
	now               func() time.Time
	buckets           map[string]daemonRateLimitBucket
}

func newDaemonRateLimiter(cfg RateLimitConfig) *daemonRateLimiter {
	if !cfg.Enabled || cfg.RequestsPerMinute <= 0 || cfg.Burst <= 0 {
		return nil
	}
	return &daemonRateLimiter{
		requestsPerMinute: cfg.RequestsPerMinute,
		burst:             cfg.Burst,
		now:               time.Now,
		buckets:           map[string]daemonRateLimitBucket{},
	}
}

func (l *daemonRateLimiter) allow(key string) (bool, time.Duration) {
	if l == nil {
		return true, 0
	}
	key = strings.TrimSpace(key)
	if key == "" {
		key = "unknown"
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	if now.IsZero() {
		now = time.Now()
	}
	bucket, ok := l.buckets[key]
	if !ok || bucket.last.IsZero() {
		bucket = daemonRateLimitBucket{
			tokens: float64(l.burst),
			last:   now,
		}
	}
	elapsed := now.Sub(bucket.last)
	if elapsed > 0 {
		refill := elapsed.Minutes() * float64(l.requestsPerMinute)
		bucket.tokens = math.Min(float64(l.burst), bucket.tokens+refill)
		bucket.last = now
	}
	if bucket.tokens < 1 {
		l.buckets[key] = bucket
		seconds := math.Ceil((1 - bucket.tokens) / (float64(l.requestsPerMinute) / 60))
		if seconds < 1 {
			seconds = 1
		}
		return false, time.Duration(seconds) * time.Second
	}
	bucket.tokens--
	l.buckets[key] = bucket
	return true, 0
}

func withDaemonRateLimit(cfg RateLimitConfig, next http.Handler) http.Handler {
	limiter := newDaemonRateLimiter(cfg)
	if limiter == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		allowed, retryAfter := limiter.allow(daemonRateLimitKey(r))
		if !allowed {
			w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(retryAfter.Seconds()))))
			writeServiceError(w, services.NewError(services.CodeRateLimited, "rate limit exceeded", map[string]any{
				"requests_per_minute": cfg.RequestsPerMinute,
				"burst":               cfg.Burst,
			}, nil))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func daemonRateLimitKey(r *http.Request) string {
	host := daemonRemoteHost(r)
	if host != "" {
		return host
	}
	return "unknown"
}

func withDaemonHardening(cfg HardeningConfig, next http.Handler) http.Handler {
	if !cfg.RequireLoopback {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := daemonRemoteHost(r)
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			writeServiceError(w, services.NewError(services.CodePolicyDenied, "non-loopback API request rejected by deployment hardening", map[string]any{
				"require_loopback": true,
			}, nil))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func daemonRemoteHost(r *http.Request) string {
	if r == nil {
		return ""
	}
	remote := strings.TrimSpace(r.RemoteAddr)
	if remote == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(remote); err == nil {
		return strings.Trim(host, "[]")
	}
	return strings.Trim(remote, "[]")
}
