package hyper

import (
	"bytes"
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

// mockResolver resolves targets by returning Target.URL directly.
type mockResolver struct{}

func (mockResolver) ResolveTarget(_ context.Context, t Target) (*url.URL, error) {
	if t.URL != nil {
		return t.URL, nil
	}
	// For route refs, return a placeholder
	if t.Route != nil {
		u, _ := url.Parse("/resolved/" + t.Route.Name)
		return u, nil
	}
	u, _ := url.Parse("")
	return u, nil
}

func mustParseURL(raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	return u
}

func encodeToMap(t *testing.T, rep Representation) map[string]any {
	t.Helper()
	var buf bytes.Buffer
	codec := JSONCodec()
	err := codec.Encode(context.Background(), &buf, rep, EncodeOptions{Resolver: mockResolver{}})
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	return result
}

func TestJSONCodec_MediaTypes(t *testing.T) {
	codec := JSONCodec()
	types := codec.MediaTypes()
	if len(types) != 1 || types[0] != "application/json" {
		t.Errorf("expected [application/json], got %v", types)
	}
}

func TestJSONCodec_FullRepresentation(t *testing.T) {
	rep := Representation{
		Kind: "contact",
		Self: &Target{URL: mustParseURL("/contacts/42")},
		State: Object{
			"name":  Scalar{V: "Ada"},
			"email": Scalar{V: "ada@example.com"},
		},
		Links: []Link{
			{Rel: "author", Target: Target{URL: mustParseURL("/users/7")}, Title: "Author Profile", Type: "text/html"},
		},
		Actions: []Action{
			{
				Name:     "update",
				Rel:      "edit",
				Method:   "PUT",
				Target:   Target{URL: mustParseURL("/contacts/42")},
				Consumes: []string{"application/json"},
				Produces: []string{"application/json"},
				Fields: []Field{
					{Name: "name", Type: "text", Required: true, Label: "Name"},
					{Name: "email", Type: "email", Value: "ada@example.com"},
				},
				Hints: map[string]any{"confirm": true},
			},
		},
		Embedded: map[string][]Representation{
			"notes": {
				{Kind: "note", State: Object{"body": Scalar{V: "Hello"}}},
			},
		},
		Meta:  map[string]any{"total": 1},
		Hints: map[string]any{"layout": "detail"},
	}

	result := encodeToMap(t, rep)

	if result["kind"] != "contact" {
		t.Errorf("kind = %v, want contact", result["kind"])
	}

	self := result["self"].(map[string]any)
	if self["href"] != "/contacts/42" {
		t.Errorf("self.href = %v, want /contacts/42", self["href"])
	}

	state := result["state"].(map[string]any)
	if state["name"] != "Ada" {
		t.Errorf("state.name = %v, want Ada", state["name"])
	}

	links := result["links"].([]any)
	if len(links) != 1 {
		t.Fatalf("links length = %d, want 1", len(links))
	}
	link := links[0].(map[string]any)
	if link["rel"] != "author" {
		t.Errorf("link.rel = %v, want author", link["rel"])
	}
	if link["href"] != "/users/7" {
		t.Errorf("link.href = %v, want /users/7", link["href"])
	}
	if link["title"] != "Author Profile" {
		t.Errorf("link.title = %v, want Author Profile", link["title"])
	}
	if link["type"] != "text/html" {
		t.Errorf("link.type = %v, want text/html", link["type"])
	}

	actions := result["actions"].([]any)
	if len(actions) != 1 {
		t.Fatalf("actions length = %d, want 1", len(actions))
	}
	action := actions[0].(map[string]any)
	if action["name"] != "update" {
		t.Errorf("action.name = %v, want update", action["name"])
	}
	if action["method"] != "PUT" {
		t.Errorf("action.method = %v, want PUT", action["method"])
	}
	if action["href"] != "/contacts/42" {
		t.Errorf("action.href = %v, want /contacts/42", action["href"])
	}

	fields := action["fields"].([]any)
	if len(fields) != 2 {
		t.Fatalf("fields length = %d, want 2", len(fields))
	}

	embedded := result["embedded"].(map[string]any)
	notes := embedded["notes"].([]any)
	if len(notes) != 1 {
		t.Fatalf("embedded notes length = %d, want 1", len(notes))
	}
	note := notes[0].(map[string]any)
	if note["kind"] != "note" {
		t.Errorf("embedded note kind = %v, want note", note["kind"])
	}

	meta := result["meta"].(map[string]any)
	if meta["total"] != float64(1) {
		t.Errorf("meta.total = %v, want 1", meta["total"])
	}

	hints := result["hints"].(map[string]any)
	if hints["layout"] != "detail" {
		t.Errorf("hints.layout = %v, want detail", hints["layout"])
	}
}

func TestJSONCodec_ScalarValues(t *testing.T) {
	tests := []struct {
		name  string
		value any
	}{
		{"string", "hello"},
		{"int", float64(42)},
		{"float", 3.14},
		{"bool_true", true},
		{"bool_false", false},
		{"null", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rep := Representation{
				State: Object{"v": Scalar{V: tt.value}},
			}
			result := encodeToMap(t, rep)
			state := result["state"].(map[string]any)
			if state["v"] != tt.value {
				t.Errorf("got %v (%T), want %v (%T)", state["v"], state["v"], tt.value, tt.value)
			}
		})
	}
}

