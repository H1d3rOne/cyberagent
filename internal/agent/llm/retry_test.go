package llm

import (
	"context"
	"fmt"
	"testing"
	"time"
)

type failingProvider struct {
	failures  int
	callCount int
}

func (f *failingProvider) Name() string { return "failing" }

func (f *failingProvider) Chat(_ context.Context, _ ChatRequest) (*ChatResponse, error) {
	f.callCount++
	if f.callCount <= f.failures {
		return nil, fmt.Errorf("anthropic API error 429: rate limited")
	}
	return &ChatResponse{
		Content:    []ContentBlock{TextBlock{Text: "ok"}},
		StopReason: StopReasonEndTurn,
	}, nil
}

func TestRetrySucceedsAfterFailures(t *testing.T) {
	inner := &failingProvider{failures: 2}
	provider := WithRetry(inner, RetryConfig{MaxRetries: 3, BaseDelay: 10 * time.Millisecond, MaxDelay: 50 * time.Millisecond})

	resp, err := provider.Chat(context.Background(), ChatRequest{})
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if inner.callCount != 3 {
		t.Errorf("expected 3 calls, got %d", inner.callCount)
	}
	if len(resp.Content) != 1 {
		t.Fatal("expected 1 content block")
	}
}

func TestRetryExhausted(t *testing.T) {
	inner := &failingProvider{failures: 10}
	provider := WithRetry(inner, RetryConfig{MaxRetries: 2, BaseDelay: 10 * time.Millisecond, MaxDelay: 50 * time.Millisecond})

	_, err := provider.Chat(context.Background(), ChatRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
	if inner.callCount != 3 {
		t.Errorf("expected 3 calls (1 + 2 retries), got %d", inner.callCount)
	}
}

func TestRetryNonRetryableError(t *testing.T) {
	inner := &struct {
		Provider
		callCount int
	}{Provider: &failingProvider{}}
	nonRetryable := &nonRetryableProvider{}
	provider := WithRetry(nonRetryable, RetryConfig{MaxRetries: 3, BaseDelay: 10 * time.Millisecond, MaxDelay: 50 * time.Millisecond})

	_, err := provider.Chat(context.Background(), ChatRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
	if nonRetryable.callCount != 1 {
		t.Errorf("expected 1 call (no retry for non-retryable), got %d", nonRetryable.callCount)
	}
	_ = inner
}

type nonRetryableProvider struct {
	callCount int
}

func (n *nonRetryableProvider) Name() string { return "nonretryable" }
func (n *nonRetryableProvider) Chat(_ context.Context, _ ChatRequest) (*ChatResponse, error) {
	n.callCount++
	return nil, fmt.Errorf("anthropic API error 401: unauthorized")
}

func TestRetryContextCancel(t *testing.T) {
	inner := &failingProvider{failures: 10}
	provider := WithRetry(inner, RetryConfig{MaxRetries: 5, BaseDelay: 500 * time.Millisecond, MaxDelay: 2 * time.Second})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := provider.Chat(ctx, ChatRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
}

type timeoutThenSuccessProvider struct {
	callCount int
}

func (t *timeoutThenSuccessProvider) Name() string { return "timeout-then-success" }

func (t *timeoutThenSuccessProvider) Chat(ctx context.Context, _ ChatRequest) (*ChatResponse, error) {
	t.callCount++
	if t.callCount == 1 {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return &ChatResponse{
		Content:    []ContentBlock{TextBlock{Text: "ok"}},
		StopReason: StopReasonEndTurn,
	}, nil
}

func TestRetryAttemptTimeoutRetries(t *testing.T) {
	inner := &timeoutThenSuccessProvider{}
	provider := WithRetry(inner, RetryConfig{
		MaxRetries:     1,
		BaseDelay:      time.Millisecond,
		MaxDelay:       time.Millisecond,
		AttemptTimeout: 10 * time.Millisecond,
	})

	resp, err := provider.Chat(context.Background(), ChatRequest{})
	if err != nil {
		t.Fatalf("expected retry after attempt timeout to succeed, got: %v", err)
	}
	if inner.callCount != 2 {
		t.Fatalf("callCount = %d, want 2", inner.callCount)
	}
	if len(resp.Content) != 1 {
		t.Fatal("expected response content")
	}
}
