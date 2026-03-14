package hyper

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func encodeHTML(t *testing.T, rep Representation, mode RenderMode) string {
	t.Helper()
	var buf bytes.Buffer
	codec := HTMLCodec()
	err := codec.Encode(context.Background(), &buf, rep, EncodeOptions{
		Resolver: mockResolver{},
		Mode:     mode,
	})
	if err != nil {
		t.Fatalf("HTML Encode failed: %v", err)
	}
	return buf.String()
}

func TestHTMLCodec_MediaTypes(t *testing.T) {
	codec := HTMLCodec()
	types := codec.MediaTypes()
	if len(types) != 1 || types[0] != "text/html" {
		t.Errorf("expected [text/html], got %v", types)
	}
}

func TestHTMLCodec_DocumentWrapper(t *testing.T) {
	html := encodeHTML(t, Representation{Kind: "contact"}, RenderDocument)
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("expected DOCTYPE in document mode")
	}
	if !strings.Contains(html, "<title>contact</title>") {
		t.Error("expected title from Kind")
	}
	if !strings.Contains(html, "<html>") {
		t.Error("expected <html> tag")
	}
	if !strings.Contains(html, "</html>") {
		t.Error("expected closing </html> tag")
	}
}

func TestHTMLCodec_FragmentMode(t *testing.T) {
	html := encodeHTML(t, Representation{Kind: "contact"}, RenderFragment)
	if strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("fragment mode should not include DOCTYPE")
	}
	if !strings.Contains(html, "<article") {
		t.Error("expected <article> in fragment")
	}
}

func TestHTMLCodec_Kind(t *testing.T) {
	html := encodeHTML(t, Representation{Kind: "contact"}, RenderFragment)
	if !strings.Contains(html, `data-kind="contact"`) {
		t.Error("expected data-kind attribute")
	}
	if !strings.Contains(html, "<h1>contact</h1>") {
		t.Error("expected h1 with kind")
	}
}

func TestHTMLCodec_ObjectState(t *testing.T) {
	rep := Representation{
		State: Object{
			"name":  Scalar{V: "Ada"},
			"email": Scalar{V: "ada@example.com"},
		},
	}
	html := encodeHTML(t, rep, RenderFragment)
	if !strings.Contains(html, "<dl>") {
		t.Error("expected definition list for object state")
	}
	if !strings.Contains(html, "<dt>name</dt>") {
		t.Error("expected dt for name")
	}
	if !strings.Contains(html, "<dd>Ada</dd>") {
		t.Error("expected dd for name value")
	}
}

func TestHTMLCodec_CollectionState(t *testing.T) {
	rep := Representation{
		State: Collection{Scalar{V: "a"}, Scalar{V: "b"}},
	}
	html := encodeHTML(t, rep, RenderFragment)
	if !strings.Contains(html, "<ol>") {
		t.Error("expected ordered list for collection state")
	}
	if !strings.Contains(html, "<li>a</li>") {
		t.Error("expected list item for 'a'")
	}
	if !strings.Contains(html, "<li>b</li>") {
		t.Error("expected list item for 'b'")
	}
}

func TestHTMLCodec_Links(t *testing.T) {
	rep := Representation{
		Links: []Link{
			{Rel: "self", Target: Target{URL: mustParseURL("/contacts")}, Title: "Contacts"},
			{Rel: "next", Target: Target{URL: mustParseURL("/contacts?page=2")}},
		},
	}
	html := encodeHTML(t, rep, RenderFragment)
	if !strings.Contains(html, "<nav>") {
		t.Error("expected nav element")
	}
	if !strings.Contains(html, `href="/contacts"`) {
		t.Error("expected href for self link")
	}
	if !strings.Contains(html, `rel="self"`) {
		t.Error("expected rel attribute")
	}
	if !strings.Contains(html, ">Contacts</a>") {
		t.Error("expected title as link text")
	}
	// Link without title should use rel as label
	if !strings.Contains(html, ">next</a>") {
		t.Error("expected rel as fallback link text")
	}
}

