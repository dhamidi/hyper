package htmlc

import (
	"fmt"
	"io"
	"strings"
)

func (e *Engine) renderNodes(w io.Writer, nodes []*node, scope map[string]any) error {
	lastIfResult := true
	for _, n := range nodes {
		if n.typ == textNode {
			io.WriteString(w, interpolate(n.text, scope))
			continue
		}

		// v-for: iterate (highest priority)
		if vfor := getAttr(n.attrs, "v-for"); vfor != "" {
			if err := e.renderVFor(w, n, vfor, scope); err != nil {
				return err
			}
			continue
		}

		// v-else: render only if previous v-if was false
		if hasAttr(n.attrs, "v-else") {
			if lastIfResult {
				continue
			}
			stripped := withoutAttrs(n, "v-else")
			if err := e.renderElement(w, stripped, scope); err != nil {
				return err
			}
			continue
		}

		// v-if: conditional rendering
		if vif := getAttr(n.attrs, "v-if"); vif != "" {
			result := isTruthy(evalExpr(vif, scope))
			lastIfResult = result
			if !result {
				continue
			}
			stripped := withoutAttrs(n, "v-if")
			if err := e.renderElement(w, stripped, scope); err != nil {
				return err
			}
			continue
		}

		// Regular element
		if err := e.renderElement(w, n, scope); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) renderElement(w io.Writer, n *node, scope map[string]any) error {
	// Component check: only for non-HTML tags
	if !isHTMLElement(n.tag) {
		if comp, ok := e.components[n.tag]; ok {
			return e.renderComponent(w, comp, n, scope)
		}
	}

	// <template> is a transparent wrapper
	if n.tag == "template" {
		return e.renderNodes(w, n.children, scope)
	}

	return e.renderHTMLElement(w, n, scope)
}

func (e *Engine) renderVFor(w io.Writer, n *node, vfor string, scope map[string]any) error {
	parts := strings.SplitN(vfor, " in ", 2)
	if len(parts) != 2 {
		return fmt.Errorf("htmlc: invalid v-for: %s", vfor)
	}
	varName := strings.TrimSpace(parts[0])
	listExpr := strings.TrimSpace(parts[1])

	list := evalExpr(listExpr, scope)
	items := toSlice(list)

	stripped := withoutAttrs(n, "v-for")
	for _, item := range items {
		childScope := mergeScope(scope, map[string]any{varName: item})

		// Handle v-if on the same element
		if vif := getAttr(stripped.attrs, "v-if"); vif != "" {
			if !isTruthy(evalExpr(vif, childScope)) {
				continue
			}
			inner := withoutAttrs(stripped, "v-if")
			if err := e.renderElement(w, inner, childScope); err != nil {
				return err
			}
			continue
		}

		if err := e.renderElement(w, stripped, childScope); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) renderComponent(w io.Writer, comp *component, n *node, scope map[string]any) error {
	childScope := make(map[string]any)

	// v-bind spread
	if vbind := getAttr(n.attrs, "v-bind"); vbind != "" {
		val := evalExpr(vbind, scope)
		if m, ok := val.(map[string]any); ok {
			for k, v := range m {
				childScope[k] = v
			}
		}
	}

	// Individual :attr bindings
	for _, a := range n.attrs {
		if strings.HasPrefix(a.name, ":") && a.name != ":is" {
			key := a.name[1:]
			childScope[key] = evalExpr(a.value, scope)
		}
	}

	return e.renderNodes(w, comp.nodes, childScope)
}

func (e *Engine) renderHTMLElement(w io.Writer, n *node, scope map[string]any) error {
	// Collect attributes: v-bind spread first, then static/:dynamic attrs override
	type kv struct {
		key, value string
	}
	seen := make(map[string]int) // key → index in attrs
	var finalAttrs []kv

	addAttr := func(key, value string) {
		if idx, ok := seen[key]; ok {
			finalAttrs[idx] = kv{key, value}
		} else {
			seen[key] = len(finalAttrs)
			finalAttrs = append(finalAttrs, kv{key, value})
		}
	}
	removeAttr := func(key string) {
		if idx, ok := seen[key]; ok {
			finalAttrs[idx].key = "" // mark as removed
			delete(seen, key)
		}
	}

	// v-bind spread
	if vbind := getAttr(n.attrs, "v-bind"); vbind != "" {
		val := evalExpr(vbind, scope)
		if m, ok := val.(map[string]any); ok {
			for k, v := range m {
				addAttr(k, sprintVal(v))
			}
		}
	}

	// Process other attributes
	for _, a := range n.attrs {
		if isDirective(a.name) {
			continue
		}

		if strings.HasPrefix(a.name, ":") {
			key := a.name[1:]
			val := evalExpr(a.value, scope)
			if isBooleanAttr(key) {
				if isTruthy(val) {
					addAttr(key, key)
				} else {
					removeAttr(key)
				}
			} else {
				if val == nil {
					removeAttr(key)
				} else {
					addAttr(key, sprintVal(val))
				}
			}
		} else {
			// Static attribute
			addAttr(a.name, a.value)
		}
	}

	// Write opening tag
	fmt.Fprintf(w, "<%s", n.tag)
	for _, a := range finalAttrs {
		if a.key == "" {
			continue
		}
		if isBooleanAttr(a.key) && a.value == a.key {
			fmt.Fprintf(w, " %s", a.key)
		} else {
			fmt.Fprintf(w, ` %s="%s"`, a.key, htmlAttrEscape(a.value))
		}
	}
	io.WriteString(w, ">")

	if isVoidElement(n.tag) {
		return nil
	}

	// v-html: insert raw HTML
	if vhtml := getAttr(n.attrs, "v-html"); vhtml != "" {
		val := evalExpr(vhtml, scope)
		if val != nil {
			io.WriteString(w, sprintVal(val))
		}
	} else {
		if err := e.renderNodes(w, n.children, scope); err != nil {
			return err
		}
	}

	fmt.Fprintf(w, "</%s>", n.tag)
	return nil
}

// interpolate replaces {{ expr }} in text with evaluated values.
func interpolate(text string, scope map[string]any) string {
	if !strings.Contains(text, "{{") {
		return text
	}
	var out strings.Builder
	for {
		idx := strings.Index(text, "{{")
		if idx < 0 {
			out.WriteString(text)
			break
		}
		out.WriteString(text[:idx])
		text = text[idx+2:]

		end := strings.Index(text, "}}")
		if end < 0 {
			out.WriteString("{{")
			out.WriteString(text)
			break
		}

		expr := strings.TrimSpace(text[:end])
		val := evalExpr(expr, scope)
		if val != nil {
			out.WriteString(htmlEscape(sprintVal(val)))
		}
		text = text[end+2:]
	}
	return out.String()
}

func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func htmlAttrEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

func mergeScope(parent, additions map[string]any) map[string]any {
	scope := make(map[string]any, len(parent)+len(additions))
	for k, v := range parent {
		scope[k] = v
	}
	for k, v := range additions {
		scope[k] = v
	}
	return scope
}

func getAttr(attrs []attr, name string) string {
	for _, a := range attrs {
		if a.name == name {
			return a.value
		}
	}
	return ""
}

func hasAttr(attrs []attr, name string) bool {
	for _, a := range attrs {
		if a.name == name {
			return true
		}
	}
	return false
}

func withoutAttrs(n *node, names ...string) *node {
	skip := make(map[string]bool, len(names))
	for _, name := range names {
		skip[name] = true
	}
	cp := *n
	cp.attrs = nil
	for _, a := range n.attrs {
		if !skip[a.name] {
			cp.attrs = append(cp.attrs, a)
		}
	}
	return &cp
}

func isDirective(name string) bool {
	return name == "v-for" || name == "v-if" || name == "v-else" ||
		name == "v-bind" || name == "v-html"
}

func isBooleanAttr(name string) bool {
	switch name {
	case "checked", "disabled", "selected", "required", "readonly",
		"multiple", "hidden", "open", "autofocus", "autoplay",
		"controls", "defer", "novalidate":
		return true
	}
	return false
}
