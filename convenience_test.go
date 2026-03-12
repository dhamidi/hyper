package hyper

import (
	"net/url"
	"testing"
)

func TestMarkdown(t *testing.T) {
	rt := Markdown("# Hello")
	if rt.MediaType != "text/markdown" {
		t.Errorf("MediaType = %q, want %q", rt.MediaType, "text/markdown")
	}
	if rt.Source != "# Hello" {
		t.Errorf("Source = %q, want %q", rt.Source, "# Hello")
	}
}

func TestPlainText(t *testing.T) {
	rt := PlainText("hello")
	if rt.MediaType != "text/plain" {
		t.Errorf("MediaType = %q, want %q", rt.MediaType, "text/plain")
	}
	if rt.Source != "hello" {
		t.Errorf("Source = %q, want %q", rt.Source, "hello")
	}
}

func TestStateFrom(t *testing.T) {
	rt := Markdown("desc")
	obj := StateFrom("name", "Alice", "age", 30, "bio", rt)

	if s, ok := obj["name"].(Scalar); !ok || s.V != "Alice" {
		t.Errorf("name = %v, want Scalar{V: \"Alice\"}", obj["name"])
	}
	if s, ok := obj["age"].(Scalar); !ok || s.V != 30 {
		t.Errorf("age = %v, want Scalar{V: 30}", obj["age"])
	}
	if v, ok := obj["bio"].(RichText); !ok || v.Source != "desc" {
		t.Errorf("bio = %v, want RichText with Source \"desc\"", obj["bio"])
	}
}

func TestStateFromPanicsOnOddLength(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for odd number of arguments")
		}
	}()
	StateFrom("key")
}

func TestStateFromPanicsOnNonStringKey(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for non-string key")
		}
	}()
	StateFrom(42, "value")
}

func TestNewLink(t *testing.T) {
	target := Target{URL: &url.URL{Path: "/items"}}
	link := NewLink("collection", target)
	if link.Rel != "collection" {
		t.Errorf("Rel = %q, want %q", link.Rel, "collection")
	}
	if link.Target.URL.Path != "/items" {
		t.Errorf("Target.URL.Path = %q, want %q", link.Target.URL.Path, "/items")
	}
}

func TestNewAction(t *testing.T) {
	target := Target{URL: &url.URL{Path: "/submit"}}
	action := NewAction("create", "POST", target)
	if action.Name != "create" {
		t.Errorf("Name = %q, want %q", action.Name, "create")
	}
	if action.Method != "POST" {
		t.Errorf("Method = %q, want %q", action.Method, "POST")
	}
	if action.Target.URL.Path != "/submit" {
		t.Errorf("Target.URL.Path = %q, want %q", action.Target.URL.Path, "/submit")
	}
}

func TestNewField(t *testing.T) {
	f := NewField("email", "email")
	if f.Name != "email" {
		t.Errorf("Name = %q, want %q", f.Name, "email")
	}
	if f.Type != "email" {
		t.Errorf("Type = %q, want %q", f.Type, "email")
	}
}

func TestWithValues(t *testing.T) {
	original := []Field{
		NewField("name", "text"),
		NewField("age", "number"),
	}
	populated := WithValues(original, map[string]any{"name": "Alice", "age": 30})

	// Check populated values
	if populated[0].Value != "Alice" {
		t.Errorf("populated[0].Value = %v, want %q", populated[0].Value, "Alice")
	}
	if populated[1].Value != 30 {
		t.Errorf("populated[1].Value = %v, want %d", populated[1].Value, 30)
	}
	// Check original not mutated
	if original[0].Value != nil {
		t.Errorf("original[0].Value = %v, want nil", original[0].Value)
	}
}

func TestWithErrors(t *testing.T) {
	original := []Field{
		NewField("name", "text"),
		NewField("age", "number"),
	}
	populated := WithErrors(original,
		map[string]any{"name": "Alice", "age": -1},
		map[string]string{"age": "must be positive"},
	)

	if populated[0].Value != "Alice" {
		t.Errorf("populated[0].Value = %v, want %q", populated[0].Value, "Alice")
	}
	if populated[0].Error != "" {
		t.Errorf("populated[0].Error = %q, want empty", populated[0].Error)
	}
	if populated[1].Value != -1 {
		t.Errorf("populated[1].Value = %v, want %d", populated[1].Value, -1)
	}
	if populated[1].Error != "must be positive" {
		t.Errorf("populated[1].Error = %q, want %q", populated[1].Error, "must be positive")
	}
	// Check original not mutated
	if original[1].Error != "" {
		t.Errorf("original[1].Error = %q, want empty", original[1].Error)
	}
}
