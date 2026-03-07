// Package calc implements local math expression detection and evaluation.
// No network calls are made; all evaluation is purely local.
package calc

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
)

// knownFunctions maps function names to their implementations.
var knownFunctions = map[string]func(float64) float64{
	"sqrt":  math.Sqrt,
	"abs":   math.Abs,
	"floor": math.Floor,
	"ceil":  math.Ceil,
	"round": math.Round,
	"log":   math.Log10,
	"sin":   math.Sin,
	"cos":   math.Cos,
	"tan":   math.Tan,
}

// IsExpression reports whether input looks like a math expression suitable
// for Eval. Implements the TR-04 heuristic:
//  1. Input must contain at least one digit.
//  2. Every alphabetic word must be a known function name.
//  3. Input must contain at least one operator or function call.
func IsExpression(input string) bool {
	if input == "" {
		return false
	}

	hasDigit := false
	for _, r := range input {
		if unicode.IsDigit(r) {
			hasDigit = true
			break
		}
	}
	if !hasDigit {
		return false
	}

	words := alphaWords(input)
	for _, w := range words {
		if _, ok := knownFunctions[w]; !ok {
			return false
		}
	}

	hasOp := strings.ContainsAny(input, "+-*/^%")
	hasFn := len(words) > 0
	return hasOp || hasFn
}

// Eval evaluates a mathematical expression string and returns the result.
// Returns an error if the expression is invalid or causes a math error
// (e.g. division by zero).
func Eval(input string) (float64, error) {
	p := &parser{s: strings.TrimSpace(input)}
	v, err := p.parseExpr()
	if err != nil {
		return 0, err
	}
	p.skipSpace()
	if p.pos < len(p.s) {
		return 0, fmt.Errorf("unexpected input at position %d: %q", p.pos, p.s[p.pos:])
	}
	return v, nil
}

// Format returns a clean string representation of v, stripping unnecessary
// trailing zeros (e.g. 63.0 → "63", 63.5 → "63.5").
func Format(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// alphaWords returns all consecutive-letter sequences in s.
func alphaWords(s string) []string {
	var words []string
	var cur strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) {
			cur.WriteRune(r)
		} else if cur.Len() > 0 {
			words = append(words, cur.String())
			cur.Reset()
		}
	}
	if cur.Len() > 0 {
		words = append(words, cur.String())
	}
	return words
}

// --- Recursive descent parser ---
// Grammar:
//   expr    = term   ( ('+' | '-') term   )*
//   term    = power  ( ('*' | '/' | '%') power )*
//   power   = unary  ( '^' unary )*
//   unary   = '-' unary | primary
//   primary = NUMBER | IDENT '(' expr ')' | '(' expr ')'

type parser struct {
	s   string
	pos int
}

func (p *parser) skipSpace() {
	for p.pos < len(p.s) && p.s[p.pos] == ' ' {
		p.pos++
	}
}

func (p *parser) parseExpr() (float64, error) {
	v, err := p.parseTerm()
	if err != nil {
		return 0, err
	}
	for {
		p.skipSpace()
		if p.pos >= len(p.s) {
			break
		}
		op := p.s[p.pos]
		if op != '+' && op != '-' {
			break
		}
		p.pos++
		right, err := p.parseTerm()
		if err != nil {
			return 0, err
		}
		if op == '+' {
			v += right
		} else {
			v -= right
		}
	}
	return v, nil
}

func (p *parser) parseTerm() (float64, error) {
	v, err := p.parsePower()
	if err != nil {
		return 0, err
	}
	for {
		p.skipSpace()
		if p.pos >= len(p.s) {
			break
		}
		op := p.s[p.pos]
		if op != '*' && op != '/' && op != '%' {
			break
		}
		p.pos++
		right, err := p.parsePower()
		if err != nil {
			return 0, err
		}
		switch op {
		case '*':
			v *= right
		case '/':
			if right == 0 {
				return 0, errors.New("division by zero")
			}
			v /= right
		case '%':
			if right == 0 {
				return 0, errors.New("division by zero")
			}
			v = math.Mod(v, right)
		}
	}
	return v, nil
}

func (p *parser) parsePower() (float64, error) {
	v, err := p.parseUnary()
	if err != nil {
		return 0, err
	}
	p.skipSpace()
	if p.pos < len(p.s) && p.s[p.pos] == '^' {
		p.pos++
		exp, err := p.parseUnary()
		if err != nil {
			return 0, err
		}
		v = math.Pow(v, exp)
	}
	return v, nil
}

func (p *parser) parseUnary() (float64, error) {
	p.skipSpace()
	if p.pos < len(p.s) && p.s[p.pos] == '-' {
		p.pos++
		v, err := p.parseUnary()
		return -v, err
	}
	return p.parsePrimary()
}

func (p *parser) parsePrimary() (float64, error) {
	p.skipSpace()
	if p.pos >= len(p.s) {
		return 0, errors.New("unexpected end of expression")
	}

	ch := rune(p.s[p.pos])

	// Parenthesised sub-expression
	if ch == '(' {
		p.pos++
		v, err := p.parseExpr()
		if err != nil {
			return 0, err
		}
		p.skipSpace()
		if p.pos >= len(p.s) || p.s[p.pos] != ')' {
			return 0, errors.New("missing closing parenthesis")
		}
		p.pos++
		return v, nil
	}

	// Function call
	if unicode.IsLetter(ch) {
		start := p.pos
		for p.pos < len(p.s) && unicode.IsLetter(rune(p.s[p.pos])) {
			p.pos++
		}
		name := p.s[start:p.pos]
		fn, ok := knownFunctions[name]
		if !ok {
			return 0, fmt.Errorf("unknown function: %q", name)
		}
		p.skipSpace()
		if p.pos >= len(p.s) || p.s[p.pos] != '(' {
			return 0, fmt.Errorf("expected '(' after %q", name)
		}
		p.pos++
		arg, err := p.parseExpr()
		if err != nil {
			return 0, err
		}
		p.skipSpace()
		if p.pos >= len(p.s) || p.s[p.pos] != ')' {
			return 0, errors.New("missing closing parenthesis")
		}
		p.pos++
		return fn(arg), nil
	}

	// Number literal
	if unicode.IsDigit(ch) || ch == '.' {
		start := p.pos
		for p.pos < len(p.s) && (unicode.IsDigit(rune(p.s[p.pos])) || p.s[p.pos] == '.') {
			p.pos++
		}
		v, err := strconv.ParseFloat(p.s[start:p.pos], 64)
		if err != nil {
			return 0, fmt.Errorf("invalid number: %q", p.s[start:p.pos])
		}
		return v, nil
	}

	return 0, fmt.Errorf("unexpected character: %q", string(ch))
}
