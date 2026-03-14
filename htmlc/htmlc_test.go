package htmlc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupComponents(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestExtractTemplate(t *testing.T) {
	input := `<!-- my component -->
<template>
  <div>Hello</div>
</template>
`
	tmpl, err := extractTemplate(input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tmpl, "<div>Hello</div>") {
		t.Fatalf("unexpected template content: %q", tmpl)
	}
}

func TestTextInterpolation(t *testing.T) {
	dir := setupComponents(t, map[string]string{
		"hello.vue": `<template><h1>{{ greeting }}</h1></template>`,
	})
	engine, err := New(Options{ComponentDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	err = engine.RenderFragment(&buf, "hello", map[string]any{
		"greeting": "Hello, World!",
	})
	if err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "<h1>Hello, World!</h1>") {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestNestedDotPath(t *testing.T) {
	dir := setupComponents(t, map[string]string{
		"nested.vue": `<template><a>{{ actions.edit.href }}</a></template>`,
	})
	engine, err := New(Options{ComponentDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	err = engine.RenderFragment(&buf, "nested", map[string]any{
		"actions": map[string]any{
			"edit": map[string]any{
				"href": "/edit/1",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "/edit/1") {
		t.Fatalf("unexpected output: %q", buf.String())
	}
}

func TestVFor(t *testing.T) {
	dir := setupComponents(t, map[string]string{
		"list.vue": `<template><ul><template v-for="item in items"><li>{{ item.name }}</li></template></ul></template>`,
	})
	engine, err := New(Options{ComponentDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	err = engine.RenderFragment(&buf, "list", map[string]any{
		"items": []map[string]any{
			{"name": "Alice"},
			{"name": "Bob"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "<li>Alice</li>") || !strings.Contains(got, "<li>Bob</li>") {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestVForOnElement(t *testing.T) {
	dir := setupComponents(t, map[string]string{
		"list.vue": `<template><ul><li v-for="item in items">{{ item.name }}</li></ul></template>`,
	})
	engine, err := New(Options{ComponentDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	err = engine.RenderFragment(&buf, "list", map[string]any{
		"items": []map[string]any{
			{"name": "Alice"},
			{"name": "Bob"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "<li>Alice</li>") || !strings.Contains(got, "<li>Bob</li>") {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestVIf(t *testing.T) {
	dir := setupComponents(t, map[string]string{
		"cond.vue": `<template><div><p v-if="show">visible</p><p v-if="hide">hidden</p></div></template>`,
	})
	engine, err := New(Options{ComponentDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	err = engine.RenderFragment(&buf, "cond", map[string]any{
		"show": true,
		"hide": false,
	})
	if err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "visible") {
		t.Fatalf("expected 'visible' in output: %q", got)
	}
	if strings.Contains(got, "hidden") {
		t.Fatalf("unexpected 'hidden' in output: %q", got)
	}
}

func TestVIfElse(t *testing.T) {
	dir := setupComponents(t, map[string]string{
		"cond.vue": `<template><div><a v-if="link">linked</a><span v-else>plain</span></div></template>`,
	})
	engine, err := New(Options{ComponentDir: dir})
	if err != nil {
		t.Fatal(err)
	}

	// v-if true: show <a>, hide <span>
	var buf strings.Builder
	engine.RenderFragment(&buf, "cond", map[string]any{"link": true})
	got := buf.String()
	if !strings.Contains(got, "<a>linked</a>") {
		t.Fatalf("expected linked: %q", got)
	}
	if strings.Contains(got, "plain") {
		t.Fatalf("unexpected plain: %q", got)
	}

	// v-if false: hide <a>, show <span>
	buf.Reset()
	engine.RenderFragment(&buf, "cond", map[string]any{"link": false})
	got = buf.String()
	if strings.Contains(got, "linked") {
		t.Fatalf("unexpected linked: %q", got)
	}
	if !strings.Contains(got, "<span>plain</span>") {
		t.Fatalf("expected plain: %q", got)
	}
}

func TestVIfWithComparison(t *testing.T) {
	dir := setupComponents(t, map[string]string{
		"cond.vue": `<template><div><p v-if="status !== 'hidden'">{{ status }}</p></div></template>`,
	})
	engine, err := New(Options{ComponentDir: dir})
	if err != nil {
		t.Fatal(err)
	}

	var buf strings.Builder
	engine.RenderFragment(&buf, "cond", map[string]any{"status": "visible"})
	if !strings.Contains(buf.String(), "visible") {
		t.Fatalf("expected visible: %q", buf.String())
	}

	buf.Reset()
	engine.RenderFragment(&buf, "cond", map[string]any{"status": "hidden"})
	if strings.Contains(buf.String(), "hidden") {
		t.Fatalf("unexpected hidden: %q", buf.String())
	}
}

func TestVIfAnd(t *testing.T) {
	dir := setupComponents(t, map[string]string{
		"cond.vue": `<template><div><input v-if="field.type !== 'select' && field.type !== 'textarea'" :type="field.type"></div></template>`,
	})
	engine, err := New(Options{ComponentDir: dir})
	if err != nil {
		t.Fatal(err)
	}

	var buf strings.Builder
	engine.RenderFragment(&buf, "cond", map[string]any{
		"field": map[string]any{"type": "text"},
	})
	if !strings.Contains(buf.String(), `type="text"`) {
		t.Fatalf("expected input for text: %q", buf.String())
	}

	buf.Reset()
	engine.RenderFragment(&buf, "cond", map[string]any{
		"field": map[string]any{"type": "select"},
	})
	if strings.Contains(buf.String(), "<input") {
		t.Fatalf("unexpected input for select: %q", buf.String())
	}
}

func TestVIfOr(t *testing.T) {
	dir := setupComponents(t, map[string]string{
		"cond.vue": `<template><nav v-if="prev || next">pagination</nav></template>`,
	})
	engine, err := New(Options{ComponentDir: dir})
	if err != nil {
		t.Fatal(err)
	}

	var buf strings.Builder
	engine.RenderFragment(&buf, "cond", map[string]any{"prev": "/p1"})
	if !strings.Contains(buf.String(), "pagination") {
		t.Fatalf("expected pagination: %q", buf.String())
	}

	buf.Reset()
	engine.RenderFragment(&buf, "cond", map[string]any{})
	if strings.Contains(buf.String(), "pagination") {
		t.Fatalf("unexpected pagination: %q", buf.String())
	}
}

func TestBindAttr(t *testing.T) {
	dir := setupComponents(t, map[string]string{
		"link.vue": `<template><a :href="url" class="link">click</a></template>`,
	})
	engine, err := New(Options{ComponentDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	engine.RenderFragment(&buf, "link", map[string]any{"url": "/page"})
	got := buf.String()
	if !strings.Contains(got, `href="/page"`) {
		t.Fatalf("expected href: %q", got)
	}
	if !strings.Contains(got, `class="link"`) {
		t.Fatalf("expected class: %q", got)
	}
}

func TestBindStringConcat(t *testing.T) {
	dir := setupComponents(t, map[string]string{
		"cls.vue": `<template><span :class="'status ' + status">text</span></template>`,
	})
	engine, err := New(Options{ComponentDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	engine.RenderFragment(&buf, "cls", map[string]any{"status": "active"})
	if !strings.Contains(buf.String(), `class="status active"`) {
		t.Fatalf("expected class: %q", buf.String())
	}
}

func TestVBind(t *testing.T) {
	dir := setupComponents(t, map[string]string{
		"spread.vue": `<template><button v-bind="attrs" class="btn">click</button></template>`,
	})
	engine, err := New(Options{ComponentDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	engine.RenderFragment(&buf, "spread", map[string]any{
		"attrs": map[string]any{
			"hx-post":   "/action",
			"hx-target": "#main",
		},
	})
	got := buf.String()
	if !strings.Contains(got, `hx-post="/action"`) {
		t.Fatalf("expected hx-post: %q", got)
	}
	if !strings.Contains(got, `class="btn"`) {
		t.Fatalf("expected class: %q", got)
	}
}

func TestVHtml(t *testing.T) {
	dir := setupComponents(t, map[string]string{
		"raw.vue": `<template><div v-html="content"></div></template>`,
	})
	engine, err := New(Options{ComponentDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	engine.RenderFragment(&buf, "raw", map[string]any{
		"content": "<strong>bold</strong>",
	})
	got := buf.String()
	if !strings.Contains(got, "<strong>bold</strong>") {
		t.Fatalf("expected raw HTML: %q", got)
	}
}

func TestBooleanAttr(t *testing.T) {
	dir := setupComponents(t, map[string]string{
		"form-input.vue": `<template><input :type="typ" :name="name" :required="req" :disabled="dis"></template>`,
	})
	engine, err := New(Options{ComponentDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	engine.RenderFragment(&buf, "form-input", map[string]any{
		"typ":  "text",
		"name": "title",
		"req":  true,
		"dis":  false,
	})
	got := buf.String()
	if !strings.Contains(got, " required") {
		t.Fatalf("expected required: %q", got)
	}
	if strings.Contains(got, "disabled") {
		t.Fatalf("unexpected disabled: %q", got)
	}
}

func TestChildComponent(t *testing.T) {
	dir := setupComponents(t, map[string]string{
		"parent.vue": `<template><div><child v-bind="data"></child></div></template>`,
		"child.vue":  `<template><span>{{ name }}</span></template>`,
	})
	engine, err := New(Options{ComponentDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	engine.RenderFragment(&buf, "parent", map[string]any{
		"data": map[string]any{"name": "Alice"},
	})
	got := buf.String()
	if !strings.Contains(got, "<span>Alice</span>") {
		t.Fatalf("expected child output: %q", got)
	}
}

func TestChildComponentInVFor(t *testing.T) {
	dir := setupComponents(t, map[string]string{
		"parent.vue": `<template><ul><template v-for="item in items"><row v-bind="item"></row></template></ul></template>`,
		"row.vue":    `<template><li>{{ title }}</li></template>`,
	})
	engine, err := New(Options{ComponentDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	engine.RenderFragment(&buf, "parent", map[string]any{
		"items": []map[string]any{
			{"title": "First"},
			{"title": "Second"},
		},
	})
	got := buf.String()
	if !strings.Contains(got, "<li>First</li>") || !strings.Contains(got, "<li>Second</li>") {
		t.Fatalf("expected rows: %q", got)
	}
}

func TestRenderPage(t *testing.T) {
	dir := setupComponents(t, map[string]string{
		"page.vue": `<template><div><h1>My Page</h1><p>content</p></div></template>`,
	})
	engine, err := New(Options{ComponentDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	engine.RenderPage(&buf, "page", map[string]any{})
	got := buf.String()
	if !strings.HasPrefix(got, "<!DOCTYPE html>") {
		t.Fatalf("expected doctype: %q", got)
	}
	if !strings.Contains(got, "<title>My Page</title>") {
		t.Fatalf("expected title: %q", got)
	}
	if !strings.Contains(got, "<body>") || !strings.Contains(got, "</body>") {
		t.Fatalf("expected body: %q", got)
	}
	if !strings.Contains(got, "<h1>My Page</h1>") {
		t.Fatalf("expected content: %q", got)
	}
}

func TestRenderFragment(t *testing.T) {
	dir := setupComponents(t, map[string]string{
		"frag.vue": `<template><div>fragment</div></template>`,
	})
	engine, err := New(Options{ComponentDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	engine.RenderFragment(&buf, "frag", map[string]any{})
	got := buf.String()
	if strings.Contains(got, "<!DOCTYPE") {
		t.Fatalf("fragment should not have doctype: %q", got)
	}
	if got != "<div>fragment</div>" {
		t.Fatalf("unexpected fragment: %q", got)
	}
}

func TestComponentNotFound(t *testing.T) {
	dir := setupComponents(t, map[string]string{
		"exists.vue": `<template><div>ok</div></template>`,
	})
	engine, err := New(Options{ComponentDir: dir})
	if err != nil {
		t.Fatal(err)
	}

	var buf strings.Builder
	err = engine.RenderFragment(&buf, "missing", map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing component")
	}
	if !IsComponentNotFound(err) {
		t.Fatalf("expected component not found error, got: %v", err)
	}
}

func TestHTMLEscaping(t *testing.T) {
	dir := setupComponents(t, map[string]string{
		"esc.vue": `<template><p>{{ content }}</p></template>`,
	})
	engine, err := New(Options{ComponentDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	engine.RenderFragment(&buf, "esc", map[string]any{
		"content": "<script>alert('xss')</script>",
	})
	got := buf.String()
	if strings.Contains(got, "<script>") {
		t.Fatalf("expected escaped output: %q", got)
	}
	if !strings.Contains(got, "&lt;script&gt;") {
		t.Fatalf("expected HTML entities: %q", got)
	}
}

func TestNilValueOmitsAttr(t *testing.T) {
	dir := setupComponents(t, map[string]string{
		"nilattr.vue": `<template><input :type="typ" :value="val"></template>`,
	})
	engine, err := New(Options{ComponentDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	engine.RenderFragment(&buf, "nilattr", map[string]any{
		"typ": "text",
	})
	got := buf.String()
	if !strings.Contains(got, `type="text"`) {
		t.Fatalf("expected type: %q", got)
	}
	if strings.Contains(got, "value") {
		t.Fatalf("unexpected value attr: %q", got)
	}
}

func TestSelectedAttr(t *testing.T) {
	dir := setupComponents(t, map[string]string{
		"opt.vue": `<template><select><option v-for="o in options" :value="o.value" :selected="o.selected">{{ o.label }}</option></select></template>`,
	})
	engine, err := New(Options{ComponentDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	engine.RenderFragment(&buf, "opt", map[string]any{
		"options": []map[string]any{
			{"value": "a", "label": "A", "selected": true},
			{"value": "b", "label": "B", "selected": false},
		},
	})
	got := buf.String()
	if !strings.Contains(got, `value="a" selected`) {
		t.Fatalf("expected selected on A: %q", got)
	}
	if strings.Contains(got, `value="b" selected`) {
		t.Fatalf("unexpected selected on B: %q", got)
	}
}

func TestStaticAttrsOverrideVBind(t *testing.T) {
	dir := setupComponents(t, map[string]string{
		"override.vue": `<template><button v-bind="spread" hx-target="closest tr">click</button></template>`,
	})
	engine, err := New(Options{ComponentDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	engine.RenderFragment(&buf, "override", map[string]any{
		"spread": map[string]any{
			"hx-post":   "/action",
			"hx-target": "#main",
		},
	})
	got := buf.String()
	if !strings.Contains(got, `hx-target="closest tr"`) {
		t.Fatalf("expected static override: %q", got)
	}
	if !strings.Contains(got, `hx-post="/action"`) {
		t.Fatalf("expected spread attr: %q", got)
	}
}

func TestHyphenatedKeys(t *testing.T) {
	dir := setupComponents(t, map[string]string{
		"hyph.vue": `<template><span>{{ actions.create-task.name }}</span></template>`,
	})
	engine, err := New(Options{ComponentDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	engine.RenderFragment(&buf, "hyph", map[string]any{
		"actions": map[string]any{
			"create-task": map[string]any{
				"name": "Quick Add",
			},
		},
	})
	if !strings.Contains(buf.String(), "Quick Add") {
		t.Fatalf("expected hyphenated key: %q", buf.String())
	}
}

func TestComment(t *testing.T) {
	dir := setupComponents(t, map[string]string{
		"cmt.vue": `<template><!-- comment --><div>ok</div></template>`,
	})
	engine, err := New(Options{ComponentDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	engine.RenderFragment(&buf, "cmt", map[string]any{})
	got := buf.String()
	if strings.Contains(got, "comment") {
		t.Fatalf("comments should be stripped: %q", got)
	}
	if !strings.Contains(got, "<div>ok</div>") {
		t.Fatalf("expected content: %q", got)
	}
}
