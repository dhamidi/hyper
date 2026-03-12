package htmlc

import (
	"testing"

	"github.com/dhamidi/hyper"
)

func TestRepresentationToScope_Kind(t *testing.T) {
	rep := hyper.Representation{Kind: "contact"}
	scope := RepresentationToScope(rep)
	if scope["kind"] != "contact" {
		t.Errorf("kind = %v, want %q", scope["kind"], "contact")
	}
}

func TestRepresentationToScope_ScalarState(t *testing.T) {
	rep := hyper.Representation{
		Kind: "contact",
		State: hyper.Object{
			"name":  hyper.Scalar{V: "Alice"},
			"email": hyper.Scalar{V: "alice@example.com"},
		},
	}
	scope := RepresentationToScope(rep)
	if scope["name"] != "Alice" {
		t.Errorf("name = %v, want %q", scope["name"], "Alice")
	}
	if scope["email"] != "alice@example.com" {
		t.Errorf("email = %v, want %q", scope["email"], "alice@example.com")
	}
}

func TestRepresentationToScope_NonScalarStateIgnored(t *testing.T) {
	rep := hyper.Representation{
		Kind: "contact",
		State: hyper.Object{
			"bio": hyper.RichText{MediaType: "text/html", Source: "<p>Hello</p>"},
		},
	}
	scope := RepresentationToScope(rep)
	if _, ok := scope["bio"]; ok {
		t.Error("non-scalar RichText should not be promoted to scope")
	}
}

func TestRepresentationToScope_CollectionState(t *testing.T) {
	rep := hyper.Representation{
		Kind:  "list",
		State: hyper.Collection{hyper.Scalar{V: "a"}},
	}
	scope := RepresentationToScope(rep)
	// Collection state should not produce extra top-level keys (only kind).
	if len(scope) != 1 {
		t.Errorf("scope has %d keys, want 1 (kind only)", len(scope))
	}
}

func TestRepresentationToScope_Embedded(t *testing.T) {
	rep := hyper.Representation{
		Kind: "contact-list",
		Embedded: map[string][]hyper.Representation{
			"rows": {
				{Kind: "contact-row", State: hyper.Object{"name": hyper.Scalar{V: "Alice"}}},
				{Kind: "contact-row", State: hyper.Object{"name": hyper.Scalar{V: "Bob"}}},
			},
		},
	}
	scope := RepresentationToScope(rep)
	rows, ok := scope["rows"].([]map[string]any)
	if !ok {
		t.Fatalf("rows not []map[string]any, got %T", scope["rows"])
	}
	if len(rows) != 2 {
		t.Fatalf("rows has %d items, want 2", len(rows))
	}
	if rows[0]["name"] != "Alice" {
		t.Errorf("rows[0].name = %v, want %q", rows[0]["name"], "Alice")
	}
	if rows[1]["name"] != "Bob" {
		t.Errorf("rows[1].name = %v, want %q", rows[1]["name"], "Bob")
	}
}

func TestRepresentationToScope_Hints(t *testing.T) {
	rep := hyper.Representation{
		Kind: "progress",
		Hints: map[string]any{
			"hx-trigger": "every 2s",
			"hx-swap":    "outerHTML",
		},
	}
	scope := RepresentationToScope(rep)
	hints, ok := scope["hints"].(map[string]any)
	if !ok {
		t.Fatalf("hints not map[string]any, got %T", scope["hints"])
	}
	if hints["hx-trigger"] != "every 2s" {
		t.Errorf("hints[hx-trigger] = %v, want %q", hints["hx-trigger"], "every 2s")
	}
}

func TestRepresentationToScope_NoHintsWhenEmpty(t *testing.T) {
	rep := hyper.Representation{Kind: "contact"}
	scope := RepresentationToScope(rep)
	if _, ok := scope["hints"]; ok {
		t.Error("hints should not be present when Hints is empty")
	}
}

