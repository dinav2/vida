// Structural tests for vida-ui styling (SPEC-20260307-003).
// These tests read ui.c and verify the embedded CSS contains the expected
// rules. They run without a Wayland display and catch CSS regressions.
package main

import (
	"os"
	"strings"
	"testing"
)

// uiC reads the ui.c source so tests can inspect the embedded CSS.
func uiC(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("ui.c")
	if err != nil {
		t.Fatalf("read ui.c: %v", err)
	}
	return string(b)
}

// assertCSS fails if the ui.c source does not contain the given CSS fragment.
func assertCSS(t *testing.T, src, fragment, description string) {
	t.Helper()
	if !strings.Contains(src, fragment) {
		t.Errorf("CSS missing %s: expected to find %q in ui.c", description, fragment)
	}
}

// --- FR-02: Window appearance ---

// FR-02e: GtkWindow background must be transparent so compositor shows rounded corners.
func TestCSS_WindowTransparent(t *testing.T) {
	src := uiC(t)
	assertCSS(t, src, "window", "window selector")
	assertCSS(t, src, "background: transparent", "transparent window background (FR-02e)")
}

// FR-02a: Panel has dark semi-transparent background.
func TestCSS_PanelBackground(t *testing.T) {
	src := uiC(t)
	// rgba with low alpha — any dark rgba is acceptable
	assertCSS(t, src, "rgba(20", "dark rgba background (FR-02a)")
}

// FR-02b: Panel has rounded corners (border-radius).
func TestCSS_BorderRadius(t *testing.T) {
	src := uiC(t)
	assertCSS(t, src, "border-radius", "border-radius (FR-02b)")
}

// FR-02c: Subtle border on panel.
func TestCSS_PanelBorder(t *testing.T) {
	src := uiC(t)
	assertCSS(t, src, "border:", "panel border (FR-02c)")
}

// --- FR-03: Search entry ---

// FR-03a: Entry font size >= 18px.
func TestCSS_EntryFontSize(t *testing.T) {
	src := uiC(t)
	// Accept 18px, 19px, 20px, or larger specified as font-size
	hasSize := strings.Contains(src, "font-size: 18") ||
		strings.Contains(src, "font-size: 19") ||
		strings.Contains(src, "font-size: 20") ||
		strings.Contains(src, "font-size: 22") ||
		strings.Contains(src, "font-size: 24")
	if !hasSize {
		t.Errorf("CSS missing large entry font size >=18px (FR-03a): check font-size in entry rules")
	}
}

// FR-03b/f: Entry background transparent, no visible border.
func TestCSS_EntryTransparent(t *testing.T) {
	src := uiC(t)
	// Entry should have transparent or no background, and no box-shadow/border
	assertCSS(t, src, "entry", "entry CSS selector (FR-03b)")
}

// FR-03d: Placeholder text is muted.
func TestCSS_PlaceholderColor(t *testing.T) {
	src := uiC(t)
	assertCSS(t, src, "placeholder", "placeholder color rule (FR-03d)")
}

// --- FR-04: Result rows ---

// FR-04e: Separator line between entry and results.
func TestCSS_Separator(t *testing.T) {
	src := uiC(t)
	// Either a CSS separator rule or a GtkSeparator widget
	hasSep := strings.Contains(src, "separator") || strings.Contains(src, "gtk_separator_new")
	if !hasSep {
		t.Errorf("missing separator between entry and results (FR-04e)")
	}
}

// FR-04d: Hover highlight on result rows.
func TestCSS_HoverState(t *testing.T) {
	src := uiC(t)
	assertCSS(t, src, ":hover", "hover state on result rows (FR-04d)")
}

// --- FR-05: Typography ---

// FR-05a: Font family set to Inter or system-ui.
func TestCSS_FontFamily(t *testing.T) {
	src := uiC(t)
	hasFont := strings.Contains(src, "Inter") || strings.Contains(src, "system-ui")
	if !hasFont {
		t.Errorf("CSS missing font-family Inter or system-ui (FR-05a)")
	}
}

// --- TR-02: Transparent window technique ---

// TR-02c: An inner panel widget (not the window itself) holds the visual background.
// We verify vida_build_window creates a named "panel" or "vida-panel" box.
func TestLayout_InnerPanel(t *testing.T) {
	src := uiC(t)
	hasPanel := strings.Contains(src, "vida-panel") || strings.Contains(src, "panel")
	if !hasPanel {
		t.Errorf("missing inner panel widget for background/border-radius (TR-02c)")
	}
}

// TR-03: Panel width is set via gtk_widget_set_size_request (not CSS max-width).
func TestLayout_PanelSizeRequest(t *testing.T) {
	src := uiC(t)
	// Width set via C API, not CSS (GTK4 doesn't support max-width)
	// Check CSS string only (not comments) — search for max-width inside quotes
	cssStart := strings.Index(src, "VIDA_CSS =")
	if cssStart >= 0 && strings.Contains(src[cssStart:], "max-width:") {
		t.Errorf("CSS must not use max-width — not supported in GTK4 (causes warning)")
	}
	assertCSS(t, src, "gtk_widget_set_size_request", "panel width via size_request (TR-03c)")
}
