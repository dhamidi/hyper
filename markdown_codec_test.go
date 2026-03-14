package hyper

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func encodeMarkdown(t *testing.T, rep Representation, mode ...RenderMode) string {
	t.Helper()
	var buf bytes.Buffer
	codec := MarkdownCodec()
	m := RenderDocument
	if len(mode) > 0 {
		m = mode[0]
	}
	err := codec.Encode(context.Background(), &buf, rep, EncodeOptions{
		Resolver: mockResolver{},
		Mode:     m,
	})
	if err != nil {
		t.Fatalf("Markdown Encode failed: %v", err)
	}
	return buf.String()
}

func TestMarkdownCodec_MediaTypes(t *testing.T) {
	codec := MarkdownCodec()
	types := codec.MediaTypes()
	if len(types) != 1 || types[0] != "text/markdown" {
		t.Errorf("expected [text/markdown], got %v", types)
	}
}

func TestMarkdownCodec_Kind(t *testing.T) {
	md := encodeMarkdown(t, Representation{Kind: "contact"})
	if !strings.Contains(md, "# contact") {
		t.Error("expected h1 with kind")
	}
}

func TestMarkdownCodec_ObjectState(t *testing.T) {
	rep := Representation{
		State: Object{
			"name":  Scalar{V: "Ada"},
			"email": Scalar{V: "ada@example.com"},
		},
	}
	md := encodeMarkdown(t, rep)
	if !strings.Contains(md, "**name:** Ada") {
		t.Error("expected bold key with value for name")
	}
	if !strings.Contains(md, "**email:** ada@example.com") {
		t.Error("expected bold key with value for email")
	}
}

func TestMarkdownCodec_CollectionState(t *testing.T) {
	rep := Representation{
		State: Collection{Scalar{V: "a"}, Scalar{V: "b"}},
	}
	md := encodeMarkdown(t, rep)
	if !strings.Contains(md, "1. a") {
		t.Error("expected numbered list item for 'a'")
	}
	if !strings.Contains(md, "2. b") {
		t.Error("expected numbered list item for 'b'")
	}
}

func TestMarkdownCodec_Links(t *testing.T) {
	rep := Representation{
		Links: []Link{
			{Rel: "self", Target: Target{URL: mustParseURL("/contacts")}, Title: "Contacts"},
			{Rel: "next", Target: Target{URL: mustParseURL("/contacts?page=2")}},
		},
	}
	md := encodeMarkdown(t, rep)
	if !strings.Contains(md, "## Links") {
		t.Error("expected Links heading")
	}
	if !strings.Contains(md, "[Contacts](/contacts) (rel: self)") {
		t.Error("expected markdown link with title and rel")
	}
	if !strings.Contains(md, "[next](/contacts?page=2) (rel: next)") {
		t.Error("expected markdown link with rel as fallback label")
	}
}

func TestMarkdownCodec_Actions(t *testing.T) {
	rep := Representation{
		Actions: []Action{
			{
				Name:   "Create Contact",
				Method: "POST",
				Target: Target{URL: mustParseURL("/contacts")},
				Fields: []Field{
					{Name: "name", Type: "text", Required: true},
					{Name: "email", Type: "email", Value: "ada@example.com"},
				},
			},
		},
	}
	md := encodeMarkdown(t, rep)
	if !strings.Contains(md, "### Create Contact (POST /contacts)") {
		t.Error("expected action heading with method and target")
	}
	if !strings.Contains(md, "- name (text, required)") {
		t.Error("expected field with type and required")
	}
	if !strings.Contains(md, `- email (email): "ada@example.com"`) {
		t.Error("expected field with default value")
	}
}

func TestMarkdownCodec_SelectField(t *testing.T) {
	rep := Representation{
		Actions: []Action{
			{
				Name:   "choose",
				Method: "POST",
				Target: Target{URL: mustParseURL("/choose")},
				Fields: []Field{
					{
						Name: "color",
						Type: "select",
						Options: []Option{
							{Value: "red", Label: "Red"},
							{Value: "blue", Label: "Blue", Selected: true},
						},
					},
				},
			},
		},
	}
	md := encodeMarkdown(t, rep)
	if !strings.Contains(md, "[ ] Red") {
		t.Error("expected unselected option")
	}
	if !strings.Contains(md, "[x] Blue") {
		t.Error("expected selected option")
	}
}

