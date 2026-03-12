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
			{Rel: "related", Target: target("/articles/2")},
		},
	}

	doc := MapRepresentation(rep, nil)
	data := doc.Data.Resource()

	if data.Links["related"] != "/articles/2" {
		t.Errorf("links.related = %v, want %q", data.Links["related"], "/articles/2")
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

func TestMapRepresentation_PaginationLinksAtTopLevel(t *testing.T) {
	rep := hyper.Representation{
		Kind:  "articles",
		Self:  targetPtr("/articles?page[number]=3"),
		State: hyper.Collection{},
		Links: []hyper.Link{
			{Rel: "first", Target: target("/articles?page[number]=1")},
			{Rel: "last", Target: target("/articles?page[number]=13")},
			{Rel: "prev", Target: target("/articles?page[number]=2")},
			{Rel: "next", Target: target("/articles?page[number]=4")},
		},
		Embedded: map[string][]hyper.Representation{
			"articles": {
				{Kind: "article", Self: targetPtr("/articles/5"), State: hyper.Object{"title": hyper.Scalar{V: "Post"}}},
			},
		},
	}

	doc := MapRepresentation(rep, nil)

	if doc.Links == nil {
		t.Fatal("expected top-level links")
	}
	if doc.Links["self"] != "/articles?page[number]=3" {
		t.Errorf("links.self = %v, want /articles?page[number]=3", doc.Links["self"])
	}
	if doc.Links["first"] != "/articles?page[number]=1" {
		t.Errorf("links.first = %v, want /articles?page[number]=1", doc.Links["first"])
	}
	if doc.Links["last"] != "/articles?page[number]=13" {
		t.Errorf("links.last = %v, want /articles?page[number]=13", doc.Links["last"])
	}
	if doc.Links["prev"] != "/articles?page[number]=2" {
		t.Errorf("links.prev = %v, want /articles?page[number]=2", doc.Links["prev"])
	}
	if doc.Links["next"] != "/articles?page[number]=4" {
		t.Errorf("links.next = %v, want /articles?page[number]=4", doc.Links["next"])
	}
}

func TestMapRepresentation_SelfLinkAtBothLevels(t *testing.T) {
	rep := hyper.Representation{
		Kind: "article",
		Self: targetPtr("/articles/1"),
		State: hyper.Object{
			"title": hyper.Scalar{V: "Hello"},
		},
	}

	doc := MapRepresentation(rep, nil)

	// Document-level self
	if doc.Links == nil || doc.Links["self"] != "/articles/1" {
		t.Errorf("doc.links.self = %v, want /articles/1", doc.Links["self"])
	}
	// Resource-level self
	data := doc.Data.Resource()
	if data.Links == nil || data.Links["self"] != "/articles/1" {
		t.Errorf("data.links.self = %v, want /articles/1", data.Links["self"])
	}
}

func TestMapRepresentation_PaginationLinksRemovedFromResource(t *testing.T) {
	rep := hyper.Representation{
		Kind: "article",
		Self: targetPtr("/articles/1"),
		Links: []hyper.Link{
			{Rel: "next", Target: target("/articles/2")},
			{Rel: "related", Target: target("/other")},
		},
	}

	doc := MapRepresentation(rep, nil)
	data := doc.Data.Resource()

	// "next" is pagination → should be in doc.Links, NOT in data.Links
	if doc.Links["next"] != "/articles/2" {
		t.Errorf("doc.links.next = %v, want /articles/2", doc.Links["next"])
	}
	if _, ok := data.Links["next"]; ok {
		t.Error("pagination rel 'next' should not appear in resource-level links")
	}
	// "related" is not pagination → stays on resource
	if data.Links["related"] != "/other" {
		t.Errorf("data.links.related = %v, want /other", data.Links["related"])
	}
}

func TestErrorDocument_SingleError(t *testing.T) {
	doc := NewErrorDocument(ErrorObject{
		Status: "404",
		Title:  "Not Found",
		Detail: "The resource does not exist",
	})

	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var raw map[string]json.RawMessage
	json.Unmarshal(out, &raw)

	// Must not have "data"
	if _, ok := raw["data"]; ok {
		t.Error("error document must not contain 'data'")
	}

	var errs []ErrorObject
	json.Unmarshal(raw["errors"], &errs)
	if len(errs) != 1 {
		t.Fatalf("errors length = %d, want 1", len(errs))
	}
	if errs[0].Status != "404" {
		t.Errorf("error.status = %q, want %q", errs[0].Status, "404")
	}
	if errs[0].Title != "Not Found" {
		t.Errorf("error.title = %q, want %q", errs[0].Title, "Not Found")
	}
	if errs[0].Detail != "The resource does not exist" {
		t.Errorf("error.detail = %q, want %q", errs[0].Detail, "The resource does not exist")
	}
}

func TestErrorDocument_ValidationErrors(t *testing.T) {
	doc := MapFieldErrors("422", map[string]string{
		"name":  "is required",
		"email": "is invalid",
	})

	if len(doc.Errors) != 2 {
		t.Fatalf("errors length = %d, want 2", len(doc.Errors))
	}

	byPointer := map[string]ErrorObject{}
	for _, e := range doc.Errors {
		if e.Source == nil {
			t.Fatal("expected source on validation error")
		}
		byPointer[e.Source.Pointer] = e
	}

	nameErr, ok := byPointer["/data/attributes/name"]
	if !ok {
		t.Fatal("expected error for /data/attributes/name")
	}
	if nameErr.Status != "422" {
		t.Errorf("name error status = %q, want %q", nameErr.Status, "422")
	}
	if nameErr.Detail != "is required" {
		t.Errorf("name error detail = %q, want %q", nameErr.Detail, "is required")
	}

	emailErr, ok := byPointer["/data/attributes/email"]
	if !ok {
		t.Fatal("expected error for /data/attributes/email")
	}
	if emailErr.Detail != "is invalid" {
		t.Errorf("email error detail = %q, want %q", emailErr.Detail, "is invalid")
	}
}

func TestErrorDocument_WithMeta(t *testing.T) {
	doc := ErrorDocument{
		Errors: []ErrorObject{
			{Status: "500", Title: "Internal Server Error"},
		},
		Meta: map[string]any{
			"request_id": "abc-123",
		},
	}

	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var raw map[string]json.RawMessage
	json.Unmarshal(out, &raw)

	var meta map[string]any
	json.Unmarshal(raw["meta"], &meta)
	if meta["request_id"] != "abc-123" {
		t.Errorf("meta.request_id = %v, want abc-123", meta["request_id"])
	}
}

func TestErrorDocument_NoData(t *testing.T) {
	doc := NewErrorDocument(ErrorObject{Status: "403", Title: "Forbidden"})

	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var raw map[string]any
	json.Unmarshal(out, &raw)

	if _, ok := raw["data"]; ok {
		t.Error("error document must not contain 'data' key")
	}
	if _, ok := raw["errors"]; !ok {
		t.Error("error document must contain 'errors' key")
	}
}

func TestErrorObject_CRUDScenarios(t *testing.T) {
	tests := []struct {
		name string
		err  ErrorObject
	}{
		{
			name: "403 Forbidden",
			err:  ErrorObject{Status: "403", Title: "Forbidden", Detail: "This operation is not supported"},
		},
		{
			name: "404 Not Found",
			err:  ErrorObject{Status: "404", Title: "Not Found", Detail: "Resource does not exist"},
		},
		{
			name: "409 Conflict",
			err:  ErrorObject{Status: "409", Title: "Conflict", Detail: "Type mismatch"},
		},
		{
			name: "422 with source pointer",
			err: ErrorObject{
				Status: "422",
				Title:  "Unprocessable Entity",
				Detail: "Name is required",
				Source: &ErrorSource{Pointer: "/data/attributes/name"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := NewErrorDocument(tt.err)
			out, err := json.Marshal(doc)
			if err != nil {
				t.Fatalf("marshal error: %v", err)
			}
			// Round-trip
			var result ErrorDocument
			if err := json.Unmarshal(out, &result); err != nil {
				t.Fatalf("unmarshal error: %v", err)
			}
			if len(result.Errors) != 1 {
				t.Fatalf("errors length = %d, want 1", len(result.Errors))
			}
			if result.Errors[0].Status != tt.err.Status {
				t.Errorf("status = %q, want %q", result.Errors[0].Status, tt.err.Status)
			}
			if result.Errors[0].Detail != tt.err.Detail {
				t.Errorf("detail = %q, want %q", result.Errors[0].Detail, tt.err.Detail)
			}
			if tt.err.Source != nil && result.Errors[0].Source.Pointer != tt.err.Source.Pointer {
				t.Errorf("source.pointer = %q, want %q", result.Errors[0].Source.Pointer, tt.err.Source.Pointer)
			}
		})
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
