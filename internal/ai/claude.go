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
	claudeDefaultBaseURL = "https://api.anthropic.com"
	claudeDefaultModel   = "claude-sonnet-4-6"
	claudeAPIVersion     = "2023-06-01"
	claudeSystemPrompt   = "You are a helpful assistant embedded in a keyboard-driven command palette. " +
		"Give concise, direct answers. Avoid markdown headers, lengthy preamble, and filler phrases."
)

// ClaudeConfig holds configuration for the Anthropic Claude provider.
type ClaudeConfig struct {
	APIKey  string
	Model   string
	BaseURL string // override for testing; empty uses the production URL
}

// ClaudeProvider implements AIProvider using the Anthropic Messages API.
type ClaudeProvider struct {
	cfg    ClaudeConfig
	client *http.Client
}

// NewClaudeProvider creates a ClaudeProvider from the given config.
func NewClaudeProvider(cfg ClaudeConfig) *ClaudeProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = claudeDefaultBaseURL
	}
	if cfg.Model == "" {
		cfg.Model = claudeDefaultModel
	}
	return &ClaudeProvider{cfg: cfg, client: &http.Client{}}
}

func (p *ClaudeProvider) Name() string { return "claude" }

// Query sends input to the Claude Messages API and streams Token chunks.
func (p *ClaudeProvider) Query(ctx context.Context, input string, history []Message) (<-chan Token, error) {
	apiKey := p.cfg.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("Claude API key not configured. Set ANTHROPIC_API_KEY or ai.claude.api_key in config.")
	}

	// Build messages array from history + current input.
	type msgPayload struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	msgs := make([]msgPayload, 0, len(history)+1)
	for _, h := range history {
		msgs = append(msgs, msgPayload{Role: h.Role, Content: h.Content})
	}
	msgs = append(msgs, msgPayload{Role: "user", Content: input})

	body, err := json.Marshal(map[string]any{
		"model":      p.cfg.Model,
		"max_tokens": 1024,
		"stream":     true,
		"system":     claudeSystemPrompt,
		"messages":   msgs,
	})
	if err != nil {
		return nil, fmt.Errorf("claude: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.cfg.BaseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("claude: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", claudeAPIVersion)

	ch := make(chan Token, 16)
	go func() {
		defer close(ch)
		p.stream(ctx, req, ch)
	}()
	return ch, nil
}

func (p *ClaudeProvider) stream(ctx context.Context, req *http.Request, ch chan<- Token) {
	resp, err := p.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return // cancelled — swallow error
		}
		ch <- Token{Error: fmt.Errorf("claude: request failed: %w", err)}
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		ch <- Token{Error: fmt.Errorf("claude: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))}
		return
	}

	scanner := bufio.NewScanner(resp.Body)
	var eventType, dataLine string

	for scanner.Scan() {
		if ctx.Err() != nil {
			return
		}
		line := scanner.Text()

		switch {
		case strings.HasPrefix(line, "event: "):
			eventType = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			dataLine = strings.TrimPrefix(line, "data: ")
		case line == "":
			// End of event — process it.
			if eventType == "content_block_delta" && dataLine != "" {
				tok := extractClaudeToken(dataLine)
				if tok != "" {
					ch <- Token{Text: tok}
				}
			} else if eventType == "message_stop" {
				ch <- Token{Done: true}
				return
			}
			eventType = ""
			dataLine = ""
		}
	}

	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		ch <- Token{Error: fmt.Errorf("claude: read stream: %w", err)}
		return
	}
	ch <- Token{Done: true}
}

func extractClaudeToken(data string) string {
	var event struct {
		Delta struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"delta"`
	}
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		return ""
	}
	if event.Delta.Type == "text_delta" {
		return event.Delta.Text
	}
	return ""
}
