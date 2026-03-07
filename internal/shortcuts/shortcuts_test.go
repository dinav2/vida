// Package shortcuts implements web search shortcut prefix expansion.
// Tests cover SCN-07, SCN-08, FR-07.
package shortcuts_test

import (
	"testing"

	"github.com/dinav2/vida/internal/shortcuts"
)

var defaultShortcuts = map[string]string{
	"g":  "https://www.google.com/search?q=%s",
	"gh": "https://github.com/search?q=%s",
	"yt": "https://www.youtube.com/results?search_query=%s",
	"dd": "https://duckduckgo.com/?q=%s",
}

// SCN-07: Google shortcut with URL encoding
func TestResolve_Google(t *testing.T) {
	h := shortcuts.New(defaultShortcuts)

	url, ok := h.Resolve("g linux kernel")
	if !ok {
		t.Fatal("Resolve(\"g linux kernel\"): expected match, got none")
	}
	want := "https://www.google.com/search?q=linux+kernel"
	if url != want {
		t.Errorf("URL = %q, want %q", url, want)
	}
}

// SCN-08: custom shortcut
func TestResolve_Custom(t *testing.T) {
	custom := map[string]string{
		"docs": "https://pkg.go.dev/search?q=%s",
	}
	h := shortcuts.New(custom)

	url, ok := h.Resolve("docs context")
	if !ok {
		t.Fatal("Resolve(\"docs context\"): expected match, got none")
	}
	want := "https://pkg.go.dev/search?q=context"
	if url != want {
		t.Errorf("URL = %q, want %q", url, want)
	}
}

// FR-07e: all four default shortcuts are present
func TestDefaults_AllPresent(t *testing.T) {
	h := shortcuts.NewWithDefaults()

	cases := []struct {
		input string
	}{
		{"g test"},
		{"gh test"},
		{"yt test"},
		{"dd test"},
	}
	for _, tc := range cases {
		_, ok := h.Resolve(tc.input)
		if !ok {
			t.Errorf("Resolve(%q): default shortcut not found", tc.input)
		}
	}
}

// FR-07f: user config overrides default for same prefix
func TestResolve_UserOverridesDefault(t *testing.T) {
	overrides := map[string]string{
		"g": "https://duckduckgo.com/?q=%s", // override Google prefix
	}
	h := shortcuts.NewWithOverrides(defaultShortcuts, overrides)

	url, ok := h.Resolve("g test")
	if !ok {
		t.Fatal("expected match")
	}
	if url == "https://www.google.com/search?q=test" {
		t.Error("user override not applied; still using default Google URL")
	}
	want := "https://duckduckgo.com/?q=test"
	if url != want {
		t.Errorf("URL = %q, want %q", url, want)
	}
}

// FR-07c: query is URL-encoded (spaces → +, special chars encoded)
func TestResolve_URLEncoding(t *testing.T) {
	h := shortcuts.New(defaultShortcuts)

	url, ok := h.Resolve("g c++ programming")
	if !ok {
		t.Fatal("expected match")
	}
	// "c++" → "c%2B%2B" or "c+++" depending on encoding strategy.
	// At minimum, spaces must be encoded and URL must not contain raw spaces.
	for _, r := range url {
		if r == ' ' {
			t.Errorf("URL contains raw space: %q", url)
			break
		}
	}
}

// Non-matching input returns false
func TestResolve_NoMatch(t *testing.T) {
	h := shortcuts.New(defaultShortcuts)

	_, ok := h.Resolve("firefox")
	if ok {
		t.Error("Resolve(\"firefox\"): expected no match, got match")
	}

	_, ok = h.Resolve("explain inode")
	if ok {
		t.Error("Resolve(\"explain inode\"): expected no match, got match")
	}
}

// Prefix without trailing space/query should not match
func TestResolve_PrefixOnly(t *testing.T) {
	h := shortcuts.New(defaultShortcuts)

	_, ok := h.Resolve("g")
	if ok {
		t.Error("Resolve(\"g\") with no query: expected no match")
	}
}

// Empty query string after prefix
func TestResolve_EmptyQuery(t *testing.T) {
	h := shortcuts.New(defaultShortcuts)

	_, ok := h.Resolve("g ")
	if ok {
		// Could be either way — document the expected behavior:
		// "g " with empty query should not open a browser (no useful URL).
		t.Log("Resolve(\"g \"): matched with empty query — verify this is intentional")
	}
}
