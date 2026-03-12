package hyper

import (
	"net/url"
	"testing"
)

func TestFindLink_Found(t *testing.T) {
	rep := Representation{
		Links: []Link{
			{Rel: "self", Target: Target{URL: &url.URL{Path: "/self"}}},
			{Rel: "next", Target: Target{URL: &url.URL{Path: "/next"}}},
		},
	}
	link, ok := FindLink(rep, "next")
	if !ok {
		t.Fatal("expected to find link with rel 'next'")
	}
	if link.Target.URL.Path != "/next" {
		t.Errorf("Target.URL.Path = %q, want %q", link.Target.URL.Path, "/next")
	}
}

func TestFindLink_NotFound(t *testing.T) {
	rep := Representation{
		Links: []Link{
			{Rel: "self", Target: Target{URL: &url.URL{Path: "/self"}}},
		},
	}
	_, ok := FindLink(rep, "missing")
	if ok {
		t.Error("expected not to find link with rel 'missing'")
	}
}

func TestFindLink_ReturnsFirstMatch(t *testing.T) {
	rep := Representation{
		Links: []Link{
			{Rel: "item", Target: Target{URL: &url.URL{Path: "/first"}}},
			{Rel: "item", Target: Target{URL: &url.URL{Path: "/second"}}},
		},
	}
	link, ok := FindLink(rep, "item")
	if !ok {
		t.Fatal("expected to find link with rel 'item'")
	}
	if link.Target.URL.Path != "/first" {
		t.Errorf("Target.URL.Path = %q, want %q", link.Target.URL.Path, "/first")
	}
}

func TestFindAction_Found(t *testing.T) {
	rep := Representation{
		Actions: []Action{
			{Name: "create", Rel: "create-item", Method: "POST", Target: Target{URL: &url.URL{Path: "/items"}}},
		},
	}
	action, ok := FindAction(rep, "create-item")
	if !ok {
		t.Fatal("expected to find action with rel 'create-item'")
	}
	if action.Name != "create" {
		t.Errorf("Name = %q, want %q", action.Name, "create")
	}
}

func TestFindAction_NotFound(t *testing.T) {
	rep := Representation{
		Actions: []Action{
			{Name: "create", Rel: "create-item", Method: "POST"},
		},
	}
	_, ok := FindAction(rep, "delete-item")
	if ok {
		t.Error("expected not to find action with rel 'delete-item'")
	}
}

func TestFindEmbedded_Found(t *testing.T) {
	child := Representation{Kind: "item"}
	rep := Representation{
		Embedded: map[string][]Representation{
			"items": {child},
		},
	}
	embedded := FindEmbedded(rep, "items")
	if len(embedded) != 1 {
		t.Fatalf("len(embedded) = %d, want 1", len(embedded))
	}
	if embedded[0].Kind != "item" {
		t.Errorf("Kind = %q, want %q", embedded[0].Kind, "item")
	}
}

func TestFindEmbedded_MissingSlot(t *testing.T) {
	rep := Representation{
		Embedded: map[string][]Representation{
			"items": {{Kind: "item"}},
		},
	}
	embedded := FindEmbedded(rep, "comments")
	if embedded != nil {
		t.Errorf("expected nil, got %v", embedded)
	}
}

func TestFindEmbedded_NilEmbedded(t *testing.T) {
	rep := Representation{}
	embedded := FindEmbedded(rep, "items")
	if embedded != nil {
		t.Errorf("expected nil, got %v", embedded)
	}
}

func TestActionValues_ExtractsValues(t *testing.T) {
	action := Action{
		Fields: []Field{
			{Name: "name", Value: "Alice"},
			{Name: "age", Value: 30},
		},
	}
	vals := ActionValues(action)
	if vals["name"] != "Alice" {
		t.Errorf("vals[name] = %v, want %q", vals["name"], "Alice")
	}
	if vals["age"] != 30 {
		t.Errorf("vals[age] = %v, want %d", vals["age"], 30)
	}
}

func TestActionValues_EmptyWhenNoValues(t *testing.T) {
	action := Action{
		Fields: []Field{
			{Name: "name"},
			{Name: "age"},
		},
	}
	vals := ActionValues(action)
	if len(vals) != 0 {
		t.Errorf("len(vals) = %d, want 0", len(vals))
	}
}

func TestActionValues_SkipsNilValues(t *testing.T) {
	action := Action{
		Fields: []Field{
			{Name: "name", Value: "Alice"},
			{Name: "age"},
			{Name: "email", Value: nil},
		},
	}
	vals := ActionValues(action)
	if len(vals) != 1 {
		t.Errorf("len(vals) = %d, want 1", len(vals))
	}
	if vals["name"] != "Alice" {
		t.Errorf("vals[name] = %v, want %q", vals["name"], "Alice")
	}
}
