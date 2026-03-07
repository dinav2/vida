// Package router implements the query routing chain.
// Tests cover FR-04 (priority ordering, edge cases).
package router_test

import (
	"context"
	"testing"

	"github.com/dinav2/vida/internal/router"
)

// --- Priority chain (FR-04a) ---

func TestRoute_CalcTakesPriority(t *testing.T) {
	// "2 + 2" matches calc AND could match "2" in app names.
	// Calc must win (priority 1).
	r := router.New(router.WithNoAI())
	result := r.Route(context.Background(), "2 + 2")
	if result.Kind != router.KindCalc {
		t.Errorf("Route(\"2 + 2\").Kind = %q, want %q", result.Kind, router.KindCalc)
	}
}

func TestRoute_ShortcutBeforeApp(t *testing.T) {
	// "g firefox" starts with shortcut prefix "g ", should resolve as shortcut not app.
	r := router.New(
		router.WithShortcuts(map[string]string{"g": "https://www.google.com/search?q=%s"}),
		router.WithNoAI(),
	)
	result := r.Route(context.Background(), "g firefox")
	if result.Kind != router.KindShortcut {
		t.Errorf("Route(\"g firefox\").Kind = %q, want %q", result.Kind, router.KindShortcut)
	}
}

func TestRoute_AppBeforeAI(t *testing.T) {
	// "firefox" matches an app; should not fall through to AI.
	r := router.New(
		router.WithApps([]router.AppEntry{{ID: "firefox.desktop", Name: "Firefox", Score: 1.0}}),
		router.WithNoAI(),
	)
	result := r.Route(context.Background(), "firefox")
	if result.Kind != router.KindAppList {
		t.Errorf("Route(\"firefox\").Kind = %q, want %q", result.Kind, router.KindAppList)
	}
}

func TestRoute_AIFallback(t *testing.T) {
	// "explain what inode is" — no calc match, no shortcut, no app → AI.
	var aiCalled bool
	r := router.New(
		router.WithAIFunc(func(_ context.Context, input string) router.Result {
			aiCalled = true
			return router.Result{Kind: router.KindAIStream}
		}),
	)
	result := r.Route(context.Background(), "explain what inode is")
	if result.Kind != router.KindAIStream {
		t.Errorf("Route(...).Kind = %q, want %q", result.Kind, router.KindAIStream)
	}
	if !aiCalled {
		t.Error("AI handler was not called for fallback input")
	}
}

// --- Edge cases (FR-04d, FR-04e) ---

func TestRoute_EmptyInput(t *testing.T) {
	var aiCalled bool
	r := router.New(
		router.WithAIFunc(func(_ context.Context, _ string) router.Result {
			aiCalled = true
			return router.Result{Kind: router.KindAIStream}
		}),
	)
	result := r.Route(context.Background(), "")
	if result.Kind != router.KindEmpty {
		t.Errorf("Route(\"\").Kind = %q, want %q", result.Kind, router.KindEmpty)
	}
	if aiCalled {
		t.Error("AI must not be called for empty input (FR-04d)")
	}
}

func TestRoute_SingleCharNoAI(t *testing.T) {
	var aiCalled bool
	r := router.New(
		router.WithAIFunc(func(_ context.Context, _ string) router.Result {
			aiCalled = true
			return router.Result{Kind: router.KindAIStream}
		}),
	)
	r.Route(context.Background(), "x")
	if aiCalled {
		t.Error("AI must not be called for input shorter than 2 chars (FR-04e)")
	}
}

// --- Cancellation propagation ---

func TestRoute_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel

	r := router.New(
		router.WithAIFunc(func(ctx context.Context, _ string) router.Result {
			// Should respect cancelled context.
			select {
			case <-ctx.Done():
				return router.Result{Kind: router.KindCancelled}
			default:
				return router.Result{Kind: router.KindAIStream}
			}
		}),
	)
	result := r.Route(ctx, "explain something")
	if result.Kind != router.KindCancelled {
		t.Errorf("Route with cancelled ctx = %q, want KindCancelled", result.Kind)
	}
}
