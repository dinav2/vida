// Structural tests for chat view (SPEC-20260309-007).
// These tests inspect ui.c and main.go for the expected chat view
// implementation without requiring a Wayland display.
package main

import (
	"strings"
	"testing"
)

// FR-01c: UI uses GtkStack so chat and palette share one window.
func TestChat_GtkStack(t *testing.T) {
	src := uiC(t)
	if !strings.Contains(src, "gtk_stack_new") {
		t.Errorf("GtkStack not created in ui.c (FR-01c)")
	}
}

// FR-01c: Stack has a "palette" page.
func TestChat_StackPalettePage(t *testing.T) {
	src := uiC(t)
	if !strings.Contains(src, `"palette"`) {
		t.Errorf(`stack child "palette" not found in ui.c (FR-01c)`)
	}
}

// FR-01c: Stack has a "chat" page.
func TestChat_StackChatPage(t *testing.T) {
	src := uiC(t)
	if !strings.Contains(src, `"chat"`) {
		t.Errorf(`stack child "chat" not found in ui.c (FR-01c)`)
	}
}

// FR-02a: GtkScrolledWindow used for message history.
func TestChat_ScrolledWindow(t *testing.T) {
	src := uiC(t)
	if !strings.Contains(src, "gtk_scrolled_window_new") {
		t.Errorf("GtkScrolledWindow not created in ui.c (FR-02a)")
	}
}

// FR-02b: .vida-msg-user CSS class present.
func TestCSS_ChatUserBubble(t *testing.T) {
	src := uiC(t)
	assertCSS(t, src, "vida-msg-user", ".vida-msg-user CSS class (FR-02b)")
}

// FR-02b: .vida-msg-ai CSS class present.
func TestCSS_ChatAIBubble(t *testing.T) {
	src := uiC(t)
	assertCSS(t, src, "vida-msg-ai", ".vida-msg-ai CSS class (FR-02b)")
}

// FR-02e: .vida-chat-entry CSS class present.
func TestCSS_ChatEntry(t *testing.T) {
	src := uiC(t)
	assertCSS(t, src, "vida-chat-entry", ".vida-chat-entry CSS class (FR-02e)")
}

// FR-02g: .vida-chat-header CSS class present for header bar.
func TestCSS_ChatHeader(t *testing.T) {
	src := uiC(t)
	assertCSS(t, src, "vida-chat-header", ".vida-chat-header CSS class (FR-02g)")
}

// TR-01: vida_chat_show function exists in ui.c (transitions to chat view).
func TestChat_ShowFunction(t *testing.T) {
	src := uiC(t)
	if !strings.Contains(src, "vida_chat_show") {
		t.Errorf("vida_chat_show not found in ui.c")
	}
}

// TR-01: vida_chat_clear function exists in ui.c (clears history on back).
func TestChat_ClearFunction(t *testing.T) {
	src := uiC(t)
	if !strings.Contains(src, "vida_chat_clear") {
		t.Errorf("vida_chat_clear not found in ui.c")
	}
}

// TR-01: vida_chat_append_message function exists for adding bubbles.
func TestChat_AppendMessageFunction(t *testing.T) {
	src := uiC(t)
	if !strings.Contains(src, "vida_chat_append_message") {
		t.Errorf("vida_chat_append_message not found in ui.c")
	}
}

// FR-01a: main.go transitions to chat view when AI command starts streaming.
func TestChat_TransitionInMainGo(t *testing.T) {
	src := mainGo(t)
	if !strings.Contains(src, "vida_chat_show") {
		t.Errorf("vida_chat_show not called from main.go (FR-01a)")
	}
}

// FR-03a/FR-03b: main.go handles Escape and Ctrl+B to return to palette.
func TestChat_EscapeReturnsToPalette(t *testing.T) {
	src := mainGo(t)
	if !strings.Contains(src, "vida_chat_clear") {
		t.Errorf("vida_chat_clear not called on Escape/back in main.go (FR-03a)")
	}
}

// FR-04b: history field passed in IPC query for follow-up turns.
func TestChat_HistoryFieldInIPC(t *testing.T) {
	src := mainGo(t)
	if !strings.Contains(src, "History") && !strings.Contains(src, "history") {
		t.Errorf("history field not found in main.go IPC query (FR-04b)")
	}
}

// FR-04d: chat entry disabled state managed in main.go.
func TestChat_EntryDisabledDuringStream(t *testing.T) {
	src := uiC(t)
	if !strings.Contains(src, "vida_chat_set_entry_sensitive") &&
		!strings.Contains(src, "gtk_widget_set_sensitive") {
		t.Errorf("chat entry sensitive/disabled not managed in ui.c (FR-04d)")
	}
}
