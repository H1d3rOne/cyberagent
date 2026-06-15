package daemon

import (
	"context"
	"errors"
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/agent/llm"
	"github.com/H1d3rOne/cyberagent/internal/interfaces/web"
)

type agentLLMFallbackTestProvider struct {
	name  string
	err   error
	resp  *llm.ChatResponse
	calls int
}

func (p *agentLLMFallbackTestProvider) Name() string {
	return p.name
}

func (p *agentLLMFallbackTestProvider) Chat(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
	p.calls++
	if p.err != nil {
		return nil, p.err
	}
	if p.resp != nil {
		return p.resp, nil
	}
	return &llm.ChatResponse{Content: []llm.ContentBlock{llm.TextBlock{Text: "ok"}}}, nil
}

type agentLLMFallbackTestProviderStore struct {
	cfg web.ProviderConfig
	err error
}

func (s *agentLLMFallbackTestProviderStore) Load() (web.ProviderConfig, error) {
	if s.err != nil {
		return web.ProviderConfig{}, s.err
	}
	return s.cfg, nil
}

func (s *agentLLMFallbackTestProviderStore) Save(cfg web.ProviderConfig) error {
	s.cfg = cfg
	return nil
}

func TestAgentLLMProviderAttemptsPreferStoredSecretOverRedactedRequest(t *testing.T) {
	mgr := newAgentManager(t.TempDir())
	mgr.AttachProviderStore(&agentLLMFallbackTestProviderStore{cfg: web.ProviderConfig{
		ActiveID: "server-profile",
		Providers: []web.ProviderProfile{{
			ID:      "server-profile",
			Name:    "Server profile",
			Type:    "openai",
			Model:   "server-model",
			APIKey:  "server-secret",
			BaseURL: "https://example.test/v1",
		}},
	}})

	attempts := mgr.agentLLMProviderAttempts(sendAgentMessageRequest{
		Provider: "openai",
		Model:    "server-model",
		BaseURL:  "https://example.test/v1",
		// APIKey is intentionally empty because browser-facing profiles redact it.
	})
	if len(attempts) != 1 {
		t.Fatalf("attempt count = %d, want 1", len(attempts))
	}
	if attempts[0].ProfileID != "server-profile" {
		t.Fatalf("first attempt profile = %q, want server-profile", attempts[0].ProfileID)
	}
	if attempts[0].Request.APIKey != "server-secret" {
		t.Fatalf("first attempt did not use server-side secret")
	}
}

func TestChatAgentLLMWithFallbackFallsThroughAuthErrors(t *testing.T) {
	mgr := newAgentManager(t.TempDir())
	bad := &agentLLMFallbackTestProvider{name: "bad", err: errors.New("openai stream error 401")}
	good := &agentLLMFallbackTestProvider{name: "good"}

	resp, streamed, used, fallbackLog, err := mgr.chatAgentLLMWithFallback(
		context.Background(),
		"",
		agentRuntimeTurnRef{},
		"plan",
		[]agentLLMProviderAttempt{
			{Provider: bad, Request: sendAgentMessageRequest{Provider: "openai", Model: "bad-model"}, ProfileID: "bad-profile"},
			{Provider: good, Request: sendAgentMessageRequest{Provider: "openai", Model: "good-model"}, ProfileID: "good-profile"},
		},
		func(agentLLMProviderAttempt) llm.ChatRequest { return llm.ChatRequest{} },
	)
	if err != nil {
		t.Fatalf("fallback returned error: %v", err)
	}
	if streamed {
		t.Fatalf("streamed = true, want false")
	}
	if resp == nil {
		t.Fatalf("response is nil")
	}
	if used.ProfileID != "good-profile" {
		t.Fatalf("used profile = %q, want good-profile", used.ProfileID)
	}
	if bad.calls != 1 || good.calls != 1 {
		t.Fatalf("provider calls bad=%d good=%d, want 1/1", bad.calls, good.calls)
	}
	if len(fallbackLog) != 1 || fallbackLog[0].ProfileID != "bad-profile" {
		t.Fatalf("fallback log = %#v, want one bad-profile record", fallbackLog)
	}
}

func TestChatAgentLLMWithFallbackStopsOnNonFailoverError(t *testing.T) {
	mgr := newAgentManager(t.TempDir())
	bad := &agentLLMFallbackTestProvider{name: "bad", err: errors.New("invalid request payload")}
	good := &agentLLMFallbackTestProvider{name: "good"}

	_, _, used, fallbackLog, err := mgr.chatAgentLLMWithFallback(
		context.Background(),
		"",
		agentRuntimeTurnRef{},
		"plan",
		[]agentLLMProviderAttempt{
			{Provider: bad, Request: sendAgentMessageRequest{Provider: "openai", Model: "bad-model"}, ProfileID: "bad-profile"},
			{Provider: good, Request: sendAgentMessageRequest{Provider: "openai", Model: "good-model"}, ProfileID: "good-profile"},
		},
		func(agentLLMProviderAttempt) llm.ChatRequest { return llm.ChatRequest{} },
	)
	if err == nil {
		t.Fatalf("error is nil, want non-failover error")
	}
	if used.ProfileID != "bad-profile" {
		t.Fatalf("used profile = %q, want bad-profile", used.ProfileID)
	}
	if bad.calls != 1 || good.calls != 0 {
		t.Fatalf("provider calls bad=%d good=%d, want 1/0", bad.calls, good.calls)
	}
	if len(fallbackLog) != 1 || fallbackLog[0].ProfileID != "bad-profile" {
		t.Fatalf("fallback log = %#v, want one bad-profile record", fallbackLog)
	}
}
