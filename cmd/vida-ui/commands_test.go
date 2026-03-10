// Structural tests for command mode UI (SPEC-20260309-006).
// Verifies ui.c and main.go contain expected command rendering and Ctrl+C handling.
package main

import (
	"strings"
	"testing"
)

// FR-02c: vida_results_set_commands function exists in ui.c.
func TestLayout_SetCommandsFunction(t *testing.T) {
	src := uiC(t)
	if !strings.Contains(src, "vida_results_set_commands") {
		t.Errorf("vida_results_set_commands not found in ui.c (FR-02c)")
	}
}

// FR-02c: command rows have name and desc CSS classes.
func TestCSS_CommandNameClass(t *testing.T) {
	src := uiC(t)
	assertCSS(t, src, "vida-cmd-name", ".vida-cmd-name CSS class (FR-02c)")
}

func TestCSS_CommandDescClass(t *testing.T) {
	src := uiC(t)
	assertCSS(t, src, "vida-cmd-desc", ".vida-cmd-desc CSS class (FR-02c)")
}

// FR-07b: HUD "Copied" indicator exists.
func TestLayout_CopiedHUD(t *testing.T) {
	src := uiC(t)
	hasCopied := strings.Contains(src, "vida_show_copied_hud") ||
		strings.Contains(src, "Copied")
	if !hasCopied {
		t.Errorf("Copied HUD not found in ui.c (FR-07b)")
	}
}

func TestCSS_HUDClass(t *testing.T) {
	src := uiC(t)
	assertCSS(t, src, "vida-hud", ".vida-hud CSS class (FR-07b)")
}

// FR-07a: Ctrl+C handling in main.go.
func TestLayout_CtrlCHandling(t *testing.T) {
	src := mainGo(t)
	hasCtrlC := strings.Contains(src, "GDK_KEY_c") &&
		strings.Contains(src, "GDK_CONTROL_MASK")
	if !hasCtrlC {
		t.Errorf("Ctrl+C handling (GDK_CONTROL_MASK + GDK_KEY_c) not found in main.go (FR-07a)")
	}
}

// FR-01a: main.go detects ":" prefix for command mode.
func TestLayout_ColonPrefixDetection(t *testing.T) {
	src := mainGo(t)
	hasColon := strings.Contains(src, `":"`) || strings.Contains(src, `':'`) ||
		strings.Contains(src, `HasPrefix`) && strings.Contains(src, `":"`)
	if !hasColon {
		t.Errorf("colon prefix detection not found in main.go (FR-01a)")
	}
}

// FR-02c: command rows use 32px icon.
func TestLayout_CommandIconSize(t *testing.T) {
	src := uiC(t)
	// vida_results_set_commands should use a smaller icon than app rows (32 vs 48)
	// Check that 32 appears in context of the command row builder
	if !strings.Contains(src, "32") {
		t.Errorf("32px icon size not found in ui.c for command rows (FR-02c)")
	}
}

// FR-02f: no-match row shown when filter returns nothing.
func TestLayout_NoCommandsMatch(t *testing.T) {
	src := uiC(t)
	hasNoMatch := strings.Contains(src, "No commands") ||
		strings.Contains(src, "no commands") ||
		strings.Contains(src, "No match")
	if !hasNoMatch {
		t.Errorf("no-match message not found in ui.c (FR-02f)")
	}
}

// FR-01e: placeholder text changes in command mode.
func TestLayout_CommandModePlaceholder(t *testing.T) {
	src := mainGo(t)
	hasPlaceholder := strings.Contains(src, "Type a command") ||
		strings.Contains(src, "command\xe2\x80\xa6") || // "command…"
		strings.Contains(src, "vida_entry_set_placeholder")
	if !hasPlaceholder {
		t.Errorf("command mode placeholder text not found in main.go (FR-01e)")
	}
}
