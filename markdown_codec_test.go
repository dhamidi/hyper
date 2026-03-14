package hyper

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func encodeMarkdown(t *testing.T, rep Representation) string {
	t.Helper()
	var buf bytes.Buffer
	codec := MarkdownCodec()
	err := codec.Encode(context.Background(), &buf, rep, EncodeOptions{
		Resolver: mockResolver{},
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
	if !strings.Contains(md, "[Contacts](/contacts)") {
		t.Error("expected markdown link with title")
	}
	if !strings.Contains(md, "[next](/contacts?page=2)") {
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
					{Name: "name", Type: "text", Required: true, Label: "Full Name"},
					{Name: "email", Type: "email", Value: "ada@example.com"},
				},
			},
		},
	}
	md := encodeMarkdown(t, rep)
	if !strings.Contains(md, "## Create Contact") {
		t.Error("expected action name as heading")
	}
	if !strings.Contains(md, "`POST /contacts`") {
		t.Error("expected endpoint description")
	}
	if !strings.Contains(md, "**Full Name**") {
		t.Error("expected field label")
	}
	if !strings.Contains(md, "`text`") {
		t.Error("expected field type")
	}
	if !strings.Contains(md, "required") {
		t.Error("expected required marker")
	}
	if !strings.Contains(md, "default: `ada@example.com`") {
		t.Error("expected default value")
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
	if !strings.Contains(md, "[item](/resolved/items)") {
		t.Error("expected resolved route in markdown link")
	}
}

func TestMarkdownCodec_EmptyRepresentation(t *testing.T) {
	md := encodeMarkdown(t, Representation{})
	if md != "" {
		t.Errorf("expected empty output for empty representation, got %q", md)
	}
}

func TestMarkdownCodec_RichTextState(t *testing.T) {
	rep := Representation{
		State: Object{
			"bio": RichText{MediaType: "text/markdown", Source: "# Hello"},
		},
	}
	md := encodeMarkdown(t, rep)
	if !strings.Contains(md, "**bio:** # Hello") {
		t.Error("expected richtext source inline")
	}
}
