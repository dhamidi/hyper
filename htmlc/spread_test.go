package htmlc

import (
	"testing"
)

func TestSpreadAttributes_Empty(t *testing.T) {
	if got := SpreadAttributes(nil); got != "" {
		t.Errorf("SpreadAttributes(nil) = %q, want empty", got)
	}
	if got := SpreadAttributes(map[string]any{}); got != "" {
		t.Errorf("SpreadAttributes({}) = %q, want empty", got)
	}
}

func TestSpreadAttributes_HxAttrs(t *testing.T) {
	attrs := map[string]any{
		"hx-delete":  "/contacts/42",
		"hx-confirm": "Are you sure?",
		"hx-target":  "closest tr",
		"hx-swap":    "outerHTML swap:1s",
	}
	got := SpreadAttributes(attrs)
	// Keys are sorted alphabetically
	want := `hx-confirm="Are you sure?" hx-delete="/contacts/42" hx-swap="outerHTML swap:1s" hx-target="closest tr"`
	if got != want {
		t.Errorf("SpreadAttributes =\n  %q\nwant\n  %q", got, want)
	}
}

func TestSpreadAttributes_BooleanTrue(t *testing.T) {
	attrs := map[string]any{
		"disabled": true,
	}
	got := SpreadAttributes(attrs)
	if got != "disabled" {
		t.Errorf("got %q, want %q", got, "disabled")
	}
}

func TestSpreadAttributes_BooleanFalse(t *testing.T) {
	attrs := map[string]any{
		"disabled": false,
	}
	got := SpreadAttributes(attrs)
	if got != "" {
		t.Errorf("got %q, want empty (false booleans omitted)", got)
	}
}

func TestSpreadAttributes_HTMLEscaping(t *testing.T) {
	attrs := map[string]any{
		"hx-confirm": `Delete "Alice" & Bob?`,
	}
	got := SpreadAttributes(attrs)
	want := `hx-confirm="Delete &#34;Alice&#34; &amp; Bob?"`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSpreadAttributes_NumericValue(t *testing.T) {
	attrs := map[string]any{
		"data-count": 42,
	}
	got := SpreadAttributes(attrs)
	want := `data-count="42"`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