func TestRepresentationToScope_Actions(t *testing.T) {
	rep := hyper.Representation{
		Kind: "contact",
		Actions: []hyper.Action{
			{
				Name:   "Delete",
				Rel:    "delete",
				Method: "DELETE",
				Hints: map[string]any{
					"hx-confirm": "Are you sure?",
					"hx-target":  "closest tr",
					"hx-swap":    "outerHTML swap:1s",
					"destructive": true,
				},
			},
		},
	}
	scope := RepresentationToScope(rep)

	// Check actions map
	actions, ok := scope["actions"].(map[string]map[string]any)
	if !ok {
		t.Fatalf("actions not map[string]map[string]any, got %T", scope["actions"])
	}
	deleteAction, ok := actions["delete"]
	if !ok {
		t.Fatal("actions[delete] not found")
	}
	if deleteAction["name"] != "Delete" {
		t.Errorf("action.name = %v, want %q", deleteAction["name"], "Delete")
	}
	if deleteAction["rel"] != "delete" {
		t.Errorf("action.rel = %v, want %q", deleteAction["rel"], "delete")
	}
	if deleteAction["method"] != "DELETE" {
		t.Errorf("action.method = %v, want %q", deleteAction["method"], "DELETE")
	}

	// Check hints on action
	hints, ok := deleteAction["hints"].(map[string]any)
	if !ok {
		t.Fatalf("action.hints not map[string]any, got %T", deleteAction["hints"])
	}
	if hints["destructive"] != true {
		t.Error("hints[destructive] should be true")
	}

	// Check hxAttrs — only hx-* prefixed keys
	hxAttrs, ok := deleteAction["hxAttrs"].(map[string]any)
	if !ok {
		t.Fatalf("action.hxAttrs not map[string]any, got %T", deleteAction["hxAttrs"])
	}
	if hxAttrs["hx-confirm"] != "Are you sure?" {
		t.Errorf("hxAttrs[hx-confirm] = %v, want %q", hxAttrs["hx-confirm"], "Are you sure?")
	}
	if hxAttrs["hx-target"] != "closest tr" {
		t.Errorf("hxAttrs[hx-target] = %v, want %q", hxAttrs["hx-target"], "closest tr")
	}
	if hxAttrs["hx-swap"] != "outerHTML swap:1s" {
		t.Errorf("hxAttrs[hx-swap] = %v, want %q", hxAttrs["hx-swap"], "outerHTML swap:1s")
	}
	// Non hx-* hints should NOT appear in hxAttrs
	if _, ok := hxAttrs["destructive"]; ok {
		t.Error("hxAttrs should not contain non-hx-* keys")
	}
}

func TestRepresentationToScope_ActionList(t *testing.T) {
	rep := hyper.Representation{
		Kind: "contact",
		Actions: []hyper.Action{
			{Name: "Edit", Rel: "edit", Method: "GET"},
			{Name: "Delete", Rel: "delete", Method: "DELETE"},
		},
	}
	scope := RepresentationToScope(rep)
	actionList, ok := scope["actionList"].([]map[string]any)
	if !ok {
		t.Fatalf("actionList not []map[string]any, got %T", scope["actionList"])
	}
	if len(actionList) != 2 {
		t.Fatalf("actionList has %d items, want 2", len(actionList))
	}
	// Order should match declaration order
	if actionList[0]["rel"] != "edit" {
		t.Errorf("actionList[0].rel = %v, want %q", actionList[0]["rel"], "edit")
	}
	if actionList[1]["rel"] != "delete" {
		t.Errorf("actionList[1].rel = %v, want %q", actionList[1]["rel"], "delete")
	}
}

func TestRepresentationToScope_ActionWithFields(t *testing.T) {
	rep := hyper.Representation{
		Kind: "contact",
		Actions: []hyper.Action{
			{
				Name:   "Create",
				Rel:    "create",
				Method: "POST",
				Fields: []hyper.Field{
					{Name: "name", Type: "text", Required: true, Label: "Full Name"},
					{Name: "email", Type: "email", Label: "Email"},
				},
			},
		},
	}
	scope := RepresentationToScope(rep)
	actions := scope["actions"].(map[string]map[string]any)
	create := actions["create"]
	fields, ok := create["fields"].([]map[string]any)
	if !ok {
		t.Fatalf("action.fields not []map[string]any, got %T", create["fields"])
	}
	if len(fields) != 2 {
		t.Fatalf("fields has %d items, want 2", len(fields))
	}
	if fields[0]["name"] != "name" {
		t.Errorf("fields[0].name = %v, want %q", fields[0]["name"], "name")
	}
	if fields[0]["required"] != true {
		t.Error("fields[0].required should be true")
	}
	if fields[0]["label"] != "Full Name" {
		t.Errorf("fields[0].label = %v, want %q", fields[0]["label"], "Full Name")
	}
}

