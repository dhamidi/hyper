package jsonapi

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/dhamidi/hyper"
)

// PrimaryData holds the primary data of a JSON:API document.
// It can represent a single resource, an array of resources, or null.
type PrimaryData struct {
	// One holds a single resource when IsMany is false and IsNull is false.
	One *Resource
	// Many holds an array of resources when IsMany is true.
	Many []*Resource
	// IsNull indicates that the primary data is explicitly null.
	IsNull bool
	// IsMany indicates that the primary data is a collection (array).
	IsMany bool
}

// Resource returns the singular resource, or nil if this is a collection or null.
func (p PrimaryData) Resource() *Resource {
	if p.IsMany || p.IsNull {
		return nil
	}
	return p.One
}

// Resources returns all resources. For singular data, returns a single-element slice.
func (p PrimaryData) Resources() []*Resource {
	if p.IsNull {
		return nil
	}
	if p.IsMany {
		return p.Many
	}
	if p.One != nil {
		return []*Resource{p.One}
	}
	return nil
}

// MarshalJSON implements json.Marshaler.
func (p PrimaryData) MarshalJSON() ([]byte, error) {
	if p.IsNull {
		return []byte("null"), nil
	}
	if p.IsMany {
		if p.Many == nil {
			return []byte("[]"), nil
		}
		return json.Marshal(p.Many)
	}
	if p.One == nil {
		return []byte("null"), nil
	}
	return json.Marshal(p.One)
}

// UnmarshalJSON implements json.Unmarshaler.
func (p *PrimaryData) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		p.IsNull = true
		return nil
	}
	// Detect array vs object
	for _, b := range data {
		if b == ' ' || b == '\t' || b == '\n' || b == '\r' {
			continue
		}
		if b == '[' {
			p.IsMany = true
			return json.Unmarshal(data, &p.Many)
		}
		break
	}
	p.One = &Resource{}
	return json.Unmarshal(data, p.One)
}

// Document represents a JSON:API top-level document.
type Document struct {
	Data     PrimaryData    `json:"data"`
	Included []*Resource    `json:"included,omitempty"`
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
//
// When rep.State is a hyper.Collection, the document uses array primary data.
// Collection items are taken from embedded representations under a key
// matching rep.Kind; remaining embedded entries go into the included array.
func MapRepresentation(rep hyper.Representation, resolver hyper.Resolver) Document {
	doc := Document{}

	isCollection := isCollectionState(rep.State)

	if isCollection {
		doc.Data = mapCollectionData(rep, resolver)

		// Non-primary embedded go into included
		primaryKey := collectionKey(rep)
		for key, reps := range rep.Embedded {
			if key == primaryKey {
				// Items in the primary collection may themselves have embedded
				// sub-resources that belong in included.
				for _, item := range reps {
					for _, subReps := range item.Embedded {
						for _, sub := range subReps {
							inc := mapResource(sub, resolver)
							doc.Included = append(doc.Included, inc)
						}
					}
				}
				continue
			}
			for _, embedded := range reps {
				inc := mapResource(embedded, resolver)
				doc.Included = append(doc.Included, inc)
			}
		}
	} else {
		res := mapResource(rep, resolver)
		doc.Data = PrimaryData{One: res}

		// Map embedded representations into included array
		for _, reps := range rep.Embedded {
			for _, embedded := range reps {
				inc := mapResource(embedded, resolver)
				doc.Included = append(doc.Included, inc)
			}
		}
	}

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

	return doc
}

// MapNullDocument creates a Document with explicitly null primary data.
func MapNullDocument() Document {
	return Document{
		Data: PrimaryData{IsNull: true},
	}
}

// isCollectionState returns true if the node is a hyper.Collection.
func isCollectionState(n hyper.Node) bool {
	_, ok := n.(hyper.Collection)
	return ok
}

// collectionKey determines which embedded key holds the primary collection items.
// It first looks for a key matching rep.Kind, then "items", then falls back
// to the first (only) embedded key.
func collectionKey(rep hyper.Representation) string {
	if _, ok := rep.Embedded[rep.Kind]; ok {
		return rep.Kind
	}
	if _, ok := rep.Embedded["items"]; ok {
		return "items"
	}
	// Fall back to the sole embedded key
	for k := range rep.Embedded {
		return k
	}
	return ""
}

// mapCollectionData produces array PrimaryData from a collection representation.
func mapCollectionData(rep hyper.Representation, resolver hyper.Resolver) PrimaryData {
	key := collectionKey(rep)
	items := rep.Embedded[key]

	resources := make([]*Resource, len(items))
	for i, item := range items {
		resources[i] = mapResource(item, resolver)
	}
	return PrimaryData{Many: resources, IsMany: true}
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
// suitable for use with hyper.WithValues. Only singular resources are
// supported; collection documents return nil.
func MapToSubmission(doc Document) map[string]any {
	res := doc.Data.Resource()
	if res == nil {
		return nil
	}
	result := make(map[string]any)
	if res.ID != "" {
		result["id"] = res.ID
	}
	for k, v := range res.Attributes {
		result[k] = v
	}
	return result
}
