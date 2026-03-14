package htmlc

import (
	"fmt"
	"reflect"
	"strings"
)

// evalExpr evaluates a template expression against a scope.
func evalExpr(expr string, scope map[string]any) any {
	p := &exprParser{input: strings.TrimSpace(expr), scope: scope}
	return p.parseOr()
}

// isTruthy returns whether a value is considered truthy in template conditions.
func isTruthy(val any) bool {
	if val == nil {
		return false
	}
	switch v := val.(type) {
	case bool:
		return v
	case string:
		return v != ""
	case int:
		return v != 0
	case float64:
		return v != 0
	}
	return true
}

// lookupPath resolves a dot-separated path against a scope map.
func lookupPath(path string, scope map[string]any) any {
	parts := strings.Split(path, ".")
	var current any = scope
	for _, part := range parts {
		m, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = m[part]
	}
	return current
}

// toSlice converts a value to []any for iteration.
func toSlice(val any) []any {
	if val == nil {
		return nil
	}
	if s, ok := val.([]any); ok {
		return s
	}
	if s, ok := val.([]map[string]any); ok {
		result := make([]any, len(s))
		for i, v := range s {
			result[i] = v
		}
		return result
	}
	rv := reflect.ValueOf(val)
	if rv.Kind() != reflect.Slice {
		return nil
	}
	result := make([]any, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		result[i] = rv.Index(i).Interface()
	}
	return result
}

type exprParser struct {
	input string
	pos   int
	scope map[string]any
}

func (p *exprParser) parseOr() any {
	left := p.parseAnd()
	for p.match("||") {
		right := p.parseAnd()
		if isTruthy(left) {
			return left
		}
		left = right
	}
	return left
}

func (p *exprParser) parseAnd() any {
	left := p.parseCmp()
	for p.match("&&") {
		right := p.parseCmp()
		if !isTruthy(left) {
			return left
		}
		left = right
	}
	return left
}

func (p *exprParser) parseCmp() any {
	left := p.parseAdd()
	if p.match("!==") {
		right := p.parseAdd()
		return !valEqual(left, right)
	}
	if p.match("===") {
		right := p.parseAdd()
		return valEqual(left, right)
	}
	return left
}

func (p *exprParser) parseAdd() any {
	left := p.parsePrimary()
	for p.match("+") {
		right := p.parsePrimary()
		left = sprintVal(left) + sprintVal(right)
	}
	return left
}

func (p *exprParser) parsePrimary() any {
	p.skipWS()
	if p.pos >= len(p.input) {
		return nil
	}

	c := p.input[p.pos]
	if c == '\'' || c == '"' {
		return p.parseString()
	}
	return p.parsePath()
}

func (p *exprParser) parseString() any {
	quote := p.input[p.pos]
	p.pos++
	start := p.pos
	for p.pos < len(p.input) && p.input[p.pos] != quote {
		p.pos++
	}
	val := p.input[start:p.pos]
	if p.pos < len(p.input) {
		p.pos++
	}
	return val
}

func (p *exprParser) parsePath() any {
	ident := p.readIdent()
	if ident == "" {
		return nil
	}
	path := ident
	for p.pos < len(p.input) && p.input[p.pos] == '.' {
		p.pos++
		next := p.readIdent()
		if next == "" {
			break
		}
		path += "." + next
	}
	return lookupPath(path, p.scope)
}

func (p *exprParser) readIdent() string {
	start := p.pos
	for p.pos < len(p.input) && isIdentChar(p.input[p.pos]) {
		p.pos++
	}
	return p.input[start:p.pos]
}

func (p *exprParser) match(op string) bool {
	p.skipWS()
	if p.pos+len(op) <= len(p.input) && p.input[p.pos:p.pos+len(op)] == op {
		p.pos += len(op)
		return true
	}
	return false
}

func (p *exprParser) skipWS() {
	for p.pos < len(p.input) && (p.input[p.pos] == ' ' || p.input[p.pos] == '\t') {
		p.pos++
	}
}

func isIdentChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') || c == '_' || c == '-'
}

func valEqual(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return fmt.Sprint(a) == fmt.Sprint(b)
}

func sprintVal(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}
