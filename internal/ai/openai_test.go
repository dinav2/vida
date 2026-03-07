// Tests cover SCN-13, SCN-19, FR-08 for the OpenAI provider.
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

// fakeOpenAIServer creates a mock OpenAI streaming server.
func fakeOpenAIServer(t *testing.T, tokens []string, statusCode int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if statusCode != http.StatusOK {
			http.Error(w, `{"error":{"message":"Incorrect API key","type":"invalid_request_error"}}`, statusCode)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		for _, tok := range tokens {
			data, _ := json.Marshal(map[string]any{
				"object": "chat.completion.chunk",
				"choices": []map[string]any{
					{"delta": map[string]string{"content": tok}},
				},
			})
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
		_, _ = fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
}

// SCN-13: OpenAI provider streams tokens
func TestOpenAIProvider_Streaming(t *testing.T) {
	wantTokens := []string{"An ", "inode ", "is..."}
	srv := fakeOpenAIServer(t, wantTokens, http.StatusOK)
	defer srv.Close()

	p := ai.NewOpenAIProvider(ai.OpenAIConfig{
		APIKey:  "test-key",
		Model:   "gpt-4o",
		BaseURL: srv.URL,
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
}

// FR-08j: request uses OpenAI Chat Completions API with stream: true
func TestOpenAIProvider_RequestFormat(t *testing.T) {
	var capturedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	p := ai.NewOpenAIProvider(ai.OpenAIConfig{
		APIKey:  "test-key",
		Model:   "gpt-4o",
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
func TestOpenAIProvider_APIKeyFromEnv(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "env-openai-key")

	var capturedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	p := ai.NewOpenAIProvider(ai.OpenAIConfig{
		APIKey:  "",
		BaseURL: srv.URL,
	})
	ch, _ := p.Query(context.Background(), "test", nil)
	for range ch {
	}

	want := "Bearer env-openai-key"
	if capturedAuth != want {
		t.Errorf("Authorization = %q, want %q", capturedAuth, want)
	}
}

// SCN-19: missing API key for OpenAI
func TestOpenAIProvider_MissingAPIKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")

	p := ai.NewOpenAIProvider(ai.OpenAIConfig{
		APIKey: "",
	})

	ch, err := p.Query(context.Background(), "test", nil)
	if err != nil {
		if !strings.Contains(err.Error(), "API key") {
			t.Errorf("error %q does not mention 'API key'", err)
		}
		return
	}
	tok := <-ch
	if tok.Error == nil {
		t.Error("expected error token for missing API key")
	}
}

// FR-08g: HTTP error surfaces as error token
func TestOpenAIProvider_HTTPError(t *testing.T) {
	srv := fakeOpenAIServer(t, nil, http.StatusUnauthorized)
	defer srv.Close()

	p := ai.NewOpenAIProvider(ai.OpenAIConfig{
		APIKey:  "bad-key",
		BaseURL: srv.URL,
	})

	ch, err := p.Query(context.Background(), "test", nil)
	if err != nil {
		return
	}
	tok := <-ch
	if tok.Error == nil {
		t.Error("expected error token for 401 response")
	}
}

// FR-08f: cancellation aborts request
func TestOpenAIProvider_Cancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	p := ai.NewOpenAIProvider(ai.OpenAIConfig{
		APIKey:  "test-key",
		BaseURL: srv.URL,
	})

	ctx, cancel := context.WithCancel(context.Background())
	ch, _ := p.Query(ctx, "test", nil)
	cancel()
	for range ch {
	}
}

// Name() returns "openai"
func TestOpenAIProvider_Name(t *testing.T) {
	p := ai.NewOpenAIProvider(ai.OpenAIConfig{APIKey: "x"})
	if p.Name() != "openai" {
		t.Errorf("Name() = %q, want %q", p.Name(), "openai")
	}
}
