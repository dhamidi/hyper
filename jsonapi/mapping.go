package jsonapi

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/dhamidi/hyper"
)

// Document represents a JSON:API top-level document.
type Document struct {
	Data     *Resource   `json:"data,omitempty"`
	Included []*Resource `json:"included,omitempty"`
	Meta     map[string]any `json:"meta,omitempty"`
}

// Resource represents a JSON:API resource object.
type Resource struct {
	Type          string                  `json:"type"`
	ID            string                  `json:"id,omitempty"`
	Attributes    map[string]any          `json:"attributes,omitempty"`
	Relationships map[string]Relationship `json:"relationships,omitempty"`
	Links         map[string]any          `json:"links,omitempty"`
	Meta          map[string]any          `json:"meta,omitempty"`
}

// Relationship represents a JSON:API relationship object.
type Relationship struct {
	Data  any            `json:"data,omitempty"`
	Links map[string]any `json:"links,omitempty"`
}

// ResourceIdentifier represents a JSON:API resource identifier object.
type ResourceIdentifier struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// MapRepresentation converts a hyper.Representation into a JSON:API Document.
// The resolver is used to resolve Target URLs; if nil, only Targets with
// direct URLs are resolved.
func MapRepresentation(rep hyper.Representation, resolver hyper.Resolver) Document {
	doc := Document{}
	res := mapResource(rep, resolver)
	doc.Data = res

	// Map top-level meta
	if len(rep.Meta) > 0 {
		doc.Meta = copyMap(rep.Meta)
	}

	// Map actions into meta.actions extension
	if len(rep.Actions) > 0 {
		if doc.Meta == nil {
			doc.Meta = make(map[string]any)
		}
		doc.Meta["actions"] = mapActions(rep.Actions, resolver)
	}

	// Map embedded representations into included array
	if len(rep.Embedded) > 0 {
		for _, reps := range rep.Embedded {
			for _, embedded := range reps {
				inc := mapResource(embedded, resolver)
				doc.Included = append(doc.Included, inc)
			}
		}
	}

	return doc
}

// mapResource maps a single hyper.Representation to a JSON:API Resource.
func mapResource(rep hyper.Representation, resolver hyper.Resolver) *Resource {
	res := &Resource{
		Type: rep.Kind,
	}

	// Extract ID and self link
	selfURL := resolveTarget(rep.Self, resolver)
	id, attrs := extractIDAndAttributes(rep.State, selfURL)
	res.ID = id
	if len(attrs) > 0 {
		res.Attributes = attrs
	}

	// Self link
	if selfURL != "" {
		if res.Links == nil {
			res.Links = make(map[string]any)
		}
		res.Links["self"] = selfURL
	}

	// Partition links into plain links vs relationships
	embeddedKeys := make(map[string]bool, len(rep.Embedded))
	for k := range rep.Embedded {
		embeddedKeys[k] = true
	}

	for _, link := range rep.Links {
		href := resolveTarget(&link.Target, resolver)
		if embeddedKeys[link.Rel] {
			// This is a relationship
			if res.Relationships == nil {
				res.Relationships = make(map[string]Relationship)
			}
			rel := res.Relationships[link.Rel]
			if rel.Links == nil {
				rel.Links = make(map[string]any)
			}
			rel.Links["related"] = href
			res.Relationships[link.Rel] = rel
		} else {
			if res.Links == nil {
				res.Links = make(map[string]any)
			}
			res.Links[link.Rel] = href
		}
	}

	// Map embedded into relationships with data
	for key, reps := range rep.Embedded {
		if res.Relationships == nil {
			res.Relationships = make(map[string]Relationship)
		}
		rel := res.Relationships[key]
		if len(reps) == 1 {
			inc := mapResource(reps[0], resolver)
			rel.Data = ResourceIdentifier{Type: inc.Type, ID: inc.ID}
		} else {
			ids := make([]ResourceIdentifier, len(reps))
			for i, r := range reps {
				inc := mapResource(r, resolver)
				ids[i] = ResourceIdentifier{Type: inc.Type, ID: inc.ID}
			}
			rel.Data = ids
		}
		res.Relationships[key] = rel
	}

	// Resource-level meta from rep.Meta
	if len(rep.Meta) > 0 {
		res.Meta = copyMap(rep.Meta)
	}

	return res
}

