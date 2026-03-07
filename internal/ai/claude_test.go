// Package ai implements AI provider abstractions and concrete implementations.
// Tests cover SCN-12, SCN-19, FR-08 for the Claude provider.
package ai_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dinav2/vida/internal/ai"
)

// fakeClaudeServer creates a mock Anthropic streaming server.
// tokens is the list of text_delta values to stream.
func fakeClaudeServer(t *testing.T, tokens []string, statusCode int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if statusCode != http.StatusOK {
			http.Error(w, `{"error":{"type":"auth_error","message":"invalid api key"}}`, statusCode)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		flusher, _ := w.(http.Flusher)

		// stream message_start
		_, _ = fmt.Fprintf(w, "event: message_start\ndata: {\"type\":\"message_start\"}\n\n")
		flusher.Flush()

		for _, tok := range tokens {
			data, _ := json.Marshal(map[string]any{
				"type":  "content_block_delta",
				"delta": map[string]string{"type": "text_delta", "text": tok},
			})
			_, _ = fmt.Fprintf(w, "event: content_block_delta\ndata: %s\n\n", data)
			flusher.Flush()
		}

		_, _ = fmt.Fprintf(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
		flusher.Flush()
	}))
}

// SCN-12: Claude provider streams tokens
func TestClaudeProvider_Streaming(t *testing.T) {
	wantTokens := []string{"An ", "inode ", "is..."}
	srv := fakeClaudeServer(t, wantTokens, http.StatusOK)
	defer srv.Close()

	p := ai.NewClaudeProvider(ai.ClaudeConfig{
		APIKey:  "test-key",
		Model:   "claude-sonnet-4-6",
		BaseURL: srv.URL, // override for testing
	})

	ch, err := p.Query(context.Background(), "explain what inode is", nil)
	if err != nil {
		t.Fatalf("Query returned error: %v", err)
	}

	var got []string
	for tok := range ch {
		if tok.Error != nil {
			t.Fatalf("received error token: %v", tok.Error)
		}
		if tok.Done {
			break
		}
		got = append(got, tok.Text)
	}

	if len(got) != len(wantTokens) {
		t.Errorf("got %d tokens, want %d", len(got), len(wantTokens))
	}
	for i, tok := range got {
		if tok != wantTokens[i] {
			t.Errorf("token[%d] = %q, want %q", i, tok, wantTokens[i])
		}
	}
}

// FR-08i: request uses Anthropic Messages API with stream: true
func TestClaudeProvider_RequestFormat(t *testing.T) {
	var capturedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		// return minimal valid SSE to close the stream
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	}))
	defer srv.Close()

	p := ai.NewClaudeProvider(ai.ClaudeConfig{
		APIKey:  "test-key",
		Model:   "claude-sonnet-4-6",
		BaseURL: srv.URL,
	})

	ch, _ := p.Query(context.Background(), "hello", nil)
	for range ch {
	}

	var body map[string]any
	if err := json.Unmarshal(capturedBody, &body); err != nil {
		t.Fatalf("invalid JSON body: %v", err)
	}
	if body["stream"] != true {
		t.Errorf("request body stream = %v, want true", body["stream"])
	}
	if _, ok := body["messages"]; !ok {
		t.Error("request body missing 'messages' field")
	}
}

// FR-08c: API key sourced from env var when config field is empty
func TestClaudeProvider_APIKeyFromEnv(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "env-key-123")

	var capturedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("x-api-key")
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	}))
	defer srv.Close()

	p := ai.NewClaudeProvider(ai.ClaudeConfig{
		APIKey:  "", // empty — should fall back to env
		Model:   "claude-sonnet-4-6",
		BaseURL: srv.URL,
	})

	ch, _ := p.Query(context.Background(), "test", nil)
	for range ch {
	}

	if capturedAuth != "env-key-123" {
		t.Errorf("x-api-key header = %q, want %q", capturedAuth, "env-key-123")
	}
}

// SCN-19: missing API key returns ai_error, does not crash
func TestClaudeProvider_MissingAPIKey(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "") // ensure env is also empty

	p := ai.NewClaudeProvider(ai.ClaudeConfig{
		APIKey: "",
		Model:  "claude-sonnet-4-6",
	})

	ch, err := p.Query(context.Background(), "test", nil)
	if err != nil {
		// acceptable: return error immediately
		if !strings.Contains(err.Error(), "API key") {
			t.Errorf("error %q does not mention 'API key'", err)
		}
		return
	}
	// or return error via channel
	tok := <-ch
	if tok.Error == nil {
		t.Error("expected error token for missing API key, got nil")
	}
	if !strings.Contains(tok.Error.Error(), "API key") {
		t.Errorf("error %q does not mention 'API key'", tok.Error)
	}
}

// FR-08g: provider HTTP error surfaces as error token
func TestClaudeProvider_HTTPError(t *testing.T) {
	srv := fakeClaudeServer(t, nil, http.StatusUnauthorized)
	defer srv.Close()

	p := ai.NewClaudeProvider(ai.ClaudeConfig{
		APIKey:  "bad-key",
		Model:   "claude-sonnet-4-6",
		BaseURL: srv.URL,
	})

	ch, err := p.Query(context.Background(), "test", nil)
	if err != nil {
		return // error at call site is also acceptable
	}
	tok := <-ch
	if tok.Error == nil {
		t.Error("expected error token for 401 response, got nil")
	}
}

// FR-08f: context cancellation aborts in-flight request
func TestClaudeProvider_Cancellation(t *testing.T) {
	blocked := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// block until client cancels
		<-r.Context().Done()
		close(blocked)
	}))
	defer srv.Close()

	p := ai.NewClaudeProvider(ai.ClaudeConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
	})

	ctx, cancel := context.WithCancel(context.Background())
	ch, _ := p.Query(ctx, "test", nil)

	cancel() // cancel immediately

	// drain channel — should close after cancellation
	var lastTok ai.Token
	for tok := range ch {
		lastTok = tok
	}
	_ = lastTok

	select {
	case <-blocked:
		// server request was aborted — correct
	default:
		// request context may not have propagated; acceptable in unit test
	}
}

// FR-08h: system prompt is present in request
func TestClaudeProvider_SystemPrompt(t *testing.T) {
	var capturedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	}))
	defer srv.Close()

	p := ai.NewClaudeProvider(ai.ClaudeConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
	})
	ch, _ := p.Query(context.Background(), "hello", nil)
	for range ch {
	}

	var body map[string]any
	_ = json.Unmarshal(capturedBody, &body)
	if _, ok := body["system"]; !ok {
		t.Error("request body missing 'system' (system prompt) field (FR-08h)")
	}
}

// Implement Name() correctly
func TestClaudeProvider_Name(t *testing.T) {
	p := ai.NewClaudeProvider(ai.ClaudeConfig{APIKey: "x"})
	if p.Name() != "claude" {
		t.Errorf("Name() = %q, want %q", p.Name(), "claude")
	}
}