func TestJSONCodec_RichText(t *testing.T) {
	rep := Representation{
		State: Object{
			"bio": RichText{MediaType: "text/markdown", Source: "# Hello"},
		},
	}
	result := encodeToMap(t, rep)
	state := result["state"].(map[string]any)
	bio := state["bio"].(map[string]any)
	if bio["_type"] != "richtext" {
		t.Errorf("_type = %v, want richtext", bio["_type"])
	}
	if bio["mediaType"] != "text/markdown" {
		t.Errorf("mediaType = %v, want text/markdown", bio["mediaType"])
	}
	if bio["source"] != "# Hello" {
		t.Errorf("source = %v, want # Hello", bio["source"])
	}
}

func TestJSONCodec_LinksResolved(t *testing.T) {
	rep := Representation{
		Links: []Link{
			{Rel: "next", Target: Target{URL: mustParseURL("/page/2")}},
		},
	}
	result := encodeToMap(t, rep)
	links := result["links"].([]any)
	link := links[0].(map[string]any)
	if link["href"] != "/page/2" {
		t.Errorf("href = %v, want /page/2", link["href"])
	}
}

func TestJSONCodec_LinkOmitsEmptyTitleAndType(t *testing.T) {
	rep := Representation{
		Links: []Link{
			{Rel: "self", Target: Target{URL: mustParseURL("/x")}},
		},
	}
	result := encodeToMap(t, rep)
	link := result["links"].([]any)[0].(map[string]any)
	if _, ok := link["title"]; ok {
		t.Error("title should be omitted when empty")
	}
	if _, ok := link["type"]; ok {
		t.Error("type should be omitted when empty")
	}
}

func TestJSONCodec_ActionsResolved(t *testing.T) {
	rep := Representation{
		Actions: []Action{
			{
				Name:   "create",
				Rel:    "add",
				Method: "POST",
				Target: Target{URL: mustParseURL("/items")},
				Fields: []Field{
					{Name: "title", Type: "text", Required: true, Label: "Title"},
				},
			},
		},
	}
	result := encodeToMap(t, rep)
	action := result["actions"].([]any)[0].(map[string]any)
	if action["href"] != "/items" {
		t.Errorf("action href = %v, want /items", action["href"])
	}
	fields := action["fields"].([]any)
	f := fields[0].(map[string]any)
	if f["name"] != "title" {
		t.Errorf("field name = %v, want title", f["name"])
	}
	if f["required"] != true {
		t.Errorf("field required = %v, want true", f["required"])
	}
	if f["label"] != "Title" {
		t.Errorf("field label = %v, want Title", f["label"])
	}
}

func TestJSONCodec_EmptyFieldsOmitted(t *testing.T) {
	rep := Representation{}
	result := encodeToMap(t, rep)
	for _, key := range []string{"kind", "self", "state", "links", "actions", "embedded", "meta", "hints"} {
		if _, ok := result[key]; ok {
			t.Errorf("key %q should be omitted for empty representation", key)
		}
	}
}

func TestJSONCodec_EmbeddedRecursive(t *testing.T) {
	rep := Representation{
		Kind: "parent",
		Embedded: map[string][]Representation{
			"children": {
				{
					Kind: "child",
					Self: &Target{URL: mustParseURL("/children/1")},
					State: Object{
						"name": Scalar{V: "Child 1"},
					},
					Embedded: map[string][]Representation{
						"toys": {
							{Kind: "toy", State: Object{"name": Scalar{V: "Ball"}}},
						},
					},
				},
			},
		},
	}
	result := encodeToMap(t, rep)
	children := result["embedded"].(map[string]any)["children"].([]any)
	child := children[0].(map[string]any)
	if child["kind"] != "child" {
		t.Errorf("child kind = %v, want child", child["kind"])
	}
	toys := child["embedded"].(map[string]any)["toys"].([]any)
	toy := toys[0].(map[string]any)
	if toy["kind"] != "toy" {
		t.Errorf("toy kind = %v, want toy", toy["kind"])
	}
	toyState := toy["state"].(map[string]any)
	if toyState["name"] != "Ball" {
		t.Errorf("toy name = %v, want Ball", toyState["name"])
	}
}

func TestJSONCodec_CollectionState(t *testing.T) {
	rep := Representation{
		State: Collection{Scalar{V: "a"}, Scalar{V: "b"}, Scalar{V: float64(3)}},
	}
	result := encodeToMap(t, rep)
	state := result["state"].([]any)
	if len(state) != 3 {
		t.Fatalf("state length = %d, want 3", len(state))
	}
	if state[0] != "a" || state[1] != "b" || state[2] != float64(3) {
		t.Errorf("state = %v, want [a b 3]", state)
	}
}

