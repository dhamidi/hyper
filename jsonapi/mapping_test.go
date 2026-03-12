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

	if doc.Data == nil {
		t.Fatal("expected data to be non-nil")
	}
	if doc.Data.Type != "contact" {
		t.Errorf("type = %q, want %q", doc.Data.Type, "contact")
	}
	if doc.Data.ID != "42" {
		t.Errorf("id = %q, want %q", doc.Data.ID, "42")
	}
	if doc.Data.Attributes["name"] != "Ada Lovelace" {
		t.Errorf("attributes.name = %v, want %q", doc.Data.Attributes["name"], "Ada Lovelace")
	}
	if doc.Data.Links["self"] != "/contacts/42" {
		t.Errorf("links.self = %v, want %q", doc.Data.Links["self"], "/contacts/42")
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

	if doc.Data.ID != "custom-id" {
		t.Errorf("id = %q, want %q", doc.Data.ID, "custom-id")
	}
	// "id" should be removed from attributes
	if _, ok := doc.Data.Attributes["id"]; ok {
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

	if doc.Data.Links["next"] != "/articles/2" {
		t.Errorf("links.next = %v, want %q", doc.Data.Links["next"], "/articles/2")
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

	// "author" link should become a relationship, not a plain link
	if _, ok := doc.Data.Links["author"]; ok {
		t.Error("author should be a relationship, not a plain link")
	}
	rel, ok := doc.Data.Relationships["author"]
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

	if doc.Data == nil {
		t.Fatal("expected data to be non-nil")
	}
	if doc.Data.Type != "empty" {
		t.Errorf("type = %q, want %q", doc.Data.Type, "empty")
	}
	if doc.Data.ID != "" {
		t.Errorf("id = %q, want empty", doc.Data.ID)
	}
}

func TestMapToSubmission(t *testing.T) {
	doc := Document{
		Data: &Resource{
			Type: "contact",
			ID:   "42",
			Attributes: map[string]any{
				"name":  "Ada",
				"email": "ada@example.com",
			},
		},
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
	if doc.Data.Type != "contact" {
		t.Errorf("type = %q, want %q", doc.Data.Type, "contact")
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

	rel, ok := doc.Data.Relationships["tags"]
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

func TestRichTextInAttributes(t *testing.T) {
	rep := hyper.Representation{
		Kind: "post",
		Self: targetPtr("/posts/1"),
		State: hyper.Object{
			"body": hyper.RichText{MediaType: "text/markdown", Source: "# Hello"},
		},
	}

	doc := MapRepresentation(rep, nil)

	body, ok := doc.Data.Attributes["body"].(map[string]any)
	if !ok {
		t.Fatalf("body type = %T, want map", doc.Data.Attributes["body"])
	}
	if body["mediaType"] != "text/markdown" {
		t.Errorf("mediaType = %v, want text/markdown", body["mediaType"])
	}
}
