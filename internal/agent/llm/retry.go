package llm

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

type RetryConfig struct {
	MaxRetries     int
	BaseDelay      time.Duration
	MaxDelay       time.Duration
	AttemptTimeout time.Duration
}

func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:     3,
		BaseDelay:      1 * time.Second,
		MaxDelay:       30 * time.Second,
		AttemptTimeout: 45 * time.Second,
	}
}

type RetryProvider struct {
	inner  Provider
	config RetryConfig
}

func WithRetry(provider Provider, config RetryConfig) *RetryProvider {
	if config.MaxRetries <= 0 {
		config.MaxRetries = 3
	}
	if config.BaseDelay <= 0 {
		config.BaseDelay = time.Second
	}
	if config.MaxDelay <= 0 {
		config.MaxDelay = 30 * time.Second
	}
	if config.AttemptTimeout < 0 {
		config.AttemptTimeout = 0
	}
	return &RetryProvider{inner: provider, config: config}
}

func (r *RetryProvider) Name() string { return r.inner.Name() }

func (r *RetryProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	var lastErr error
	for attempt := 0; attempt <= r.config.MaxRetries; attempt++ {
		attemptCtx, cancel := r.attemptContext(ctx)
		resp, err := r.inner.Chat(attemptCtx, req)
		cancel()
		if err == nil {
			return resp, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if !isRetryable(err) {
			return nil, err
		}
		lastErr = err
		if attempt < r.config.MaxRetries {
			delay := r.backoff(attempt)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}
	}
	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

func (r *RetryProvider) attemptContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if r.config.AttemptTimeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, r.config.AttemptTimeout)
}

func (r *RetryProvider) StreamChat(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error) {
	sp, ok := r.inner.(StreamProvider)
	if !ok {
		return nil, fmt.Errorf("inner provider does not support streaming")
	}

	var lastErr error
	for attempt := 0; attempt <= r.config.MaxRetries; attempt++ {
		ch, err := sp.StreamChat(ctx, req)
		if err == nil {
			return ch, nil
		}
		if !isRetryable(err) {
			return nil, err
		}
		lastErr = err
		if attempt < r.config.MaxRetries {
			delay := r.backoff(attempt)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}
	}
	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

func (r *RetryProvider) backoff(attempt int) time.Duration {
	delay := time.Duration(float64(r.config.BaseDelay) * math.Pow(2, float64(attempt)))
	if delay > r.config.MaxDelay {
		delay = r.config.MaxDelay
	}
	return delay
}

func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	msg := err.Error()
	for _, code := range []string{"429", "500", "502", "503", "529"} {
		if strings.Contains(msg, code) {
			return true
		}
	}
	if strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "context deadline exceeded") ||
		strings.Contains(msg, "EOF") {
		return true
	}
	return false
}
