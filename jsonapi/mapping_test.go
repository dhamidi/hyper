package jsonapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/dhamidi/hyper"
)

func mustURL(raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	return u
}

func target(raw string) hyper.Target {
	return hyper.Target{URL: mustURL(raw)}
}

func targetPtr(raw string) *hyper.Target {
	t := target(raw)
	return &t
}

func TestMapRepresentation_BasicResource(t *testing.T) {
	rep := hyper.Representation{
		Kind: "contact",
		Self: targetPtr("/contacts/42"),
		State: hyper.Object{
			"name":  hyper.Scalar{V: "Ada Lovelace"},
			"email": hyper.Scalar{V: "ada@example.com"},
		},
	}

	doc := MapRepresentation(rep, nil)
	data := doc.Data.Resource()

	if data == nil {
		t.Fatal("expected data to be non-nil")
	}
	if data.Type != "contact" {
		t.Errorf("type = %q, want %q", data.Type, "contact")
	}
	if data.ID != "42" {
		t.Errorf("id = %q, want %q", data.ID, "42")
	}
	if data.Attributes["name"] != "Ada Lovelace" {
		t.Errorf("attributes.name = %v, want %q", data.Attributes["name"], "Ada Lovelace")
	}
	if data.Links["self"] != "/contacts/42" {
		t.Errorf("links.self = %v, want %q", data.Links["self"], "/contacts/42")
	}
}

func TestMapRepresentation_IDFromState(t *testing.T) {
	rep := hyper.Representation{
		Kind: "user",
		Self: targetPtr("/users/99"),
		State: hyper.Object{
			"id":   hyper.Scalar{V: "custom-id"},
			"name": hyper.Scalar{V: "Test"},
		},
	}

	doc := MapRepresentation(rep, nil)
	data := doc.Data.Resource()

	if data.ID != "custom-id" {
		t.Errorf("id = %q, want %q", data.ID, "custom-id")
	}
	// "id" should be removed from attributes
	if _, ok := data.Attributes["id"]; ok {
		t.Error("id should not appear in attributes")
	}
}

func TestMapRepresentation_WithLinks(t *testing.T) {
	rep := hyper.Representation{
		Kind: "article",
		Self: targetPtr("/articles/1"),
		Links: []hyper.Link{
			{Rel: "next", Target: target("/articles/2")},
		},
	}

	doc := MapRepresentation(rep, nil)
	data := doc.Data.Resource()

	if data.Links["next"] != "/articles/2" {
		t.Errorf("links.next = %v, want %q", data.Links["next"], "/articles/2")
	}
}

func TestMapRepresentation_WithEmbedded(t *testing.T) {
	rep := hyper.Representation{
		Kind: "article",
		Self: targetPtr("/articles/1"),
		State: hyper.Object{
			"title": hyper.Scalar{V: "Hello"},
		},
		Links: []hyper.Link{
			{Rel: "author", Target: target("/users/7")},
		},
		Embedded: map[string][]hyper.Representation{
			"author": {
				{
					Kind: "user",
					Self: targetPtr("/users/7"),
					State: hyper.Object{
						"name": hyper.Scalar{V: "Ada"},
					},
				},
			},
		},
	}

	doc := MapRepresentation(rep, nil)
	data := doc.Data.Resource()

	// "author" link should become a relationship, not a plain link
	if _, ok := data.Links["author"]; ok {
		t.Error("author should be a relationship, not a plain link")
	}
	rel, ok := data.Relationships["author"]
	if !ok {
		t.Fatal("expected author relationship")
	}
	rid, ok := rel.Data.(ResourceIdentifier)
	if !ok {
		t.Fatalf("relationship data type = %T, want ResourceIdentifier", rel.Data)
	}
	if rid.Type != "user" || rid.ID != "7" {
		t.Errorf("relationship identifier = %+v, want {user, 7}", rid)
	}
	if rel.Links["related"] != "/users/7" {
		t.Errorf("relationship links.related = %v, want %q", rel.Links["related"], "/users/7")
	}

	// Included array
	if len(doc.Included) != 1 {
		t.Fatalf("included length = %d, want 1", len(doc.Included))
	}
	if doc.Included[0].Type != "user" {
		t.Errorf("included[0].type = %q, want %q", doc.Included[0].Type, "user")
	}
}

