// Package router implements the query routing chain for vida.
// Priority: calc → shortcuts → apps → AI (FR-04).
package router

import (
	"context"

	"github.com/dinav2/vida/internal/apps"
	"github.com/dinav2/vida/internal/calc"
	"github.com/dinav2/vida/internal/shortcuts"
)

// Kind identifies the type of routing result.
type Kind string

const (
	KindEmpty     Kind = "empty"
	KindCalc      Kind = "calc"
	KindShortcut  Kind = "shortcut"
	KindAppList   Kind = "app_list"
	KindAIStream  Kind = "ai_stream"
	KindCancelled Kind = "cancelled"
)

// AppEntry is a simplified app result used for testing and dependency injection.
type AppEntry struct {
	ID    string
	Name  string
	Icon  string
	Exec  string
	Score float64
}

// Result is the output of a routing decision.
type Result struct {
	Kind Kind

	// Calc result
	CalcValue string

	// Shortcut result
	ShortcutURL string

	// App results
	Apps []AppEntry

	// AI result channel (non-nil when Kind == KindAIStream)
	AIFunc func(context.Context, string) Result
}

// Router routes queries to the appropriate handler.
type Router struct {
	shortcutHandler *shortcuts.Handler
	appIndex        *apps.Index
	appEntries      []AppEntry // for test injection
	aiFunc          func(context.Context, string) Result
	noAI            bool
}

// Option configures a Router.
type Option func(*Router)

// WithShortcuts sets the shortcut map.
func WithShortcuts(m map[string]string) Option {
	return func(r *Router) {
		r.shortcutHandler = shortcuts.New(m)
	}
}

// WithApps sets a fixed list of app entries (for testing).
func WithApps(entries []AppEntry) Option {
	return func(r *Router) {
		r.appEntries = entries
	}
}

// WithAppIndex sets a real app index.
func WithAppIndex(idx *apps.Index) Option {
	return func(r *Router) {
		r.appIndex = idx
	}
}

// WithAIFunc sets the AI handler function.
func WithAIFunc(fn func(context.Context, string) Result) Option {
	return func(r *Router) {
		r.aiFunc = fn
	}
}

// WithNoAI disables the AI fallback.
func WithNoAI() Option {
	return func(r *Router) {
		r.noAI = true
	}
}

// New creates a Router with the given options.
func New(opts ...Option) *Router {
	r := &Router{}
	for _, o := range opts {
		o(r)
	}
	return r
}

// Route dispatches input through the priority chain and returns a Result.
func (r *Router) Route(ctx context.Context, input string) Result {
	// FR-04d: empty input
	if input == "" {
		return Result{Kind: KindEmpty}
	}

	// 1. Calc (highest priority)
	if calc.IsExpression(input) {
		val, err := calc.Eval(input)
		if err == nil {
			return Result{Kind: KindCalc, CalcValue: calc.Format(val)}
		}
	}

	// 2. Shortcuts
	if r.shortcutHandler != nil {
		if url, ok := r.shortcutHandler.Resolve(input); ok {
			return Result{Kind: KindShortcut, ShortcutURL: url}
		}
	}

	// 3. App search
	if r.appIndex != nil {
		matched := r.appIndex.Search(input, 10)
		if len(matched) > 0 {
			entries := make([]AppEntry, len(matched))
			for i, a := range matched {
				entries[i] = AppEntry{ID: a.ID, Name: a.Name, Icon: a.Icon, Exec: a.Exec}
			}
			return Result{Kind: KindAppList, Apps: entries}
		}
	} else if len(r.appEntries) > 0 {
		// test injection: filter by name prefix
		var matched []AppEntry
		for _, e := range r.appEntries {
			matched = append(matched, e)
		}
		if len(matched) > 0 {
			return Result{Kind: KindAppList, Apps: matched}
		}
	}

	// FR-04e: skip AI for very short input
	if len(input) < 2 {
		return Result{Kind: KindEmpty}
	}

	// 4. AI fallback
	if !r.noAI && r.aiFunc != nil {
		return r.aiFunc(ctx, input)
	}

	return Result{Kind: KindEmpty}
}
