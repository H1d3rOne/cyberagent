package llm

import (
	"context"
	"net/http"
)

type OllamaProvider struct {
	openai *OpenAIProvider
}

func NewOllama(config ProviderConfig) *OllamaProvider {
	if config.BaseURL == "" {
		config.BaseURL = "http://localhost:11434/v1/chat/completions"
	}
	if config.Model == "" {
		config.Model = "llama3.1"
	}
	return &OllamaProvider{openai: &OpenAIProvider{config: config, client: &http.Client{}}}
}

func (p *OllamaProvider) Name() string { return "ollama" }

func (p *OllamaProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	return p.openai.Chat(ctx, req)
}

func (p *OllamaProvider) StreamChat(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error) {
	return p.openai.StreamChat(ctx, req)
}
