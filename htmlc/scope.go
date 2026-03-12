package htmlc

import (
	"strings"

	"github.com/dhamidi/hyper"
)

// RepresentationToScope converts a hyper.Representation into a map[string]any
// scope suitable for use as template data in htmlc components.
//
// Scalar values from State are promoted to top-level scope keys.
// Embedded representations become nested scope slices keyed by slot name.
// Representation-level Hints are surfaced under "hints".
// Actions are surfaced both as a map keyed by Rel ("actions") and as an
// ordered slice ("actionList"). Each action scope includes computed
// properties for component rendering: "hasFields" (bool), "isGet" (bool),
// and "formMethod" (string — "GET" or "POST"). It also includes an
// "hxAttrs" map containing hx-* prefixed hints for attribute spreading
// via v-bind.
// Links are surfaced as a map keyed by Rel ("links").
//
// Target URLs are NOT resolved by this function — the codec is responsible
// for resolving Action.Target and Link.Target through the Resolver and
// injecting the results (href, hx-{method}) into the scope before rendering.
func RepresentationToScope(r hyper.Representation) map[string]any {
	scope := map[string]any{"kind": r.Kind}

	if obj, ok := r.State.(hyper.Object); ok {
		for k, v := range obj {
			if s, ok := v.(hyper.Scalar); ok {
				scope[k] = s.V
			}
		}
	}

	// Embedded representations become nested scopes for slots.
	for slot, reps := range r.Embedded {
		items := make([]map[string]any, len(reps))
		for i, embedded := range reps {
			items[i] = RepresentationToScope(embedded)
		}
		scope[slot] = items
	}

	// Surface representation-level hints.
	if len(r.Hints) > 0 {
		scope["hints"] = r.Hints
	}

	// Surface actions as structured data keyed by rel.
	// Also build an actionList array for enumeration components.
	if len(r.Actions) > 0 {
		actions := make(map[string]map[string]any, len(r.Actions))
		actionList := make([]map[string]any, 0, len(r.Actions))
		for _, a := range r.Actions {
			hasFields := len(a.Fields) > 0
			isGet := a.Method == "GET"
			formMethod := "POST"
			if isGet {
				formMethod = "GET"
			}
			actionScope := map[string]any{
				"name":       a.Name,
				"rel":        a.Rel,
				"method":     a.Method,
				"hasFields":  hasFields,
				"isGet":      isGet,
				"formMethod": formMethod,
			}
			if len(a.Hints) > 0 {
				actionScope["hints"] = a.Hints
				// Flatten hx-* hints for direct attribute spreading.
				hxAttrs := make(map[string]any)
				for k, v := range a.Hints {
					if strings.HasPrefix(k, "hx-") {
						hxAttrs[k] = v
					}
				}
				if len(hxAttrs) > 0 {
					actionScope["hxAttrs"] = hxAttrs
				}
			}
			if len(a.Fields) > 0 {
				actionScope["fields"] = FieldsToScope(a.Fields)
			}
			actions[a.Rel] = actionScope
			actionList = append(actionList, actionScope)
		}
		scope["actions"] = actions
		scope["actionList"] = actionList
	}

	// Surface links as structured data keyed by rel.
	if len(r.Links) > 0 {
		links := make(map[string]map[string]any, len(r.Links))
		for _, l := range r.Links {
			links[l.Rel] = map[string]any{
				"rel":   l.Rel,
				"title": l.Title,
			}
		}
		scope["links"] = links
	}

	return scope
}

// FieldsToScope converts action fields into template-friendly maps.
// Each field is represented as a map with keys: name, type, required,
// readOnly, and optionally value, label, help, error, and options.
func FieldsToScope(fields []hyper.Field) []map[string]any {
	result := make([]map[string]any, len(fields))
	for i, f := range fields {
		m := map[string]any{
			"name":     f.Name,
			"type":     f.Type,
			"required": f.Required,
			"readOnly": f.ReadOnly,
		}
		if f.Value != nil {
			m["value"] = f.Value
		}
		if f.Label != "" {
			m["label"] = f.Label
		}
		if f.Help != "" {
			m["help"] = f.Help
		}
		if f.Error != "" {
			m["error"] = f.Error
		}
		if len(f.Options) > 0 {
			opts := make([]map[string]any, len(f.Options))
			for j, o := range f.Options {
				opts[j] = map[string]any{
					"value":    o.Value,
					"label":    o.Label,
					"selected": o.Selected,
				}
			}
			m["options"] = opts
		}
		result[i] = m
	}
	return result
}
