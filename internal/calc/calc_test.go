// Package calc implements local math expression evaluation.
// Tests cover SCN-04, SCN-05, SCN-06 and FR-05.
package calc_test

import (
	"testing"
	"time"

	"github.com/dinav2/vida/internal/calc"
)

// --- Detection (TR-04) ---

func TestIsExpression(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		// Should detect as math
		{"42 * 1.5", true},
		{"sqrt(144)", true},
		{"(100 + 200) / 3", true},
		{"10 / 0", true},
		{"2^8", true},
		{"10 % 3", true},
		{"abs(-5)", true},
		{"floor(3.7)", true},
		{"ceil(3.2)", true},
		{"round(3.5)", true},
		{"log(100)", true},
		{"sin(0)", true},
		{"cos(0)", true},
		{"tan(0)", true},
		{"1+2", true},
		{"3.14 * 2", true},

		// Should NOT detect as math (falls through to next handler)
		{"hello world", false},
		{"what is 2 times 3", false}, // long alpha word "times"
		{"g linux kernel", false},    // shortcut prefix
		{"firefox", false},           // app name
		{"explain what inode is", false},
		{"", false},
		{"42", false}, // digit only, no operator or function
	}

	for _, tc := range cases {
		got := calc.IsExpression(tc.input)
		if got != tc.want {
			t.Errorf("IsExpression(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

// --- Evaluation (FR-05a..FR-05g) ---

// SCN-04: basic arithmetic
func TestEval_BasicArithmetic(t *testing.T) {
	cases := []struct {
		input string
		want  float64
	}{
		{"42 * 1.5", 63},
		{"1 + 2", 3},
		{"10 - 4", 6},
		{"100 / 4", 25},
		{"2 ^ 8", 256},
		{"10 % 3", 1},
		{"(100 + 200) / 3", 100},
	}

	for _, tc := range cases {
		result, err := calc.Eval(tc.input)
		if err != nil {
			t.Errorf("Eval(%q) unexpected error: %v", tc.input, err)
			continue
		}
		if result != tc.want {
			t.Errorf("Eval(%q) = %v, want %v", tc.input, result, tc.want)
		}
	}
}

// SCN-05: math functions
func TestEval_Functions(t *testing.T) {
	cases := []struct {
		input string
		want  float64
	}{
		{"sqrt(144)", 12},
		{"abs(-7)", 7},
		{"abs(7)", 7},
		{"floor(3.9)", 3},
		{"ceil(3.1)", 4},
		{"round(3.5)", 4},
		{"round(3.4)", 3},
	}

	for _, tc := range cases {
		result, err := calc.Eval(tc.input)
		if err != nil {
			t.Errorf("Eval(%q) unexpected error: %v", tc.input, err)
			continue
		}
		if result != tc.want {
			t.Errorf("Eval(%q) = %v, want %v", tc.input, result, tc.want)
		}
	}
}

// FR-05a: trig functions exist and return without error
func TestEval_TrigFunctions(t *testing.T) {
	cases := []string{"sin(0)", "cos(0)", "tan(0)"}
	for _, input := range cases {
		_, err := calc.Eval(input)
		if err != nil {
			t.Errorf("Eval(%q) unexpected error: %v", input, err)
		}
	}
}

// SCN-06: division by zero returns an error, does not panic
func TestEval_DivisionByZero(t *testing.T) {
	_, err := calc.Eval("10 / 0")
	if err == nil {
		t.Error("Eval(\"10 / 0\") expected error, got nil")
	}
}

// FR-05g: non-numeric input returns ErrNotExpression or similar non-crash
func TestEval_NonNumeric(t *testing.T) {
	_, err := calc.Eval("hello + world")
	if err == nil {
		t.Error("Eval(\"hello + world\") expected error, got nil")
	}
}

// FR-05e: result is available in < 5ms (AC-P4 proxy)
func TestEval_Performance(t *testing.T) {
	start := time.Now()
	_, _ = calc.Eval("42 * 1.5")
	elapsed := time.Since(start)
	if elapsed > 5*time.Millisecond {
		t.Errorf("Eval took %v, want < 5ms", elapsed)
	}
}

// --- Format helper ---

func TestFormat(t *testing.T) {
	cases := []struct {
		input float64
		want  string
	}{
		{63, "63"},
		{63.5, "63.5"},
		{12, "12"},
		{3.14159, "3.14159"},
	}
	for _, tc := range cases {
		got := calc.Format(tc.input)
		if got != tc.want {
			t.Errorf("Format(%v) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