// extractIDAndAttributes extracts the resource ID and attributes from rep.State.
// ID precedence: State["id"] > last path segment of selfURL > "".
func extractIDAndAttributes(state hyper.Node, selfURL string) (string, map[string]any) {
	attrs := nodeToMap(state)
	if attrs == nil {
		return extractIDFromURL(selfURL), nil
	}

	// Check for "id" in attributes
	if idVal, ok := attrs["id"]; ok {
		id := fmt.Sprintf("%v", idVal)
		delete(attrs, "id")
		return id, attrs
	}

	return extractIDFromURL(selfURL), attrs
}

// extractIDFromURL returns the last non-empty path segment of the URL.
func extractIDFromURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	segments := strings.Split(strings.TrimRight(u.Path, "/"), "/")
	if len(segments) == 0 {
		return ""
	}
	return segments[len(segments)-1]
}

// nodeToMap converts a hyper.Node to a map[string]any. Returns nil for non-Object nodes.
func nodeToMap(n hyper.Node) map[string]any {
	if n == nil {
		return nil
	}
	obj, ok := n.(hyper.Object)
	if !ok {
		return nil
	}
	result := make(map[string]any, len(obj))
	for k, v := range obj {
		result[k] = valueToAny(v)
	}
	return result
}

// valueToAny converts a hyper.Value to a plain Go value.
func valueToAny(v hyper.Value) any {
	switch val := v.(type) {
	case hyper.Scalar:
		return val.V
	case hyper.RichText:
		return map[string]any{
			"mediaType": val.MediaType,
			"source":    val.Source,
		}
	default:
		return nil
	}
}

// mapActions converts hyper actions to a JSON-friendly representation.
func mapActions(actions []hyper.Action, resolver hyper.Resolver) []map[string]any {
	result := make([]map[string]any, len(actions))
	for i, a := range actions {
		action := map[string]any{
			"name":   a.Name,
			"rel":    a.Rel,
			"method": a.Method,
			"href":   resolveTarget(&a.Target, resolver),
		}
		if len(a.Consumes) > 0 {
			action["consumes"] = a.Consumes
		}
		if len(a.Produces) > 0 {
			action["produces"] = a.Produces
		}
		if len(a.Fields) > 0 {
			action["fields"] = mapFields(a.Fields)
		}
		result[i] = action
	}
	return result
}

// mapFields converts hyper fields to a JSON-friendly representation.
func mapFields(fields []hyper.Field) []map[string]any {
	result := make([]map[string]any, len(fields))
	for i, f := range fields {
		field := map[string]any{
			"name":     f.Name,
			"type":     f.Type,
			"required": f.Required,
		}
		if f.Value != nil {
			field["value"] = f.Value
		}
		if f.Label != "" {
			field["label"] = f.Label
		}
		if f.Help != "" {
			field["help"] = f.Help
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
			field["options"] = opts
		}
		result[i] = field
	}
	return result
}

// resolveTarget attempts to resolve a hyper.Target to a URL string.
func resolveTarget(t *hyper.Target, resolver hyper.Resolver) string {
	if t == nil {
		return ""
	}
	if t.URL != nil {
		return t.URL.String()
	}
	if resolver != nil {
		u, err := resolver.ResolveTarget(nil, *t)
		if err == nil && u != nil {
			return u.String()
		}
	}
	return ""
}

// copyMap creates a shallow copy of a map.
func copyMap(m map[string]any) map[string]any {
	result := make(map[string]any, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}

// MapToSubmission extracts field values from a JSON:API request document.
// It reads from data.attributes and returns them as a flat key-value map
// suitable for use with hyper.WithValues.
func MapToSubmission(doc Document) map[string]any {
	if doc.Data == nil {
		return nil
	}
	result := make(map[string]any)
	if doc.Data.ID != "" {
		result["id"] = doc.Data.ID
	}
	for k, v := range doc.Data.Attributes {
		result[k] = v
	}
	return result
}
