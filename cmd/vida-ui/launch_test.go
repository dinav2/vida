// Structural tests for app launching UI (SPEC-20260307-004).
// Verifies vida_launch_app, vida_select_row, and selection CSS exist in ui.c/main.go.
package main

import (
	"os"
	"strings"
	"testing"
)

func mainGo(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	return string(b)
}

// TR-02a/b: Selected row CSS class exists with distinct highlight.
func TestCSS_SelectedRowHighlight(t *testing.T) {
	src := uiC(t)
	assertCSS(t, src, "vida-row-selected", "selected row CSS class (TR-02a)")
	idx := strings.Index(src, "vida-row-selected")
	if idx >= 0 {
		end := idx + 200
		if end > len(src) {
			end = len(src)
		}
		if !strings.Contains(src[idx:end], "rgba(255") {
			t.Errorf(".vida-row-selected has no rgba background (TR-02b)")
		}
	}
}

// TR-03c: vida_launch_app function exists in ui.c.
func TestLayout_LaunchAppFunction(t *testing.T) {
	src := uiC(t)
	if !strings.Contains(src, "vida_launch_app") {
		t.Errorf("vida_launch_app not found in ui.c (TR-03c)")
	}
}

// TR-01b: vida_select_row function exists in ui.c for highlight management.
func TestLayout_SelectRowFunction(t *testing.T) {
	src := uiC(t)
	if !strings.Contains(src, "vida_select_row") {
		t.Errorf("vida_select_row not found in ui.c (TR-01b)")
	}
}

// TR-03c: launch uses GDesktopAppInfo.
func TestLayout_UsesGAppInfo(t *testing.T) {
	src := uiC(t)
	usesAppInfo := strings.Contains(src, "g_desktop_app_info") ||
		strings.Contains(src, "g_app_info_launch") ||
		strings.Contains(src, "GDesktopAppInfo")
	if !usesAppInfo {
		t.Errorf("vida_launch_app must use GDesktopAppInfo/g_app_info_launch (TR-03c)")
	}
}

// FR-01a: Down/Up arrow handling in goOnKeyPressed.
func TestLayout_ArrowKeyHandling(t *testing.T) {
	src := mainGo(t)
	if !strings.Contains(src, "GDK_KEY_Down") {
		t.Errorf("goOnKeyPressed missing GDK_KEY_Down handling (FR-01a)")
	}
	if !strings.Contains(src, "GDK_KEY_Up") {
		t.Errorf("goOnKeyPressed missing GDK_KEY_Up handling (FR-01a)")
	}
}

// TR-04a: current result kind tracked for Enter key branching.
func TestLayout_ResultKindTracked(t *testing.T) {
	src := mainGo(t)
	if !strings.Contains(src, "currentKind") {
		t.Errorf("currentKind variable not found in main.go (TR-04a)")
	}
}

// TR-01a: selectedIdx variable exists in main.go.
func TestLayout_SelectedIdxVar(t *testing.T) {
	src := mainGo(t)
	if !strings.Contains(src, "selectedIdx") {
		t.Errorf("selectedIdx variable not found in main.go (TR-01a)")
	}
}
