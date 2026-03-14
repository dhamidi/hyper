package hyper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
)

// jsonRepCodec encodes Representation values as native JSON (§14.3).
type jsonRepCodec struct{}

// JSONCodec returns a RepresentationCodec that encodes representations
// using the native hyper JSON wire format (§14.3).
func JSONCodec() RepresentationCodec { return jsonRepCodec{} }

func (jsonRepCodec) MediaTypes() []string { return []string{"application/json"} }

func (c jsonRepCodec) DecodeRepresentation(_ context.Context, r io.Reader) (Representation, error) {
	var raw map[string]any
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return Representation{}, fmt.Errorf("json: decode representation: %w", err)
	}
	return decodeRepresentation(raw)
}

func (c jsonRepCodec) Encode(ctx context.Context, w io.Writer, rep Representation, opts EncodeOptions) error {
	out, err := encodeRepresentation(ctx, rep, opts)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(out)
}

func encodeRepresentation(ctx context.Context, rep Representation, opts EncodeOptions) (map[string]any, error) {
	out := make(map[string]any)

	if rep.Kind != "" {
		out["kind"] = rep.Kind
	}

	if rep.Self != nil {
		href, err := resolveTarget(ctx, *rep.Self, opts.Resolver)
		if err != nil {
			return nil, fmt.Errorf("json: resolve self: %w", err)
		}
		out["self"] = map[string]any{"href": href}
	}

	if rep.State != nil {
		encoded, err := encodeNode(rep.State)
		if err != nil {
			return nil, fmt.Errorf("json: encode state: %w", err)
		}
		out["state"] = encoded
	}

	if len(rep.Links) > 0 {
		links, err := encodeLinks(ctx, rep.Links, opts)
		if err != nil {
			return nil, err
		}
		out["links"] = links
	}

	if len(rep.Actions) > 0 {
		actions, err := encodeActions(ctx, rep.Actions, opts)
		if err != nil {
			return nil, err
		}
		out["actions"] = actions
	}

	if len(rep.Embedded) > 0 {
		embedded, err := encodeEmbedded(ctx, rep.Embedded, opts)
		if err != nil {
			return nil, err
		}
		out["embedded"] = embedded
	}

	if len(rep.Meta) > 0 {
		out["meta"] = rep.Meta
	}

	if len(rep.Hints) > 0 {
		out["hints"] = rep.Hints
	}

	return out, nil
}

func resolveTarget(ctx context.Context, t Target, r Resolver) (string, error) {
	if r != nil {
		u, err := r.ResolveTarget(ctx, t)
		if err != nil {
			return "", err
		}
		return u.String(), nil
	}
	if t.URL != nil {
		return t.URL.String(), nil
	}
	return "", nil
}

func encodeNode(n Node) (any, error) {
	switch v := n.(type) {
	case Object:
		return encodeObject(v)
	case Collection:
		return encodeCollection(v)
	default:
		return nil, fmt.Errorf("json: unsupported node type %T", n)
	}
}

func encodeObject(o Object) (map[string]any, error) {
	out := make(map[string]any, len(o))
	for k, v := range o {
		encoded, err := encodeValue(v)
		if err != nil {
			return nil, err
		}
		out[k] = encoded
	}
	return out, nil
}

func encodeCollection(c Collection) ([]any, error) {
	out := make([]any, len(c))
	for i, v := range c {
		encoded, err := encodeValue(v)
		if err != nil {
			return nil, err
		}
		out[i] = encoded
	}
	return out, nil
}

func encodeValue(v Value) (any, error) {
	switch val := v.(type) {
	case Scalar:
		return val.V, nil
	case RichText:
		return map[string]any{
			"_type":     "richtext",
			"mediaType": val.MediaType,
			"source":    val.Source,
		}, nil
	default:
		return nil, fmt.Errorf("json: unsupported value type %T", v)
	}
}