func TestHTMLCodec_Actions(t *testing.T) {
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
	html := encodeHTML(t, rep, RenderFragment)
	if !strings.Contains(html, `<form method="POST" action="/contacts">`) {
		t.Error("expected form element with method and action")
	}
	if !strings.Contains(html, "<h2>Create Contact</h2>") {
		t.Error("expected action name as heading")
	}
	if !strings.Contains(html, `name="name"`) {
		t.Error("expected input with name")
	}
	if !strings.Contains(html, "required") {
		t.Error("expected required attribute")
	}
	if !strings.Contains(html, `<label>Full Name`) {
		t.Error("expected label")
	}
	if !strings.Contains(html, `value="ada@example.com"`) {
		t.Error("expected value attribute")
	}
	if !strings.Contains(html, `<button type="submit">`) {
		t.Error("expected submit button")
	}
}

func TestHTMLCodec_MethodOverride_PUT(t *testing.T) {
	rep := Representation{
		Actions: []Action{
			{
				Name:   "Update Contact",
				Method: "PUT",
				Target: Target{URL: mustParseURL("/contacts/42")},
				Fields: []Field{
					{Name: "name", Type: "text"},
				},
			},
		},
	}
	html := encodeHTML(t, rep, RenderFragment)
	if !strings.Contains(html, `<form method="POST" action="/contacts/42">`) {
		t.Error("expected PUT action to render as POST form")
	}
	if !strings.Contains(html, `<input type="hidden" name="_method" value="PUT">`) {
		t.Error("expected hidden _method field with value PUT")
	}
}

func TestHTMLCodec_MethodOverride_DELETE(t *testing.T) {
	rep := Representation{
		Actions: []Action{
			{
				Name:   "Delete Contact",
				Method: "DELETE",
				Target: Target{URL: mustParseURL("/contacts/42")},
			},
		},
	}
	html := encodeHTML(t, rep, RenderFragment)
	if !strings.Contains(html, `<form method="POST" action="/contacts/42">`) {
		t.Error("expected DELETE action to render as POST form")
	}
	if !strings.Contains(html, `<input type="hidden" name="_method" value="DELETE">`) {
		t.Error("expected hidden _method field with value DELETE")
	}
}

func TestHTMLCodec_MethodUnchanged_GET(t *testing.T) {
	rep := Representation{
		Actions: []Action{
			{
				Name:   "Search",
				Method: "GET",
				Target: Target{URL: mustParseURL("/search")},
				Fields: []Field{
					{Name: "q", Type: "text"},
				},
			},
		},
	}
	html := encodeHTML(t, rep, RenderFragment)
	if !strings.Contains(html, `<form method="GET" action="/search">`) {
		t.Error("expected GET action to remain as GET form")
	}
	if strings.Contains(html, `name="_method"`) {
		t.Error("GET action should not have _method hidden field")
	}
}

func TestHTMLCodec_MethodUnchanged_POST(t *testing.T) {
	rep := Representation{
		Actions: []Action{
			{
				Name:   "Create",
				Method: "POST",
				Target: Target{URL: mustParseURL("/contacts")},
				Fields: []Field{
					{Name: "name", Type: "text"},
				},
			},
		},
	}
	html := encodeHTML(t, rep, RenderFragment)
	if !strings.Contains(html, `<form method="POST" action="/contacts">`) {
		t.Error("expected POST action to remain as POST form")
	}
	if strings.Contains(html, `name="_method"`) {
		t.Error("POST action should not have _method hidden field")
	}
}

func TestHTMLCodec_SelectField(t *testing.T) {
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
	html := encodeHTML(t, rep, RenderFragment)
	if !strings.Contains(html, "<select") {
		t.Error("expected select element")
	}
	if !strings.Contains(html, `<option value="red">Red</option>`) {
		t.Error("expected red option")
	}
	if !strings.Contains(html, `<option value="blue" selected>Blue</option>`) {
		t.Error("expected selected blue option")
	}
}

func TestHTMLCodec_HiddenField(t *testing.T) {
	rep := Representation{
		Actions: []Action{
			{
				Name:   "update",
				Method: "POST",
				Target: Target{URL: mustParseURL("/update")},
				Fields: []Field{
					{Name: "id", Type: "hidden", Value: "42"},
				},
			},
		},
	}
	html := encodeHTML(t, rep, RenderFragment)
	if !strings.Contains(html, `type="hidden"`) {
		t.Error("expected hidden input")
	}
	if !strings.Contains(html, `value="42"`) {
		t.Error("expected value for hidden field")
	}
	// Hidden fields should not have labels
	if strings.Contains(html, "<label>") {
		t.Error("hidden fields should not have labels")
	}
}

