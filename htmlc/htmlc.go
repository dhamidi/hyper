// Package htmlc is a server-side Go template engine that uses Vue.js
// Single File Component (.vue) syntax. It reads .vue files from a component
// directory and renders them as HTML, supporting text interpolation,
// directives (v-for, v-if, v-bind, v-html), and child component composition.
package htmlc

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Options configures the htmlc engine.
type Options struct {
	ComponentDir string // directory containing .vue component files
}

// Engine is the htmlc template engine.
type Engine struct {
	components map[string]*component
}

type component struct {
	name  string
	nodes []*node
}

// ErrComponentNotFound is returned when a requested component does not exist.
var ErrComponentNotFound = errors.New("component not found")

// IsComponentNotFound reports whether err indicates a missing component.
func IsComponentNotFound(err error) bool {
	return errors.Is(err, ErrComponentNotFound)
}

// New creates a new Engine from the given options.
// It reads and parses all .vue files in ComponentDir.
func New(opts Options) (*Engine, error) {
	e := &Engine{
		components: make(map[string]*component),
	}

	entries, err := os.ReadDir(opts.ComponentDir)
	if err != nil {
		return nil, fmt.Errorf("htmlc: reading component dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".vue") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".vue")
		data, err := os.ReadFile(filepath.Join(opts.ComponentDir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("htmlc: reading %s: %w", entry.Name(), err)
		}

		tmpl, err := extractTemplate(string(data))
		if err != nil {
			return nil, fmt.Errorf("htmlc: parsing %s: %w", entry.Name(), err)
		}

		nodes, err := parseTemplate(tmpl)
		if err != nil {
			return nil, fmt.Errorf("htmlc: parsing template in %s: %w", entry.Name(), err)
		}

		e.components[name] = &component{name: name, nodes: nodes}
	}

	return e, nil
}

// RenderPage renders the named component as a full HTML document
// (with <!DOCTYPE html>, <html>, <head>, <body> wrapper).
// The scope provides template data.
func (e *Engine) RenderPage(w io.Writer, name string, scope map[string]any) error {
	c, ok := e.components[name]
	if !ok {
		return fmt.Errorf("htmlc: %w: %s", ErrComponentNotFound, name)
	}

	var buf strings.Builder
	if err := e.renderNodes(&buf, c.nodes, scope); err != nil {
		return err
	}

	content := buf.String()
	title := extractH1(content)
	if title == "" {
		title = name
	}

	fmt.Fprintf(w, "<!DOCTYPE html>\n<html>\n<head><title>%s</title></head>\n<body>\n", htmlEscape(title))
	io.WriteString(w, content)
	io.WriteString(w, "\n</body>\n</html>")
	return nil
}

// RenderFragment renders the named component as a bare HTML fragment
// (no document wrapper, just the component's <template> content).
func (e *Engine) RenderFragment(w io.Writer, name string, scope map[string]any) error {
	c, ok := e.components[name]
	if !ok {
		return fmt.Errorf("htmlc: %w: %s", ErrComponentNotFound, name)
	}
	return e.renderNodes(w, c.nodes, scope)
}

// extractH1 extracts text content from the first <h1> element in rendered HTML.
func extractH1(html string) string {
	idx := strings.Index(html, "<h1")
	if idx < 0 {
		return ""
	}
	gt := strings.Index(html[idx:], ">")
	if gt < 0 {
		return ""
	}
	start := idx + gt + 1
	end := strings.Index(html[start:], "</h1>")
	if end < 0 {
		return ""
	}
	return stripTags(html[start : start+end])
}

// stripTags removes HTML tags from a string.
func stripTags(s string) string {
	var out strings.Builder
	inTag := false
	for _, c := range s {
		if c == '<' {
			inTag = true
			continue
		}
		if c == '>' {
			inTag = false
			continue
		}
		if !inTag {
			out.WriteRune(c)
		}
	}
	return out.String()
}
