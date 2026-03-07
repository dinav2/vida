// Package shortcuts implements web search shortcut prefix expansion.
// A shortcut maps a short prefix (e.g. "g") to a URL template (e.g.
// "https://www.google.com/search?q=%s") and expands "g linux kernel"
// into a ready-to-open browser URL.
package shortcuts

import (
	"net/url"
	"strings"
)

// Default shortcuts bundled with vida (FR-07e).
var defaults = map[string]string{
	"g":  "https://www.google.com/search?q=%s",
	"gh": "https://github.com/search?q=%s",
	"yt": "https://www.youtube.com/results?search_query=%s",
	"dd": "https://duckduckgo.com/?q=%s",
}

// Handler resolves shortcut prefix inputs to browser URLs.
type Handler struct {
	shortcuts map[string]string
}

// New creates a Handler using exactly the provided shortcuts map.
func New(shortcuts map[string]string) *Handler {
	return &Handler{shortcuts: shortcuts}
}

// NewWithDefaults creates a Handler pre-loaded with the four built-in shortcuts.
func NewWithDefaults() *Handler {
	cp := make(map[string]string, len(defaults))
	for k, v := range defaults {
		cp[k] = v
	}
	return &Handler{shortcuts: cp}
}

// NewWithOverrides creates a Handler that merges base with overrides,
// where overrides take precedence on key conflict (FR-07f).
func NewWithOverrides(base, overrides map[string]string) *Handler {
	merged := make(map[string]string, len(base)+len(overrides))
	for k, v := range base {
		merged[k] = v
	}
	for k, v := range overrides {
		merged[k] = v
	}
	return &Handler{shortcuts: merged}
}

// Resolve checks whether input matches a shortcut prefix and, if so,
// returns the expanded URL and true. Returns ("", false) when:
//   - No space is present in input (bare prefix, no query).
//   - The query portion after the prefix is empty.
//   - The prefix is not a known shortcut.
//
// Query is URL-encoded with spaces as '+' (FR-07c).
func (h *Handler) Resolve(input string) (string, bool) {
	idx := strings.Index(input, " ")
	if idx < 0 {
		return "", false
	}

	prefix := input[:idx]
	query := strings.TrimSpace(input[idx+1:])
	if query == "" {
		return "", false
	}

	tmpl, ok := h.shortcuts[prefix]
	if !ok {
		return "", false
	}

	encoded := url.QueryEscape(query)
	return strings.Replace(tmpl, "%s", encoded, 1), true
}
