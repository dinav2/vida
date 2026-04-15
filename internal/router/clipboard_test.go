// Tests for clipboard command routing (SPEC-20260318-011).
// :cb and :clipboard route to KindCommandList so the daemon can intercept
// and broadcast show_clipboard.
package router_test

import (
	"context"
	"testing"

	"github.com/dinav2/vida/internal/router"
)

// SCN-01: ":cb" routes to command list with CommandQuery="cb"
func TestRoute_ColonCb_IsCommandList(t *testing.T) { // SCN-01
	r := router.New(router.WithNoAI())
	result := r.Route(context.Background(), ":cb")
	if result.Kind != router.KindCommandList {
		t.Errorf("Route(\":cb\").Kind = %q, want %q", result.Kind, router.KindCommandList)
	}
	if result.CommandQuery != "cb" {
		t.Errorf("Route(\":cb\").CommandQuery = %q, want %q", result.CommandQuery, "cb")
	}
}

// SCN-02: ":clipboard" routes to command list with CommandQuery="clipboard"
func TestRoute_ColonClipboard_IsCommandList(t *testing.T) { // SCN-02
	r := router.New(router.WithNoAI())
	result := r.Route(context.Background(), ":clipboard")
	if result.Kind != router.KindCommandList {
		t.Errorf("Route(\":clipboard\").Kind = %q, want %q", result.Kind, router.KindCommandList)
	}
	if result.CommandQuery != "clipboard" {
		t.Errorf("Route(\":clipboard\").CommandQuery = %q, want %q", result.CommandQuery, "clipboard")
	}
}