func TestMapRepresentation_WithActions(t *testing.T) {
	rep := hyper.Representation{
		Kind: "contact",
		Self: targetPtr("/contacts/42"),
		Actions: []hyper.Action{
			{
				Name:   "update",
				Rel:    "update",
				Method: "PUT",
				Target: target("/contacts/42"),
				Fields: []hyper.Field{
					{Name: "name", Type: "text", Required: true},
				},
			},
		},
	}

	doc := MapRepresentation(rep, nil)

	actions, ok := doc.Meta["actions"].([]map[string]any)
	if !ok {
		t.Fatalf("meta.actions type = %T, want []map[string]any", doc.Meta["actions"])
	}
	if len(actions) != 1 {
		t.Fatalf("actions length = %d, want 1", len(actions))
	}
	if actions[0]["name"] != "update" {
		t.Errorf("action name = %v, want %q", actions[0]["name"], "update")
	}
	if actions[0]["method"] != "PUT" {
		t.Errorf("action method = %v, want %q", actions[0]["method"], "PUT")
	}
}

func TestMapRepresentation_WithMeta(t *testing.T) {
	rep := hyper.Representation{
		Kind: "list",
		Meta: map[string]any{
			"total": 100,
			"page":  1,
		},
	}

	doc := MapRepresentation(rep, nil)

	if doc.Meta["total"] != 100 {
		t.Errorf("meta.total = %v, want 100", doc.Meta["total"])
	}
}

func TestMapRepresentation_EmptyRepresentation(t *testing.T) {
	rep := hyper.Representation{Kind: "empty"}

	doc := MapRepresentation(rep, nil)
	data := doc.Data.Resource()

	if data == nil {
		t.Fatal("expected data to be non-nil")
	}
	if data.Type != "empty" {
		t.Errorf("type = %q, want %q", data.Type, "empty")
	}
	if data.ID != "" {
		t.Errorf("id = %q, want empty", data.ID)
	}
}

func TestMapToSubmission(t *testing.T) {
	doc := Document{
		Data: PrimaryData{One: &Resource{
			Type: "contact",
			ID:   "42",
			Attributes: map[string]any{
				"name":  "Ada",
				"email": "ada@example.com",
			},
		}},
	}

	result := MapToSubmission(doc)

	if result["id"] != "42" {
		t.Errorf("id = %v, want %q", result["id"], "42")
	}
	if result["name"] != "Ada" {
		t.Errorf("name = %v, want %q", result["name"], "Ada")
	}
	if result["email"] != "ada@example.com" {
		t.Errorf("email = %v, want %q", result["email"], "ada@example.com")
	}
}

