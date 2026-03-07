// Package ai defines the AIProvider interface and shared types used by all
// provider implementations.
package ai

import "context"

// Message is a single turn in a conversation.
type Message struct {
	Role    string // "user" | "assistant"
	Content string
}

// Token is a single streamed chunk from a provider.
// When Done is true the stream is complete. When Error is non-nil the stream
// has failed and no further tokens will arrive.
type Token struct {
	Text  string
	Done  bool
	Error error
}

// AIProvider is the interface all AI backends must implement.
type AIProvider interface {
	// Name returns the provider identifier (e.g. "claude", "openai").
	Name() string
	// Query sends input to the provider and returns a channel of Token chunks.
	// The channel is closed after a Token with Done==true or Error!=nil.
	// Cancelling ctx aborts the in-flight HTTP request.
	Query(ctx context.Context, input string, history []Message) (<-chan Token, error)
}
