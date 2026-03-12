// Structural tests for the answer bar (SPEC-20260310-009).
package main

import (
	"strings"
	"testing"
)

// FR-01a/TR-01: vida_answer_set exists in ui.c.
func TestLayout_AnswerSetFunction(t *testing.T) {
	src := uiC(t)
	if !strings.Contains(src, "vida_answer_set") {
		t.Errorf("vida_answer_set not found in ui.c (TR-01)")
	}
}

// TR-01: vida_answer_clear exists in ui.c.
func TestLayout_AnswerClearFunction(t *testing.T) {
	src := uiC(t)
	if !strings.Contains(src, "vida_answer_clear") {
		t.Errorf("vida_answer_clear not found in ui.c (TR-01)")
	}
}

// TR-03: .vida-answer CSS class present.
func TestCSS_AnswerBarClass(t *testing.T) {
	src := uiC(t)
	assertCSS(t, src, "vida-answer", ".vida-answer CSS class (TR-03)")
}

// TR-03: .vida-answer-value CSS class present.
func TestCSS_AnswerValueClass(t *testing.T) {
	src := uiC(t)
	assertCSS(t, src, "vida-answer-value", ".vida-answer-value CSS class (TR-03)")
}

// TR-03: .vida-answer-type CSS class present.
func TestCSS_AnswerTypeClass(t *testing.T) {
	src := uiC(t)
	assertCSS(t, src, "vida-answer-type", ".vida-answer-type CSS class (TR-03)")
}

// FR-01e: answer bar uses a non-interactive container, not GtkButton.
func TestLayout_AnswerNotButton(t *testing.T) {
	src := uiC(t)
	// The answer bar widget must be a GtkBox/container, not a button.
	// Check vida_answer_set uses gtk_label_set_text not gtk_button_new.
	if strings.Contains(src, "gtk_button_new") {
		// Allow buttons elsewhere (app rows etc) — just ensure vida_answer_set
		// function body does not create a button.
		// We check that vida-answer CSS class is not applied to a button widget.
		answerSection := extractFuncBody(src, "vida_answer_set")
		if strings.Contains(answerSection, "gtk_button_new") {
			t.Errorf("vida_answer_set must not create a GtkButton (FR-01e)")
		}
	}
}

// TR-04: main.go calls vida_answer_set for "calc" result kind (SCN-01).
func TestLayout_AnswerSetCalledForCalc(t *testing.T) {
	src := mainGo(t)
	if !strings.Contains(src, "vida_answer_set") {
		t.Errorf("vida_answer_set not called in main.go (TR-04)")
	}
}

// TR-04: main.go calls vida_answer_clear for non-answer result kinds (SCN-04).
func TestLayout_AnswerClearCalledForOtherKinds(t *testing.T) {
	src := mainGo(t)
	if !strings.Contains(src, "vida_answer_clear") {
		t.Errorf("vida_answer_clear not called in main.go (TR-04)")
	}
}

// TR-04: gAnswer global exists in main.go.
func TestLayout_GAnswerGlobal(t *testing.T) {
	src := mainGo(t)
	if !strings.Contains(src, "gAnswer") {
		t.Errorf("gAnswer global not found in main.go (TR-04)")
	}
}

// extractFuncBody returns a rough slice of src starting at the first line
// containing funcName and ending at the next top-level closing brace.
// Used for narrow checks within a specific function.
func extractFuncBody(src, funcName string) string {
	idx := strings.Index(src, funcName)
	if idx < 0 {
		return ""
	}
	end := strings.Index(src[idx:], "\n}\n")
	if end < 0 {
		return src[idx:]
	}
	return src[idx : idx+end]
}