func TestHTMLCodec_FieldHelp(t *testing.T) {
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
	html := encodeHTML(t, rep, RenderFragment)
	if !strings.Contains(html, "<small>Enter your full name</small>") {
		t.Error("expected help text in <small>")
	}
}

func TestHTMLCodec_FieldError(t *testing.T) {
	rep := Representation{
		Actions: []Action{
			{
				Name:   "create",
				Method: "POST",
				Target: Target{URL: mustParseURL("/create")},
				Fields: []Field{
					{Name: "email", Type: "email", Error: "invalid email"},
				},
			},
		},
	}
	html := encodeHTML(t, rep, RenderFragment)
	if !strings.Contains(html, "<em>invalid email</em>") {
		t.Error("expected error message in <em>")
	}
}

func TestHTMLCodec_Embedded(t *testing.T) {
	rep := Representation{
		Kind: "list",
		Embedded: map[string][]Representation{
			"items": {
				{Kind: "item", State: Object{"name": Scalar{V: "First"}}},
				{Kind: "item", State: Object{"name": Scalar{V: "Second"}}},
			},
		},
	}
	html := encodeHTML(t, rep, RenderFragment)
	if !strings.Contains(html, `data-slot="items"`) {
		t.Error("expected data-slot attribute on section")
	}
	if !strings.Contains(html, "<dd>First</dd>") {
		t.Error("expected first embedded item state")
	}
	if !strings.Contains(html, "<dd>Second</dd>") {
		t.Error("expected second embedded item state")
	}
}

func TestHTMLCodec_HTMLEscaping(t *testing.T) {
	rep := Representation{
		Kind: "<script>alert('xss')</script>",
		State: Object{
			"data": Scalar{V: `<img onerror="alert(1)">`},
		},
	}
	html := encodeHTML(t, rep, RenderFragment)
	if strings.Contains(html, "<script>") {
		t.Error("kind should be HTML-escaped")
	}
	if strings.Contains(html, `<img onerror`) {
		t.Error("state values should be HTML-escaped")
	}
}

func TestHTMLCodec_RichTextState(t *testing.T) {
	rep := Representation{
		State: Object{
			"bio": RichText{MediaType: "text/markdown", Source: "# Hello"},
		},
	}
	html := encodeHTML(t, rep, RenderFragment)
	if !strings.Contains(html, "<pre># Hello</pre>") {
		t.Error("expected richtext source in pre tag")
	}
}

func TestHTMLCodec_FileField(t *testing.T) {
	rep := Representation{
		Actions: []Action{
			{
				Name:   "upload",
				Method: "POST",
				Target: Target{URL: mustParseURL("/upload")},
				Fields: []Field{
					{Name: "file", Type: "file", Accept: "image/*", Multiple: true},
				},
			},
		},
	}
	html := encodeHTML(t, rep, RenderFragment)
	if !strings.Contains(html, `type="file"`) {
		t.Error("expected file input type")
	}
	if !strings.Contains(html, `accept="image/*"`) {
		t.Error("expected accept attribute")
	}
	if !strings.Contains(html, "multiple") {
		t.Error("expected multiple attribute")
	}
}

func TestHTMLCodec_ReadOnlyField(t *testing.T) {
	rep := Representation{
		Actions: []Action{
			{
				Name:   "view",
				Method: "POST",
				Target: Target{URL: mustParseURL("/view")},
				Fields: []Field{
					{Name: "id", Type: "text", Value: "42", ReadOnly: true},
				},
			},
		},
	}
	html := encodeHTML(t, rep, RenderFragment)
	if !strings.Contains(html, "readonly") {
		t.Error("expected readonly attribute")
	}
}

