package hyper

import "testing"

// --- test types ---

type fullRepType struct{}

func (fullRepType) HyperRepresentation() Representation {
	return Representation{Kind: "full"}
}

type nodeType struct{}

func (nodeType) HyperNode() Node {
	return Object{"key": Scalar{V: "val"}}
}

type multiType struct{}

func (multiType) HyperNode() Node {
	return Object{"x": Scalar{V: 1}}
}

func (multiType) HyperLinks() []Link {
	return []Link{{Rel: "self"}}
}

func (multiType) HyperActions() []Action {
	return []Action{{Name: "edit"}}
}

func (multiType) HyperEmbedded() map[string][]Representation {
	return map[string][]Representation{
		"items": {{Kind: "child"}},
	}
}

// repAndNode implements both RepresentationProvider and NodeProvider.
type repAndNode struct{}

func (repAndNode) HyperRepresentation() Representation {
	return Representation{Kind: "rep-wins"}
}

func (repAndNode) HyperNode() Node {
	return Object{"ignored": Scalar{V: true}}
}

type plainType struct{ X int }

// --- tests ---

func TestBuildRepresentation_RepresentationProvider(t *testing.T) {
	r := BuildRepresentation(fullRepType{})
	if r.Kind != "full" {
		t.Fatalf("expected Kind \"full\", got %q", r.Kind)
	}
}

func TestBuildRepresentation_NodeProvider(t *testing.T) {
	r := BuildRepresentation(nodeType{})
	obj, ok := r.State.(Object)
	if !ok {
		t.Fatal("expected State to be Object")
	}
	s, ok := obj["key"].(Scalar)
	if !ok || s.V != "val" {
		t.Fatalf("unexpected state: %v", obj)
	}
}

func TestBuildRepresentation_MultipleProviders(t *testing.T) {
	r := BuildRepresentation(multiType{})

	if r.State == nil {
		t.Fatal("expected State to be set")
	}
	if len(r.Links) != 1 || r.Links[0].Rel != "self" {
		t.Fatalf("unexpected Links: %v", r.Links)
	}
	if len(r.Actions) != 1 || r.Actions[0].Name != "edit" {
		t.Fatalf("unexpected Actions: %v", r.Actions)
	}
	items, ok := r.Embedded["items"]
	if !ok || len(items) != 1 || items[0].Kind != "child" {
		t.Fatalf("unexpected Embedded: %v", r.Embedded)
	}
}

func TestBuildRepresentation_RepresentationProviderPrecedence(t *testing.T) {
	r := BuildRepresentation(repAndNode{})
	if r.Kind != "rep-wins" {
		t.Fatalf("expected RepresentationProvider to take precedence, got Kind %q", r.Kind)
	}
	if r.State != nil {
		t.Fatal("expected State to be nil when RepresentationProvider is used")
	}
}

func TestBuildRepresentation_NoProviders(t *testing.T) {
	r := BuildRepresentation(plainType{X: 42})
	if r.Kind != "" || r.State != nil || r.Links != nil || r.Actions != nil || r.Embedded != nil {
		t.Fatalf("expected zero Representation, got %+v", r)
	}
}