func TestMapToSubmission_NilData(t *testing.T) {
	doc := Document{}
	result := MapToSubmission(doc)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestRepresentationCodec_Encode(t *testing.T) {
	codec := RepresentationCodec{}

	if mt := codec.MediaTypes(); len(mt) != 1 || mt[0] != "application/vnd.api+json" {
		t.Errorf("MediaTypes() = %v, want [application/vnd.api+json]", mt)
	}

	rep := hyper.Representation{
		Kind: "contact",
		Self: targetPtr("/contacts/1"),
		State: hyper.Object{
			"name": hyper.Scalar{V: "Test"},
		},
	}

	var buf bytes.Buffer
	err := codec.Encode(context.Background(), &buf, rep, hyper.EncodeOptions{})
	if err != nil {
		t.Fatalf("Encode error: %v", err)
	}

	var doc Document
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if doc.Data.Resource().Type != "contact" {
		t.Errorf("type = %q, want %q", doc.Data.Resource().Type, "contact")
	}
}

func TestSubmissionCodec_Decode(t *testing.T) {
	codec := SubmissionCodec{}

	if mt := codec.MediaTypes(); len(mt) != 1 || mt[0] != "application/vnd.api+json" {
		t.Errorf("MediaTypes() = %v, want [application/vnd.api+json]", mt)
	}

	body := `{"data":{"type":"contact","id":"42","attributes":{"name":"Ada","email":"ada@example.com"}}}`
	r := bytes.NewReader([]byte(body))

	var result map[string]any
	err := codec.Decode(context.Background(), r, &result, hyper.DecodeOptions{})
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if result["id"] != "42" {
		t.Errorf("id = %v, want %q", result["id"], "42")
	}
	if result["name"] != "Ada" {
		t.Errorf("name = %v, want %q", result["name"], "Ada")
	}
}

func TestSubmissionCodec_DecodeInvalidTarget(t *testing.T) {
	codec := SubmissionCodec{}
	r := bytes.NewReader([]byte(`{}`))

	var s string
	err := codec.Decode(context.Background(), r, &s, hyper.DecodeOptions{})
	if err == nil {
		t.Error("expected error for invalid target type")
	}
}

func TestMapRepresentation_MultipleEmbedded(t *testing.T) {
	rep := hyper.Representation{
		Kind: "article",
		Self: targetPtr("/articles/1"),
		Embedded: map[string][]hyper.Representation{
			"tags": {
				{Kind: "tag", Self: targetPtr("/tags/1"), State: hyper.Object{"name": hyper.Scalar{V: "go"}}},
				{Kind: "tag", Self: targetPtr("/tags/2"), State: hyper.Object{"name": hyper.Scalar{V: "rest"}}},
			},
		},
	}

	doc := MapRepresentation(rep, nil)

	rel, ok := doc.Data.Resource().Relationships["tags"]
	if !ok {
		t.Fatal("expected tags relationship")
	}
	ids, ok := rel.Data.([]ResourceIdentifier)
	if !ok {
		t.Fatalf("relationship data type = %T, want []ResourceIdentifier", rel.Data)
	}
	if len(ids) != 2 {
		t.Fatalf("tags length = %d, want 2", len(ids))
	}
	if ids[0].ID != "1" || ids[1].ID != "2" {
		t.Errorf("tag ids = %v, want [1, 2]", ids)
	}
}

func TestMapRepresentation_CollectionMultipleItems(t *testing.T) {
	rep := hyper.Representation{
		Kind:  "contacts",
		Self:  targetPtr("/contacts"),
		State: hyper.Collection{},
		Embedded: map[string][]hyper.Representation{
			"contacts": {
				{
					Kind:  "contact",
					Self:  targetPtr("/contacts/1"),
					State: hyper.Object{"name": hyper.Scalar{V: "Ada"}},
				},
				{
					Kind:  "contact",
					Self:  targetPtr("/contacts/2"),
					State: hyper.Object{"name": hyper.Scalar{V: "Grace"}},
				},
			},
		},
	}

	doc := MapRepresentation(rep, nil)

	if !doc.Data.IsMany {
		t.Fatal("expected collection (IsMany=true)")
	}
	if len(doc.Data.Many) != 2 {
		t.Fatalf("data length = %d, want 2", len(doc.Data.Many))
	}
	if doc.Data.Many[0].Type != "contact" {
		t.Errorf("data[0].type = %q, want %q", doc.Data.Many[0].Type, "contact")
	}
	if doc.Data.Many[0].Attributes["name"] != "Ada" {
		t.Errorf("data[0].attributes.name = %v, want %q", doc.Data.Many[0].Attributes["name"], "Ada")
	}
	if doc.Data.Many[1].Attributes["name"] != "Grace" {
		t.Errorf("data[1].attributes.name = %v, want %q", doc.Data.Many[1].Attributes["name"], "Grace")
	}
}

func TestMapRepresentation_EmptyCollection(t *testing.T) {
	rep := hyper.Representation{
		Kind:  "contacts",
		Self:  targetPtr("/contacts"),
		State: hyper.Collection{},
	}

	doc := MapRepresentation(rep, nil)

	if !doc.Data.IsMany {
		t.Fatal("expected collection (IsMany=true)")
	}
	if len(doc.Data.Many) != 0 {
		t.Fatalf("data length = %d, want 0", len(doc.Data.Many))
	}

	// Verify JSON output is an empty array
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var raw map[string]json.RawMessage
	json.Unmarshal(out, &raw)
	if string(raw["data"]) != "[]" {
		t.Errorf("JSON data = %s, want []", raw["data"])
	}
}

func TestMapRepresentation_SingleItemCollection(t *testing.T) {
	rep := hyper.Representation{
		Kind:  "contacts",
		Self:  targetPtr("/contacts"),
		State: hyper.Collection{},
		Embedded: map[string][]hyper.Representation{
			"contacts": {
				{
					Kind:  "contact",
					Self:  targetPtr("/contacts/1"),
					State: hyper.Object{"name": hyper.Scalar{V: "Ada"}},
				},
			},
		},
	}

	doc := MapRepresentation(rep, nil)

	if !doc.Data.IsMany {
		t.Fatal("expected collection (IsMany=true) even for single item")
	}
	if len(doc.Data.Many) != 1 {
		t.Fatalf("data length = %d, want 1", len(doc.Data.Many))
	}
	if doc.Data.Many[0].Type != "contact" {
		t.Errorf("data[0].type = %q, want %q", doc.Data.Many[0].Type, "contact")
	}

	// Verify JSON output is an array, not an object
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var raw map[string]json.RawMessage
	json.Unmarshal(out, &raw)
	if raw["data"][0] != '[' {
		t.Errorf("JSON data should be array, got: %s", raw["data"])
	}
}

func TestMapNullDocument(t *testing.T) {
	doc := MapNullDocument()

	if !doc.Data.IsNull {
		t.Fatal("expected IsNull=true")
	}
	if doc.Data.Resource() != nil {
		t.Error("Resource() should return nil for null data")
	}

	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var raw map[string]json.RawMessage
	json.Unmarshal(out, &raw)
	if string(raw["data"]) != "null" {
		t.Errorf("JSON data = %s, want null", raw["data"])
	}
}

func TestPrimaryData_UnmarshalNull(t *testing.T) {
	input := `{"data":null}`
	var doc Document
	if err := json.Unmarshal([]byte(input), &doc); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if !doc.Data.IsNull {
		t.Error("expected IsNull=true after unmarshaling null data")
	}
}

func TestPrimaryData_UnmarshalArray(t *testing.T) {
	input := `{"data":[{"type":"contact","id":"1","attributes":{"name":"Ada"}},{"type":"contact","id":"2","attributes":{"name":"Grace"}}]}`
	var doc Document
	if err := json.Unmarshal([]byte(input), &doc); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if !doc.Data.IsMany {
		t.Fatal("expected IsMany=true after unmarshaling array data")
	}
	if len(doc.Data.Many) != 2 {
		t.Fatalf("data length = %d, want 2", len(doc.Data.Many))
	}
	if doc.Data.Many[0].ID != "1" {
		t.Errorf("data[0].id = %q, want %q", doc.Data.Many[0].ID, "1")
	}
}

func TestMapRepresentation_CollectionWithIncluded(t *testing.T) {
	rep := hyper.Representation{
		Kind:  "contacts",
		Self:  targetPtr("/contacts"),
		State: hyper.Collection{},
		Embedded: map[string][]hyper.Representation{
			"contacts": {
				{
					Kind:  "contact",
					Self:  targetPtr("/contacts/1"),
					State: hyper.Object{"name": hyper.Scalar{V: "Ada"}},
					Embedded: map[string][]hyper.Representation{
						"company": {
							{Kind: "company", Self: targetPtr("/companies/10"), State: hyper.Object{"name": hyper.Scalar{V: "Acme"}}},
						},
					},
				},
			},
			"sponsors": {
				{Kind: "sponsor", Self: targetPtr("/sponsors/5"), State: hyper.Object{"name": hyper.Scalar{V: "BigCo"}}},
			},
		},
	}

	doc := MapRepresentation(rep, nil)

	if !doc.Data.IsMany {
		t.Fatal("expected collection")
	}
	if len(doc.Data.Many) != 1 {
		t.Fatalf("data length = %d, want 1", len(doc.Data.Many))
	}
	// Included should have the company sub-resource and the sponsors entry
	if len(doc.Included) != 2 {
		t.Fatalf("included length = %d, want 2", len(doc.Included))
	}
	types := map[string]bool{}
	for _, inc := range doc.Included {
		types[inc.Type] = true
	}
	if !types["company"] {
		t.Error("expected company in included")
	}
	if !types["sponsor"] {
		t.Error("expected sponsor in included")
	}
}

func TestMapRepresentation_CollectionWithActions(t *testing.T) {
	rep := hyper.Representation{
		Kind:  "contacts",
		Self:  targetPtr("/contacts"),
		State: hyper.Collection{},
		Actions: []hyper.Action{
			{
				Name:   "create",
				Rel:    "create",
				Method: "POST",
				Target: target("/contacts"),
				Fields: []hyper.Field{
					{Name: "name", Type: "text", Required: true},
				},
			},
		},
	}

	doc := MapRepresentation(rep, nil)

	if !doc.Data.IsMany {
		t.Fatal("expected collection")
	}
	actions, ok := doc.Meta["actions"].([]map[string]any)
	if !ok {
		t.Fatalf("meta.actions type = %T, want []map[string]any", doc.Meta["actions"])
	}
	if len(actions) != 1 {
		t.Fatalf("actions length = %d, want 1", len(actions))
	}
	if actions[0]["name"] != "create" {
		t.Errorf("action name = %v, want %q", actions[0]["name"], "create")
	}
}

func TestRichTextInAttributes(t *testing.T) {
	rep := hyper.Representation{
		Kind: "post",
		Self: targetPtr("/posts/1"),
		State: hyper.Object{
			"body": hyper.RichText{MediaType: "text/markdown", Source: "# Hello"},
		},
	}

	doc := MapRepresentation(rep, nil)

	body, ok := doc.Data.Resource().Attributes["body"].(map[string]any)
	if !ok {
		t.Fatalf("body type = %T, want map", doc.Data.Resource().Attributes["body"])
	}
	if body["mediaType"] != "text/markdown" {
		t.Errorf("mediaType = %v, want text/markdown", body["mediaType"])
	}
}
