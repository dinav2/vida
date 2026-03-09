// Structural tests for app icon display (SPEC-20260308-005).
// Verifies ui.c and main.go contain the expected icon-loading code and CSS.
// Runs without a Wayland display.
package main

import (
	"strings"
	"testing"
)

// --- FR-01: Icon display ---

// FR-01a/d: .vida-app-icon CSS class exists.
func TestCSS_AppIconClass(t *testing.T) {
	src := uiC(t)
	assertCSS(t, src, "vida-app-icon", ".vida-app-icon CSS class (FR-01d)")
}

// FR-02c: Icon has border-radius for rounded-square look.
func TestCSS_AppIconBorderRadius(t *testing.T) {
	src := uiC(t)
	// Find the vida-app-icon block and confirm border-radius is there
	idx := strings.Index(src, "vida-app-icon")
	if idx < 0 {
		t.Fatal("vida-app-icon not found in ui.c")
	}
	chunk := src[idx:]
	end := strings.Index(chunk, "}")
	if end > 0 {
		chunk = chunk[:end]
	}
	if !strings.Contains(chunk, "border-radius") {
		t.Errorf(".vida-app-icon missing border-radius (FR-02c)")
	}
}

// FR-02b: Icon has left and right margin defined.
func TestCSS_AppIconMargin(t *testing.T) {
	src := uiC(t)
	idx := strings.Index(src, "vida-app-icon")
	if idx < 0 {
		t.Fatal("vida-app-icon not found in ui.c")
	}
	chunk := src[idx:]
	end := strings.Index(chunk, "}")
	if end > 0 {
		chunk = chunk[:end]
	}
	if !strings.Contains(chunk, "margin") {
		t.Errorf(".vida-app-icon missing margin (FR-02b)")
	}
}

// FR-02d: Row min-height is at least 56px.
func TestCSS_RowMinHeight56(t *testing.T) {
	src := uiC(t)
	// Accept 56px or larger
	has56 := strings.Contains(src, "min-height: 56px") ||
		strings.Contains(src, "min-height: 60px") ||
		strings.Contains(src, "min-height: 64px") ||
		strings.Contains(src, "min-height:56px")
	if !has56 {
		t.Errorf(".vida-row min-height must be >= 56px (FR-02d)")
	}
}

// FR-01b: GDesktopAppInfo / GIcon used for icon loading.
func TestLayout_UsesGIconForAppIcon(t *testing.T) {
	src := uiC(t)
	usesGIcon := strings.Contains(src, "gtk_image_new_from_gicon") ||
		strings.Contains(src, "g_themed_icon_new")
	if !usesGIcon {
		t.Errorf("icon loading must use g_themed_icon_new / gtk_image_new_from_gicon (FR-01b)")
	}
}

// FR-01c: Fallback icon name exists in source.
func TestLayout_FallbackIconName(t *testing.T) {
	src := uiC(t)
	if !strings.Contains(src, "application-x-executable") {
		t.Errorf("fallback icon application-x-executable[-symbolic] not found in ui.c (FR-01c)")
	}
}

// FR-01a: gtk_image_set_pixel_size called with 48.
func TestLayout_IconPixelSize48(t *testing.T) {
	src := uiC(t)
	has48 := strings.Contains(src, "gtk_image_set_pixel_size") &&
		(strings.Contains(src, ", 48)") || strings.Contains(src, ",48)"))
	if !has48 {
		t.Errorf("gtk_image_set_pixel_size(img, 48) not found in ui.c (FR-01a)")
	}
}

// FR-04a: vida_results_set_apps signature includes icons parameter.
func TestLayout_SetAppsHasIconsParam(t *testing.T) {
	src := uiC(t)
	// Signature must include a second char** for icons
	// Look for the function definition with two const char** params
	hasIcons := strings.Contains(src, "vida_results_set_apps") &&
		(strings.Contains(src, "const char **icons") ||
			strings.Contains(src, "const char** icons") ||
			strings.Contains(src, "char **icons"))
	if !hasIcons {
		t.Errorf("vida_results_set_apps must have icons parameter (FR-04a)")
	}
}

// FR-04b: main.go passes icons to vida_results_set_apps.
func TestLayout_MainGoPassesIcons(t *testing.T) {
	src := mainGo(t)
	if !strings.Contains(src, "Icons") {
		t.Errorf("main.go must parse Icons from IPC response and pass to C (FR-04b)")
	}
}

// FR-04c: main.go extracts icon names from resp (resp.Icons).
func TestLayout_MainGoExtractsIconField(t *testing.T) {
	src := mainGo(t)
	hasIconField := strings.Contains(src, "resp.Icons") ||
		strings.Contains(src, ".Icons")
	if !hasIconField {
		t.Errorf("main.go must read Icons field from IPC result message (FR-04c)")
	}
}
