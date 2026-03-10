// Router tests for command mode (SPEC-20260309-006).
package router_test

import (
	"context"
	"testing"

	"github.com/dinav2/vida/internal/router"
)

// FR-01a: input starting with ":" routes to KindCommandList.
func TestRoute_ColonPrefix_CommandMode(t *testing.T) {
	r := router.New(router.WithNoAI())
	result := r.Route(context.Background(), ":lock")
	if result.Kind != router.KindCommandList {
		t.Errorf("Route(\":lock\").Kind = %q, want KindCommandList (FR-01a)", result.Kind)
	}
}

// FR-01a: bare ":" returns KindCommandList with empty query.
func TestRoute_ColonAlone_CommandMode(t *testing.T) {
	r := router.New(router.WithNoAI())
	result := r.Route(context.Background(), ":")
	if result.Kind != router.KindCommandList {
		t.Errorf("Route(\":\").Kind = %q, want KindCommandList", result.Kind)
	}
	if result.CommandQuery != "" {
		t.Errorf("CommandQuery = %q, want empty for bare \":\"", result.CommandQuery)
	}
}

// FR-01b: "lock" without colon does NOT enter command mode.
func TestRoute_NocolonNoCommandMode(t *testing.T) {
	r := router.New(router.WithNoAI())
	result := r.Route(context.Background(), "lock")
	if result.Kind == router.KindCommandList {
		t.Errorf("Route(\"lock\").Kind = KindCommandList but colon prefix is required (FR-01b)")
	}
}

// CommandQuery is text after ":".
func TestRoute_ColonPrefix_QueryExtracted(t *testing.T) {
	r := router.New(router.WithNoAI())
	result := r.Route(context.Background(), ":translate hello world")
	if result.Kind != router.KindCommandList {
		t.Fatalf("Kind = %q, want KindCommandList", result.Kind)
	}
	if result.CommandQuery != "translate hello world" {
		t.Errorf("CommandQuery = %q, want \"translate hello world\"", result.CommandQuery)
	}
}

// Command mode takes priority before calc.
func TestRoute_ColonBeforeCalc(t *testing.T) {
	r := router.New(router.WithNoAI())
	// ":2 + 2" should be command mode, not calc
	result := r.Route(context.Background(), ":2 + 2")
	if result.Kind != router.KindCommandList {
		t.Errorf("Route(\":2 + 2\").Kind = %q, want KindCommandList (command mode before calc)", result.Kind)
	}
}
