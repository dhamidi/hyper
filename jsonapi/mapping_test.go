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

	if result["_type"] != "contact" {
		t.Errorf("_type = %v, want %q", result["_type"], "contact")
	}
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
	if result["_type"] != "contact" {
		t.Errorf("_type = %v, want %q", result["_type"], "contact")
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

func TestSubmissionCodec_DecodeToSubmission(t *testing.T) {
	codec := SubmissionCodec{}
	body := `{"data":{"type":"contact","id":"42","attributes":{"name":"Ada"},"relationships":{"company":{"data":{"type":"companies","id":"10"}}}}}`
	r := bytes.NewReader([]byte(body))

	var sub Submission
	err := codec.Decode(context.Background(), r, &sub, hyper.DecodeOptions{})
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if sub.Type != "contact" {
		t.Errorf("Type = %q, want %q", sub.Type, "contact")
	}
	if sub.ID != "42" {
		t.Errorf("ID = %q, want %q", sub.ID, "42")
	}
	if sub.Attributes["name"] != "Ada" {
		t.Errorf("Attributes[name] = %v, want %q", sub.Attributes["name"], "Ada")
	}
	rel, ok := sub.Relationships["company"]
	if !ok {
		t.Fatal("expected company relationship")
	}
	if rel.One == nil || rel.One.Type != "companies" || rel.One.ID != "10" {
		t.Errorf("company relationship = %+v, want {companies, 10}", rel.One)
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

// --- Submission test cases ---

func TestMapToSubmission_CreateWithTypeAndAttributes(t *testing.T) {
	body := `{"data":{"type":"contacts","attributes":{"name":"Ada"}}}`
	var doc Document
	json.Unmarshal([]byte(body), &doc)

	result := MapToSubmission(doc)
	if result["_type"] != "contacts" {
		t.Errorf("_type = %v, want %q", result["_type"], "contacts")
	}
	if result["name"] != "Ada" {
		t.Errorf("name = %v, want %q", result["name"], "Ada")
	}
	// No id on create without client-generated ID
	if _, ok := result["id"]; ok {
		t.Error("id should not be present for create without client-generated ID")
	}
}

func TestMapToSubmission_CreateWithRelationships(t *testing.T) {
	body := `{
		"data": {
			"type": "articles",
			"attributes": {"title": "Rails is Omakase"},
			"relationships": {
				"author": {"data": {"type": "people", "id": "9"}},
				"tags": {"data": [{"type": "tags", "id": "2"}, {"type": "tags", "id": "3"}]}
			}
		}
	}`
	var doc Document
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	result := MapToSubmission(doc)

	// Flat convenience keys
	if result["author_id"] != "9" {
		t.Errorf("author_id = %v, want %q", result["author_id"], "9")
	}
	tagIDs, ok := result["tags_ids"].([]string)
	if !ok {
		t.Fatalf("tags_ids type = %T, want []string", result["tags_ids"])
	}
	if len(tagIDs) != 2 || tagIDs[0] != "2" || tagIDs[1] != "3" {
		t.Errorf("tags_ids = %v, want [2 3]", tagIDs)
	}

	// Structured relationships
	rels, ok := result["_relationships"].(map[string]any)
	if !ok {
		t.Fatalf("_relationships type = %T, want map[string]any", result["_relationships"])
	}
	authorRel, ok := rels["author"].(map[string]any)
	if !ok {
		t.Fatalf("_relationships.author type = %T", rels["author"])
	}
	if authorRel["type"] != "people" || authorRel["id"] != "9" {
		t.Errorf("author rel = %v, want {people 9}", authorRel)
	}
}

func TestMapToSubmission_UpdateWithIDTypeAndAttributes(t *testing.T) {
	body := `{"data":{"type":"contacts","id":"42","attributes":{"name":"Ada Updated"}}}`
	var doc Document
	json.Unmarshal([]byte(body), &doc)

	result := MapToSubmission(doc)
	if result["_type"] != "contacts" {
		t.Errorf("_type = %v, want %q", result["_type"], "contacts")
	}
	if result["id"] != "42" {
		t.Errorf("id = %v, want %q", result["id"], "42")
	}
	if result["name"] != "Ada Updated" {
		t.Errorf("name = %v, want %q", result["name"], "Ada Updated")
	}
}

func TestMapToSubmission_UpdateWithRelationshipChanges(t *testing.T) {
	body := `{
		"data": {
			"type": "articles",
			"id": "1",
			"attributes": {"title": "Updated Title"},
			"relationships": {
				"author": {"data": {"type": "people", "id": "12"}}
			}
		}
	}`
	var doc Document
	json.Unmarshal([]byte(body), &doc)

	result := MapToSubmission(doc)
	if result["_type"] != "articles" {
		t.Errorf("_type = %v, want %q", result["_type"], "articles")
	}
	if result["id"] != "1" {
		t.Errorf("id = %v, want %q", result["id"], "1")
	}
	if result["title"] != "Updated Title" {
		t.Errorf("title = %v, want %q", result["title"], "Updated Title")
	}
	if result["author_id"] != "12" {
		t.Errorf("author_id = %v, want %q", result["author_id"], "12")
	}
}

func TestMapToSubmission_RelationshipOnlyToOne(t *testing.T) {
	// PATCH /articles/1/relationships/author
	body := `{"data":{"type":"people","id":"12"}}`
	var doc Document
	json.Unmarshal([]byte(body), &doc)

	result := MapToSubmission(doc)
	// Single resource identifier treated as a resource with type and id
	if result["_type"] != "people" {
		t.Errorf("_type = %v, want %q", result["_type"], "people")
	}
	if result["id"] != "12" {
		t.Errorf("id = %v, want %q", result["id"], "12")
	}
}

func TestMapToSubmission_RelationshipOnlyToMany(t *testing.T) {
	// POST /articles/1/relationships/tags
	body := `{"data":[{"type":"tags","id":"2"},{"type":"tags","id":"3"}]}`
	var doc Document
	json.Unmarshal([]byte(body), &doc)

	result := MapToSubmission(doc)
	data, ok := result["_relationship_data"].([]map[string]any)
	if !ok {
		t.Fatalf("_relationship_data type = %T, want []map[string]any", result["_relationship_data"])
	}
	if len(data) != 2 {
		t.Fatalf("_relationship_data length = %d, want 2", len(data))
	}
	if data[0]["type"] != "tags" || data[0]["id"] != "2" {
		t.Errorf("data[0] = %v, want {tags 2}", data[0])
	}
	if data[1]["type"] != "tags" || data[1]["id"] != "3" {
		t.Errorf("data[1] = %v, want {tags 3}", data[1])
	}
}

func TestMapToSubmission_ClearToOneRelationship(t *testing.T) {
	// PATCH /articles/1/relationships/author with null data
	body := `{"data":null}`
	var doc Document
	json.Unmarshal([]byte(body), &doc)

	result := MapToSubmission(doc)
	isNull, ok := result["_null"].(bool)
	if !ok || !isNull {
		t.Errorf("_null = %v, want true", result["_null"])
	}
}

func TestMapToSubmission_ClearToManyRelationship(t *testing.T) {
	// PATCH /articles/1/relationships/tags with empty array
	body := `{"data":[]}`
	var doc Document
	json.Unmarshal([]byte(body), &doc)

	result := MapToSubmission(doc)
	data, ok := result["_relationship_data"].([]map[string]any)
	if !ok {
		t.Fatalf("_relationship_data type = %T, want []map[string]any", result["_relationship_data"])
	}
	if len(data) != 0 {
		t.Errorf("_relationship_data length = %d, want 0", len(data))
	}
}

func TestMapToSubmission_ClientGeneratedID(t *testing.T) {
	body := `{"data":{"type":"contacts","id":"550e8400-e29b-41d4-a716-446655440000","attributes":{"name":"Ada"}}}`
	var doc Document
	json.Unmarshal([]byte(body), &doc)

	result := MapToSubmission(doc)
	if result["_type"] != "contacts" {
		t.Errorf("_type = %v, want %q", result["_type"], "contacts")
	}
	if result["id"] != "550e8400-e29b-41d4-a716-446655440000" {
		t.Errorf("id = %v, want UUID", result["id"])
	}
	if result["name"] != "Ada" {
		t.Errorf("name = %v, want %q", result["name"], "Ada")
	}
}

func TestMapToSubmission_TypeAlwaysExtractable(t *testing.T) {
	// Even with no attributes, type should be present
	body := `{"data":{"type":"contacts"}}`
	var doc Document
	json.Unmarshal([]byte(body), &doc)

	result := MapToSubmission(doc)
	if result["_type"] != "contacts" {
		t.Errorf("_type = %v, want %q", result["_type"], "contacts")
	}
}

// --- MapSubmission (structured) test cases ---

func TestMapSubmission_CreateWithRelationships(t *testing.T) {
	body := `{
		"data": {
			"type": "articles",
			"attributes": {"title": "Hello"},
			"relationships": {
				"author": {"data": {"type": "people", "id": "9"}},
				"tags": {"data": [{"type": "tags", "id": "2"}, {"type": "tags", "id": "3"}]}
			}
		}
	}`
	var doc Document
	json.Unmarshal([]byte(body), &doc)

	sub := MapSubmission(doc)
	if sub == nil {
		t.Fatal("expected non-nil submission")
	}
	if sub.Type != "articles" {
		t.Errorf("Type = %q, want %q", sub.Type, "articles")
	}
	if sub.Attributes["title"] != "Hello" {
		t.Errorf("Attributes[title] = %v, want %q", sub.Attributes["title"], "Hello")
	}

	author, ok := sub.Relationships["author"]
	if !ok {
		t.Fatal("expected author relationship")
	}
	if author.One == nil || author.One.Type != "people" || author.One.ID != "9" {
		t.Errorf("author = %+v, want {people 9}", author.One)
	}

	tags, ok := sub.Relationships["tags"]
	if !ok {
		t.Fatal("expected tags relationship")
	}
	if !tags.IsMany || len(tags.Many) != 2 {
		t.Fatalf("tags.Many length = %d, want 2", len(tags.Many))
	}
	if tags.Many[0].Type != "tags" || tags.Many[0].ID != "2" {
		t.Errorf("tags[0] = %+v, want {tags 2}", tags.Many[0])
	}
}

func TestMapSubmission_RelationshipOnlyToOne(t *testing.T) {
	body := `{"data":{"type":"people","id":"12"}}`
	var doc Document
	json.Unmarshal([]byte(body), &doc)

	sub := MapSubmission(doc)
	if sub == nil {
		t.Fatal("expected non-nil submission")
	}
	// Single resource identifier: type and ID extracted as resource
	if sub.Type != "people" {
		t.Errorf("Type = %q, want %q", sub.Type, "people")
	}
	if sub.ID != "12" {
		t.Errorf("ID = %q, want %q", sub.ID, "12")
	}
}

func TestMapSubmission_RelationshipOnlyToMany(t *testing.T) {
	body := `{"data":[{"type":"tags","id":"2"},{"type":"tags","id":"3"}]}`
	var doc Document
	json.Unmarshal([]byte(body), &doc)

	sub := MapSubmission(doc)
	if sub == nil {
		t.Fatal("expected non-nil submission")
	}
	if sub.RelData == nil {
		t.Fatal("expected RelData to be set")
	}
	if !sub.RelData.IsMany {
		t.Error("expected IsMany=true")
	}
	if len(sub.RelData.Many) != 2 {
		t.Fatalf("RelData.Many length = %d, want 2", len(sub.RelData.Many))
	}
	if sub.RelData.Many[0].Type != "tags" || sub.RelData.Many[0].ID != "2" {
		t.Errorf("RelData.Many[0] = %+v, want {tags 2}", sub.RelData.Many[0])
	}
}

func TestMapSubmission_ClearToOneRelationship(t *testing.T) {
	body := `{"data":null}`
	var doc Document
	json.Unmarshal([]byte(body), &doc)

	sub := MapSubmission(doc)
	if sub == nil {
		t.Fatal("expected non-nil submission")
	}
	if sub.RelData == nil {
		t.Fatal("expected RelData to be set")
	}
	if !sub.RelData.IsNull {
		t.Error("expected RelData.IsNull=true")
	}
}

func TestMapSubmission_ClearToManyRelationship(t *testing.T) {
	body := `{"data":[]}`
	var doc Document
	json.Unmarshal([]byte(body), &doc)

	sub := MapSubmission(doc)
	if sub == nil {
		t.Fatal("expected non-nil submission")
	}
	if sub.RelData == nil {
		t.Fatal("expected RelData to be set")
	}
	if !sub.RelData.IsMany {
		t.Error("expected RelData.IsMany=true")
	}
	if len(sub.RelData.Many) != 0 {
		t.Errorf("RelData.Many length = %d, want 0", len(sub.RelData.Many))
	}
}

func TestMapSubmission_NilForEmptyDocument(t *testing.T) {
	doc := Document{}
	sub := MapSubmission(doc)
	if sub != nil {
		t.Errorf("expected nil for empty document, got %+v", sub)
	}
}

func TestRelationship_UnmarshalJSON_ToOne(t *testing.T) {
	body := `{"data":{"type":"people","id":"9"}}`
	var rel Relationship
	if err := json.Unmarshal([]byte(body), &rel); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	rid, ok := rel.Data.(ResourceIdentifier)
	if !ok {
		t.Fatalf("Data type = %T, want ResourceIdentifier", rel.Data)
	}
	if rid.Type != "people" || rid.ID != "9" {
		t.Errorf("Data = %+v, want {people 9}", rid)
	}
}

func TestRelationship_UnmarshalJSON_ToMany(t *testing.T) {
	body := `{"data":[{"type":"tags","id":"2"},{"type":"tags","id":"3"}]}`
	var rel Relationship
	if err := json.Unmarshal([]byte(body), &rel); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	rids, ok := rel.Data.([]ResourceIdentifier)
	if !ok {
		t.Fatalf("Data type = %T, want []ResourceIdentifier", rel.Data)
	}
	if len(rids) != 2 {
		t.Fatalf("Data length = %d, want 2", len(rids))
	}
}

func TestRelationship_UnmarshalJSON_Null(t *testing.T) {
	body := `{"data":null}`
	var rel Relationship
	if err := json.Unmarshal([]byte(body), &rel); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if rel.Data != nil {
		t.Errorf("Data = %v, want nil", rel.Data)
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