func encodeLinks(ctx context.Context, links []Link, opts EncodeOptions) ([]map[string]any, error) {
	out := make([]map[string]any, len(links))
	for i, l := range links {
		href, err := resolveTarget(ctx, l.Target, opts.Resolver)
		if err != nil {
			return nil, fmt.Errorf("json: resolve link %q: %w", l.Rel, err)
		}
		m := map[string]any{
			"rel":  l.Rel,
			"href": href,
		}
		if l.Title != "" {
			m["title"] = l.Title
		}
		if l.Type != "" {
			m["type"] = l.Type
		}
		out[i] = m
	}
	return out, nil
}

func encodeActions(ctx context.Context, actions []Action, opts EncodeOptions) ([]map[string]any, error) {
	out := make([]map[string]any, len(actions))
	for i, a := range actions {
		href, err := resolveTarget(ctx, a.Target, opts.Resolver)
		if err != nil {
			return nil, fmt.Errorf("json: resolve action %q: %w", a.Name, err)
		}
		m := map[string]any{
			"name": a.Name,
		}
		if a.Rel != "" {
			m["rel"] = a.Rel
		}
		if a.Method != "" {
			m["method"] = a.Method
		}
		if href != "" {
			m["href"] = href
		}
		if len(a.Consumes) > 0 {
			m["consumes"] = a.Consumes
		}
		if len(a.Produces) > 0 {
			m["produces"] = a.Produces
		}
		if len(a.Fields) > 0 {
			m["fields"] = encodeFields(a.Fields)
		}
		if len(a.Hints) > 0 {
			m["hints"] = a.Hints
		}
		out[i] = m
	}
	return out, nil
}

func encodeFields(fields []Field) []map[string]any {
	out := make([]map[string]any, len(fields))
	for i, f := range fields {
		m := map[string]any{
			"name": f.Name,
		}
		if f.Type != "" {
			m["type"] = f.Type
		}
		if f.Value != nil {
			m["value"] = f.Value
		}
		if f.Required {
			m["required"] = true
		}
		if f.ReadOnly {
			m["readOnly"] = true
		}
		if f.Label != "" {
			m["label"] = f.Label
		}
		if f.Help != "" {
			m["help"] = f.Help
		}
		if len(f.Options) > 0 {
			opts := make([]map[string]any, len(f.Options))
			for j, o := range f.Options {
				om := map[string]any{
					"value": o.Value,
				}
				if o.Label != "" {
					om["label"] = o.Label
				}
				if o.Selected {
					om["selected"] = true
				}
				opts[j] = om
			}
			m["options"] = opts
		}
		if f.Error != "" {
			m["error"] = f.Error
		}
		if f.Accept != "" {
			m["accept"] = f.Accept
		}
		if f.MaxSize > 0 {
			m["maxSize"] = f.MaxSize
		}
		if f.Multiple {
			m["multiple"] = true
		}
		out[i] = m
	}
	return out
}

func encodeEmbedded(ctx context.Context, embedded map[string][]Representation, opts EncodeOptions) (map[string]any, error) {
	out := make(map[string]any, len(embedded))
	for slot, reps := range embedded {
		encoded := make([]map[string]any, len(reps))
		for i, r := range reps {
			m, err := encodeRepresentation(ctx, r, opts)
			if err != nil {
				return nil, fmt.Errorf("json: encode embedded %q[%d]: %w", slot, i, err)
			}
			encoded[i] = m
		}
		out[slot] = encoded
	}
	return out, nil
}

// jsonSubCodec decodes JSON request bodies into map[string]any targets.
type jsonSubCodec struct{}

// JSONSubmissionCodec returns a SubmissionCodec that decodes JSON request
// bodies into map[string]any targets.
func JSONSubmissionCodec() SubmissionCodec { return jsonSubCodec{} }

func (jsonSubCodec) MediaTypes() []string { return []string{"application/json"} }

func (jsonSubCodec) Encode(values map[string]any) (io.Reader, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(values); err != nil {
		return nil, fmt.Errorf("json: %w", err)
	}
	return &buf, nil
}

func (jsonSubCodec) Decode(_ context.Context, r io.Reader, dst any, _ DecodeOptions) error {
	switch target := dst.(type) {
	case *map[string]any:
		if err := json.NewDecoder(r).Decode(target); err != nil {
			return fmt.Errorf("json: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("json: Decode target must be *map[string]any, got %T", dst)
	}
}