func TestMarkdownCodec_FieldHelp(t *testing.T) {
	rep := Representation{
		Actions: []Action{
			{
				Name:   "create",
				Method: "POST",
				Target: Target{URL: mustParseURL("/create")},
				Fields: []Field{
					{Name: "name", Type: "text", Help: "Enter your full name"},
				},
			},
		},
	}
	md := encodeMarkdown(t, rep)
	if !strings.Contains(md, "*Enter your full name*") {
		t.Error("expected help text in italics")
	}
}

func TestMarkdownCodec_Embedded(t *testing.T) {
	rep := Representation{
		Kind: "list",
		Embedded: map[string][]Representation{
			"items": {
				{Kind: "item", State: Object{"name": Scalar{V: "First"}}},
				{Kind: "item", State: Object{"name": Scalar{V: "Second"}}},
			},
		},
	}
	md := encodeMarkdown(t, rep)
	if !strings.Contains(md, "## items") {
		t.Error("expected embedded slot heading")
	}
	if !strings.Contains(md, "### item") {
		t.Error("expected embedded kind as h3")
	}
	if !strings.Contains(md, "First") {
		t.Error("expected first embedded item state")
	}
	if !strings.Contains(md, "Second") {
		t.Error("expected second embedded item state")
	}
}

func TestMarkdownCodec_RouteTarget(t *testing.T) {
	rep := Representation{
		Links: []Link{
			{Rel: "item", Target: Target{Route: &RouteRef{Name: "items"}}},
		},
	}
	md := encodeMarkdown(t, rep)
	if !strings.Contains(md, "[item](/resolved/items) (rel: item)") {
		t.Error("expected resolved route in markdown link with rel")
	}
}

func TestMarkdownCodec_EmptyRepresentation(t *testing.T) {
	md := encodeMarkdown(t, Representation{})
	if md != "" {
		t.Errorf("expected empty output for empty representation, got %q", md)
	}
}

func TestMarkdownCodec_RichTextMarkdownPassthrough(t *testing.T) {
	rep := Representation{
		State: Object{
			"bio": RichText{MediaType: "text/markdown", Source: "# Hello"},
		},
	}
	md := encodeMarkdown(t, rep)
	if !strings.Contains(md, "**bio:** # Hello") {
		t.Error("expected markdown richtext source passed through inline")
	}
}

func TestMarkdownCodec_RichTextOtherFencedBlock(t *testing.T) {
	rep := Representation{
		State: Object{
			"code": RichText{MediaType: "text/html", Source: "<b>bold</b>"},
		},
	}
	md := encodeMarkdown(t, rep)
	if !strings.Contains(md, "```text/html") {
		t.Error("expected fenced code block with media type hint")
	}
	if !strings.Contains(md, "<b>bold</b>") {
		t.Error("expected source in fenced code block")
	}
}

func TestMarkdownCodec_RenderFragmentOmitsHeading(t *testing.T) {
	rep := Representation{
		Kind:  "contact",
		State: Object{"name": Scalar{V: "Ada"}},
	}
	md := encodeMarkdown(t, rep, RenderFragment)
	if strings.Contains(md, "# contact") {
		t.Error("expected Kind heading omitted in RenderFragment mode")
	}
	if !strings.Contains(md, "**name:** Ada") {
		t.Error("expected state still rendered in RenderFragment mode")
	}
}

func TestMarkdownCodec_Meta(t *testing.T) {
	rep := Representation{
		Meta: map[string]any{
			"version": "1.0",
			"author":  "Ada",
		},
	}
	md := encodeMarkdown(t, rep)
	if !strings.Contains(md, "## Meta") {
		t.Error("expected Meta heading")
	}
	if !strings.Contains(md, "**author:** Ada") {
		t.Error("expected author meta")
	}
	if !strings.Contains(md, "**version:** 1.0") {
		t.Error("expected version meta")
	}
}