func TestRepresentationToScope_ActionNoHxAttrsWithoutHxHints(t *testing.T) {
	rep := hyper.Representation{
		Kind: "contact",
		Actions: []hyper.Action{
			{
				Name:   "Delete",
				Rel:    "delete",
				Method: "DELETE",
				Hints:  map[string]any{"destructive": true},
			},
		},
	}
	scope := RepresentationToScope(rep)
	actions := scope["actions"].(map[string]map[string]any)
	if _, ok := actions["delete"]["hxAttrs"]; ok {
		t.Error("hxAttrs should not be present when no hx-* hints exist")
	}
}

func TestRepresentationToScope_Links(t *testing.T) {
	rep := hyper.Representation{
		Kind: "root",
		Links: []hyper.Link{
			{Rel: "contacts", Title: "Contacts"},
			{Rel: "settings", Title: "Settings"},
		},
	}
	scope := RepresentationToScope(rep)
	links, ok := scope["links"].(map[string]map[string]any)
	if !ok {
		t.Fatalf("links not map[string]map[string]any, got %T", scope["links"])
	}
	if len(links) != 2 {
		t.Fatalf("links has %d entries, want 2", len(links))
	}
	if links["contacts"]["title"] != "Contacts" {
		t.Errorf("links[contacts].title = %v, want %q", links["contacts"]["title"], "Contacts")
	}
	if links["settings"]["rel"] != "settings" {
		t.Errorf("links[settings].rel = %v, want %q", links["settings"]["rel"], "settings")
	}
}

func TestRepresentationToScope_NoActionsWhenEmpty(t *testing.T) {
	rep := hyper.Representation{Kind: "contact"}
	scope := RepresentationToScope(rep)
	if _, ok := scope["actions"]; ok {
		t.Error("actions should not be present when Actions is empty")
	}
	if _, ok := scope["actionList"]; ok {
		t.Error("actionList should not be present when Actions is empty")
	}
}

func TestRepresentationToScope_NoLinksWhenEmpty(t *testing.T) {
	rep := hyper.Representation{Kind: "contact"}
	scope := RepresentationToScope(rep)
	if _, ok := scope["links"]; ok {
		t.Error("links should not be present when Links is empty")
	}
}

func TestFieldsToScope_Basic(t *testing.T) {
	fields := []hyper.Field{
		{Name: "name", Type: "text", Required: true, Label: "Full Name", Help: "Enter your full name"},
		{Name: "email", Type: "email", Value: "alice@example.com"},
	}
	result := FieldsToScope(fields)
	if len(result) != 2 {
		t.Fatalf("result has %d items, want 2", len(result))
	}

	// First field
	f := result[0]
	if f["name"] != "name" {
		t.Errorf("f[0].name = %v, want %q", f["name"], "name")
	}
	if f["type"] != "text" {
		t.Errorf("f[0].type = %v, want %q", f["type"], "text")
	}
	if f["required"] != true {
		t.Error("f[0].required should be true")
	}
	if f["readOnly"] != false {
		t.Error("f[0].readOnly should be false")
	}
	if f["label"] != "Full Name" {
		t.Errorf("f[0].label = %v, want %q", f["label"], "Full Name")
	}
	if f["help"] != "Enter your full name" {
		t.Errorf("f[0].help = %v, want %q", f["help"], "Enter your full name")
	}
	if _, ok := f["value"]; ok {
		t.Error("f[0].value should not be set when Value is nil")
	}

	// Second field
	f = result[1]
	if f["value"] != "alice@example.com" {
		t.Errorf("f[1].value = %v, want %q", f["value"], "alice@example.com")
	}
}

func TestFieldsToScope_ReadOnly(t *testing.T) {
	fields := []hyper.Field{
		{Name: "id", Type: "text", ReadOnly: true},
	}
	result := FieldsToScope(fields)
	if result[0]["readOnly"] != true {
		t.Error("readOnly should be true")
	}
}