func TestHTMLCodec_TextareaField(t *testing.T) {
	rep := Representation{
		Actions: []Action{
			{
				Name:   "comment",
				Method: "POST",
				Target: Target{URL: mustParseURL("/comment")},
				Fields: []Field{
					{Name: "body", Type: "textarea", Value: "Hello world"},
				},
			},
		},
	}
	html := encodeHTML(t, rep, RenderFragment)
	if !strings.Contains(html, "<textarea") {
		t.Error("expected textarea element")
	}
	if !strings.Contains(html, "Hello world</textarea>") {
		t.Error("expected textarea content")
	}
}

func TestHTMLCodec_RouteTarget(t *testing.T) {
	rep := Representation{
		Links: []Link{
			{Rel: "item", Target: Target{Route: &RouteRef{Name: "items"}}},
		},
	}
	html := encodeHTML(t, rep, RenderFragment)
	if !strings.Contains(html, `href="/resolved/items"`) {
		t.Error("expected resolved route href")
	}
}

func TestHTMLCodec_ActionHints_HtmxAttributes(t *testing.T) {
	rep := Representation{
		Actions: []Action{
			{
				Name:   "Update Email",
				Method: "POST",
				Target: Target{URL: mustParseURL("/contacts/42/email")},
				Fields: []Field{
					{Name: "email", Type: "email"},
				},
				Hints: map[string]any{
					"hx-target": "#contact-email",
					"hx-swap":   "outerHTML",
				},
			},
		},
	}
	html := encodeHTML(t, rep, RenderFragment)
	if !strings.Contains(html, `hx-target="#contact-email"`) {
		t.Error("expected hx-target attribute on form")
	}
	if !strings.Contains(html, `hx-swap="outerHTML"`) {
		t.Error("expected hx-swap attribute on form")
	}
}

func TestHTMLCodec_ActionHints_Hidden(t *testing.T) {
	rep := Representation{
		Actions: []Action{
			{
				Name:   "Secret Action",
				Method: "POST",
				Target: Target{URL: mustParseURL("/secret")},
				Hints: map[string]any{
					"hidden": true,
				},
			},
		},
	}
	html := encodeHTML(t, rep, RenderFragment)
	if strings.Contains(html, "Secret Action") {
		t.Error("action with hidden: true should not be rendered")
	}
	if strings.Contains(html, "<form") {
		t.Error("hidden action should not produce a form element")
	}
}

func TestHTMLCodec_ActionHints_Destructive(t *testing.T) {
	rep := Representation{
		Actions: []Action{
			{
				Name:   "Delete",
				Method: "DELETE",
				Target: Target{URL: mustParseURL("/contacts/42")},
				Hints: map[string]any{
					"destructive": true,
				},
			},
		},
	}
	html := encodeHTML(t, rep, RenderFragment)
	if !strings.Contains(html, `class="destructive"`) {
		t.Error("expected class=\"destructive\" on form for destructive hint")
	}
}

func TestHTMLCodec_ActionHints_HTMLEscaping(t *testing.T) {
	rep := Representation{
		Actions: []Action{
			{
				Name:   "Test",
				Method: "POST",
				Target: Target{URL: mustParseURL("/test")},
				Hints: map[string]any{
					"hx-target": `<script>alert("xss")</script>`,
				},
			},
		},
	}
	html := encodeHTML(t, rep, RenderFragment)
	if strings.Contains(html, "<script>") {
		t.Error("hint values must be HTML-escaped")
	}
	if !strings.Contains(html, "hx-target=") {
		t.Error("expected hx-target attribute to be present")
	}
}

func TestHTMLCodec_RepresentationHints(t *testing.T) {
	rep := Representation{
		Kind: "contact",
		Hints: map[string]any{
			"id":      "main-contact",
			"hx-boost": "true",
		},
	}
	html := encodeHTML(t, rep, RenderFragment)
	if !strings.Contains(html, `id="main-contact"`) {
		t.Error("expected id attribute from representation hints")
	}
	if !strings.Contains(html, `hx-boost="true"`) {
		t.Error("expected hx-boost attribute from representation hints")
	}
}

func TestHTMLCodec_EmptyRepresentation(t *testing.T) {
	html := encodeHTML(t, Representation{}, RenderFragment)
	if !strings.Contains(html, "<article>") {
		t.Error("expected article even for empty representation")
	}
	if !strings.Contains(html, "</article>") {
		t.Error("expected closing article tag")
	}
}
