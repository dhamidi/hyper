package hyper

// Markdown returns a RichText value with MediaType "text/markdown".
func Markdown(source string) RichText {
	return RichText{MediaType: "text/markdown", Source: source}
}

// PlainText returns a RichText value with MediaType "text/plain".
func PlainText(source string) RichText {
	return RichText{MediaType: "text/plain", Source: source}
}

// StateFrom builds an Object from alternating key-value pairs.
// Values that do not implement the Value interface are automatically
// wrapped in Scalar{V: v}. Values that already implement Value
// (e.g. RichText) are used as-is.
// Panics if len(pairs) is odd or if any key is not a string.
func StateFrom(pairs ...any) Object {
	if len(pairs)%2 != 0 {
		panic("hyper.StateFrom: odd number of arguments")
	}
	obj := make(Object, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		key, ok := pairs[i].(string)
		if !ok {
			panic("hyper.StateFrom: key is not a string")
		}
		if v, ok := pairs[i+1].(Value); ok {
			obj[key] = v
		} else {
			obj[key] = Scalar{V: pairs[i+1]}
		}
	}
	return obj
}

// NewLink creates a Link with the given rel and target.
func NewLink(rel string, target Target) Link {
	return Link{Rel: rel, Target: target}
}

// NewAction creates an Action with the given name, method, and target.
func NewAction(name, method string, target Target) Action {
	return Action{Name: name, Method: method, Target: target}
}

// NewField creates a Field with the given name and type.
func NewField(name, fieldType string) Field {
	return Field{Name: name, Type: fieldType}
}

// WithValues returns a shallow copy of fields with Value populated from the given map.
func WithValues(fields []Field, values map[string]any) []Field {
	result := make([]Field, len(fields))
	copy(result, fields)
	for i, f := range result {
		if v, ok := values[f.Name]; ok {
			result[i].Value = v
		}
	}
	return result
}

// WithErrors returns a shallow copy of fields with Value and Error populated from the given maps.
func WithErrors(fields []Field, values map[string]any, errors map[string]string) []Field {
	result := make([]Field, len(fields))
	copy(result, fields)
	for i, f := range result {
		if v, ok := values[f.Name]; ok {
			result[i].Value = v
		}
		if e, ok := errors[f.Name]; ok {
			result[i].Error = e
		}
	}
	return result
}