func TestJSONCodec_FieldOptions(t *testing.T) {
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
	result := encodeToMap(t, rep)
	action := result["actions"].([]any)[0].(map[string]any)
	fields := action["fields"].([]any)
	f := fields[0].(map[string]any)
	opts := f["options"].([]any)
	if len(opts) != 2 {
		t.Fatalf("options length = %d, want 2", len(opts))
	}
	opt1 := opts[1].(map[string]any)
	if opt1["selected"] != true {
		t.Error("second option should be selected")
	}
}

func TestJSONCodec_FileFieldConstraints(t *testing.T) {
	rep := Representation{
		Actions: []Action{
			{
				Name:     "upload",
				Method:   "POST",
				Consumes: []string{"multipart/form-data"},
				Fields: []Field{
					{
						Name:     "file",
						Type:     "file",
						Required: true,
						Label:    "Upload File",
						Accept:   "image/*",
						MaxSize:  10485760, // 10 MB
						Multiple: true,
					},
				},
			},
		},
	}

	result := encodeToMap(t, rep)
	actions := result["actions"].([]any)
	action := actions[0].(map[string]any)
	fields := action["fields"].([]any)
	field := fields[0].(map[string]any)

	if field["accept"] != "image/*" {
		t.Errorf("accept = %v, want image/*", field["accept"])
	}
	if field["maxSize"] != float64(10485760) {
		t.Errorf("maxSize = %v, want 10485760", field["maxSize"])
	}
	if field["multiple"] != true {
		t.Errorf("multiple = %v, want true", field["multiple"])
	}
}

func TestJSONCodec_FileFieldConstraints_OmittedWhenZero(t *testing.T) {
	rep := Representation{
		Actions: []Action{
			{
				Name:   "upload",
				Method: "POST",
				Fields: []Field{
					{
						Name: "file",
						Type: "file",
					},
				},
			},
		},
	}

	result := encodeToMap(t, rep)
	action := result["actions"].([]any)[0].(map[string]any)
	field := action["fields"].([]any)[0].(map[string]any)

	if _, ok := field["accept"]; ok {
		t.Error("accept should be omitted when empty")
	}
	if _, ok := field["maxSize"]; ok {
		t.Error("maxSize should be omitted when zero")
	}
	if _, ok := field["multiple"]; ok {
		t.Error("multiple should be omitted when false")
	}
}

func TestJSONSubmissionCodec_MediaTypes(t *testing.T) {
	codec := JSONSubmissionCodec()
	types := codec.MediaTypes()
	if len(types) != 1 || types[0] != "application/json" {
		t.Errorf("expected [application/json], got %v", types)
	}
}

func TestJSONSubmissionCodec_DecodeMap(t *testing.T) {
	body := `{"name":"Ada","age":30,"active":true}`
	codec := JSONSubmissionCodec()
	var result map[string]any
	err := codec.Decode(context.Background(), strings.NewReader(body), &result, DecodeOptions{})
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if result["name"] != "Ada" {
		t.Errorf("name = %v, want Ada", result["name"])
	}
	if result["age"] != float64(30) {
		t.Errorf("age = %v, want 30", result["age"])
	}
	if result["active"] != true {
		t.Errorf("active = %v, want true", result["active"])
	}
}

func TestJSONSubmissionCodec_InvalidTarget(t *testing.T) {
	codec := JSONSubmissionCodec()
	var s string
	err := codec.Decode(context.Background(), strings.NewReader("{}"), &s, DecodeOptions{})
	if err == nil {
		t.Error("expected error for unsupported target type")
	}
}

func TestJSONCodec_RoundTrip(t *testing.T) {
	rep := Representation{
		Kind: "item",
		Self: &Target{URL: mustParseURL("/items/1")},
		State: Object{
			"title":  Scalar{V: "Test"},
			"count":  Scalar{V: float64(42)},
			"active": Scalar{V: true},
		},
	}

	// Encode
	var buf bytes.Buffer
	codec := JSONCodec()
	err := codec.Encode(context.Background(), &buf, rep, EncodeOptions{Resolver: mockResolver{}})
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	// Decode state values via submission codec
	subCodec := JSONSubmissionCodec()
	var encoded map[string]any
	err = json.Unmarshal(buf.Bytes(), &encoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	stateJSON, _ := json.Marshal(encoded["state"])
	var state map[string]any
	err = subCodec.Decode(context.Background(), bytes.NewReader(stateJSON), &state, DecodeOptions{})
	if err != nil {
		t.Fatalf("Decode state failed: %v", err)
	}
	if state["title"] != "Test" {
		t.Errorf("title = %v, want Test", state["title"])
	}
	if state["count"] != float64(42) {
		t.Errorf("count = %v, want 42", state["count"])
	}
	if state["active"] != true {
		t.Errorf("active = %v, want true", state["active"])
	}
}

func TestJSONCodec_RouteRefResolved(t *testing.T) {
	rep := Representation{
		Links: []Link{
			{Rel: "item", Target: Target{Route: &RouteRef{Name: "items"}}},
		},
	}
	result := encodeToMap(t, rep)
	link := result["links"].([]any)[0].(map[string]any)
	if link["href"] != "/resolved/items" {
		t.Errorf("href = %v, want /resolved/items", link["href"])
	}
}
