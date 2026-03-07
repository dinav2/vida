package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

const (
	openaiDefaultBaseURL = "https://api.openai.com"
	openaiDefaultModel   = "gpt-4o"
	openaiSystemPrompt   = "You are a helpful assistant embedded in a keyboard-driven command palette. " +
		"Give concise, direct answers. Avoid markdown headers, lengthy preamble, and filler phrases."
)

// OpenAIConfig holds configuration for the OpenAI provider.
type OpenAIConfig struct {
	APIKey  string
	Model   string
	BaseURL string // override for testing; empty uses the production URL
}

// OpenAIProvider implements AIProvider using the OpenAI Chat Completions API.
type OpenAIProvider struct {
	cfg    OpenAIConfig
	client *http.Client
}

// NewOpenAIProvider creates an OpenAIProvider from the given config.
func NewOpenAIProvider(cfg OpenAIConfig) *OpenAIProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = openaiDefaultBaseURL
	}
	if cfg.Model == "" {
		cfg.Model = openaiDefaultModel
	}
	return &OpenAIProvider{cfg: cfg, client: &http.Client{}}
}

func (p *OpenAIProvider) Name() string { return "openai" }

// Query sends input to the OpenAI Chat Completions API and streams Token chunks.
func (p *OpenAIProvider) Query(ctx context.Context, input string, history []Message) (<-chan Token, error) {
	apiKey := p.cfg.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("OpenAI API key not configured. Set OPENAI_API_KEY or ai.openai.api_key in config.")
	}

	type msgPayload struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	msgs := []msgPayload{{Role: "system", Content: openaiSystemPrompt}}
	for _, h := range history {
		msgs = append(msgs, msgPayload{Role: h.Role, Content: h.Content})
	}
	msgs = append(msgs, msgPayload{Role: "user", Content: input})

	body, err := json.Marshal(map[string]any{
		"model":    p.cfg.Model,
		"stream":   true,
		"messages": msgs,
	})
	if err != nil {
		return nil, fmt.Errorf("openai: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.cfg.BaseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openai: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	ch := make(chan Token, 16)
	go func() {
		defer close(ch)
		p.stream(ctx, req, ch)
	}()
	return ch, nil
}

func (p *OpenAIProvider) stream(ctx context.Context, req *http.Request, ch chan<- Token) {
	resp, err := p.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		ch <- Token{Error: fmt.Errorf("openai: request failed: %w", err)}
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		ch <- Token{Error: fmt.Errorf("openai: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))}
		return
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		if ctx.Err() != nil {
			return
		}
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			ch <- Token{Done: true}
			return
		}
		if tok := extractOpenAIToken(data); tok != "" {
			ch <- Token{Text: tok}
		}
	}

	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		ch <- Token{Error: fmt.Errorf("openai: read stream: %w", err)}
		return
	}
	ch <- Token{Done: true}
}

func extractOpenAIToken(data string) string {
	var chunk struct {
		Choices []struct {
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(data), &chunk); err != nil || len(chunk.Choices) == 0 {
		return ""
	}
	return chunk.Choices[0].Delta.Content
}