func TestFieldsToScope_Error(t *testing.T) {
	fields := []hyper.Field{
		{Name: "email", Type: "email", Error: "invalid email"},
	}
	result := FieldsToScope(fields)
	if result[0]["error"] != "invalid email" {
		t.Errorf("error = %v, want %q", result[0]["error"], "invalid email")
	}
}

func TestFieldsToScope_Options(t *testing.T) {
	fields := []hyper.Field{
		{
			Name: "role",
			Type: "select",
			Options: []hyper.Option{
				{Value: "admin", Label: "Administrator", Selected: false},
				{Value: "user", Label: "User", Selected: true},
			},
		},
	}
	result := FieldsToScope(fields)
	opts, ok := result[0]["options"].([]map[string]any)
	if !ok {
		t.Fatalf("options not []map[string]any, got %T", result[0]["options"])
	}
	if len(opts) != 2 {
		t.Fatalf("options has %d items, want 2", len(opts))
	}
	if opts[0]["value"] != "admin" {
		t.Errorf("opts[0].value = %v, want %q", opts[0]["value"], "admin")
	}
	if opts[0]["label"] != "Administrator" {
		t.Errorf("opts[0].label = %v, want %q", opts[0]["label"], "Administrator")
	}
	if opts[0]["selected"] != false {
		t.Error("opts[0].selected should be false")
	}
	if opts[1]["selected"] != true {
		t.Error("opts[1].selected should be true")
	}
}

func TestFieldsToScope_NoOptionalFieldsWhenEmpty(t *testing.T) {
	fields := []hyper.Field{
		{Name: "q", Type: "text"},
	}
	result := FieldsToScope(fields)
	f := result[0]
	for _, key := range []string{"value", "label", "help", "error", "options"} {
		if _, ok := f[key]; ok {
			t.Errorf("%s should not be present when empty/nil", key)
		}
	}
}

func TestRepresentationToScope_FullContactExample(t *testing.T) {
	// Integration test matching the contacts-hypermedia use case.
	rep := hyper.Representation{
		Kind: "contact-row",
		State: hyper.Object{
			"id":    hyper.Scalar{V: 42},
			"name":  hyper.Scalar{V: "Alice"},
			"email": hyper.Scalar{V: "alice@example.com"},
		},
		Actions: []hyper.Action{
			{
				Name:   "Edit",
				Rel:    "edit",
				Method: "GET",
			},
			{
				Name:   "Delete",
				Rel:    "delete",
				Method: "DELETE",
				Hints: map[string]any{
					"hx-confirm": "Are you sure you want to delete this contact?",
					"hx-target":  "closest tr",
					"hx-swap":    "outerHTML swap:1s",
				},
			},
		},
	}

	scope := RepresentationToScope(rep)

	// Verify state scalars
	if scope["id"] != 42 {
		t.Errorf("id = %v, want 42", scope["id"])
	}
	if scope["name"] != "Alice" {
		t.Errorf("name = %v, want %q", scope["name"], "Alice")
	}

	// Verify actions map
	actions := scope["actions"].(map[string]map[string]any)
	if len(actions) != 2 {
		t.Fatalf("actions has %d entries, want 2", len(actions))
	}

	// Edit action: no hints, no hxAttrs
	edit := actions["edit"]
	if edit["method"] != "GET" {
		t.Errorf("edit.method = %v, want %q", edit["method"], "GET")
	}
	if _, ok := edit["hxAttrs"]; ok {
		t.Error("edit action should not have hxAttrs (no hints)")
	}

	// Delete action: hx-* hints extracted to hxAttrs
	del := actions["delete"]
	hxAttrs := del["hxAttrs"].(map[string]any)
	if hxAttrs["hx-confirm"] != "Are you sure you want to delete this contact?" {
		t.Error("delete hxAttrs missing hx-confirm")
	}

	// Verify actionList preserves order
	actionList := scope["actionList"].([]map[string]any)
	if actionList[0]["rel"] != "edit" || actionList[1]["rel"] != "delete" {
		t.Error("actionList does not preserve declaration order")
	}
}
