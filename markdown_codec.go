package hyper

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
)

// mdRepCodec encodes Representation values as Markdown (§13).
type mdRepCodec struct{}

// MarkdownCodec returns a RepresentationCodec that encodes representations
// as Markdown. Links become [text](url) links, actions become descriptive
// blocks listing fields and their types, and state values are rendered as
// Markdown prose.
//
// Markdown is treated as a read-oriented alternate representation per §13.1.
// Actions are degraded to descriptive prose since Markdown has no native form
// controls. UI-specific hints are omitted.
//
// In RenderDocument mode (the default), the Kind field is rendered as a
// top-level heading. In RenderFragment mode, the heading is omitted and
// only state, links, actions, and embedded sections are rendered.
func MarkdownCodec() RepresentationCodec { return mdRepCodec{} }

func (mdRepCodec) MediaTypes() []string { return []string{"text/markdown"} }

func (c mdRepCodec) Encode(ctx context.Context, w io.Writer, rep Representation, opts EncodeOptions) error {
	if rep.Kind != "" && opts.Mode != RenderFragment {
		if _, err := fmt.Fprintf(w, "# %s\n\n", rep.Kind); err != nil {
			return err
		}
	}

	if rep.State != nil {
		if err := mdWriteState(w, rep.State); err != nil {
			return err
		}
	}

	if len(rep.Links) > 0 {
		if err := mdWriteLinks(ctx, w, rep.Links, opts); err != nil {
			return err
		}
	}

	if len(rep.Actions) > 0 {
		if err := mdWriteActions(ctx, w, rep.Actions, opts); err != nil {
			return err
		}
	}

	if len(rep.Embedded) > 0 {
		if err := mdWriteEmbedded(ctx, w, rep.Embedded, opts); err != nil {
			return err
		}
	}

	if len(rep.Meta) > 0 {
		if err := mdWriteMeta(w, rep.Meta); err != nil {
			return err
		}
	}

	return nil
}

func mdWriteState(w io.Writer, n Node) error {
	switch v := n.(type) {
	case Object:
		return mdWriteObjectState(w, v)
	case Collection:
		return mdWriteCollectionState(w, v)
	default:
		return fmt.Errorf("markdown: unsupported node type %T", n)
	}
}

func mdWriteObjectState(w io.Writer, o Object) error {
	keys := make([]string, 0, len(o))
	for k := range o {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		val := mdFormatValue(o[k])
		if _, err := fmt.Fprintf(w, "- **%s:** %s\n", k, val); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(w, "\n"); err != nil {
		return err
	}
	return nil
}

func mdWriteCollectionState(w io.Writer, c Collection) error {
	for i, v := range c {
		val := mdFormatValue(v)
		if _, err := fmt.Fprintf(w, "%d. %s\n", i+1, val); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(w, "\n"); err != nil {
		return err
	}
	return nil
}

func mdFormatValue(v Value) string {
	switch val := v.(type) {
	case Scalar:
		return fmt.Sprintf("%v", val.V)
	case RichText:
		if val.MediaType == "text/markdown" {
			return val.Source
		}
		lang := val.MediaType
		return fmt.Sprintf("\n```%s\n%s\n```", lang, val.Source)
	default:
		return ""
	}
}

func mdWriteLinks(ctx context.Context, w io.Writer, links []Link, opts EncodeOptions) error {
	if _, err := io.WriteString(w, "## Links\n\n"); err != nil {
		return err
	}
	for _, l := range links {
		href, err := resolveTarget(ctx, l.Target, opts.Resolver)
		if err != nil {
			return fmt.Errorf("markdown: resolve link %q: %w", l.Rel, err)
		}
		label := l.Title
		if label == "" {
			label = l.Rel
		}
		if _, err := fmt.Fprintf(w, "- [%s](%s) (rel: %s)\n", label, href, l.Rel); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(w, "\n"); err != nil {
		return err
	}
	return nil
}

func mdWriteActions(ctx context.Context, w io.Writer, actions []Action, opts EncodeOptions) error {
	for _, a := range actions {
		href, err := resolveTarget(ctx, a.Target, opts.Resolver)
		if err != nil {
			return fmt.Errorf("markdown: resolve action %q: %w", a.Name, err)
		}

		method := a.Method
		if method == "" {
			method = "POST"
		}

		if _, err := fmt.Fprintf(w, "### %s (%s %s)\n\n", a.Name, method, href); err != nil {
			return err
		}

		for _, f := range a.Fields {
			if err := mdWriteField(w, f); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(w, "\n"); err != nil {
			return err
		}
	}
	return nil
}

func mdWriteField(w io.Writer, f Field) error {
	fieldType := f.Type
	if fieldType == "" {
		fieldType = "text"
	}

	var attrs []string
	attrs = append(attrs, fieldType)
	if f.Required {
		attrs = append(attrs, "required")
	}
	if f.ReadOnly {
		attrs = append(attrs, "read-only")
	}

	desc := strings.Join(attrs, ", ")
	if f.Value != nil {
		if _, err := fmt.Fprintf(w, "- %s (%s): %q\n", f.Name, desc, fmt.Sprintf("%v", f.Value)); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintf(w, "- %s (%s)\n", f.Name, desc); err != nil {
			return err
		}
	}

	if len(f.Options) > 0 {
		for _, o := range f.Options {
			optLabel := o.Label
			if optLabel == "" {
				optLabel = o.Value
			}
			marker := " "
			if o.Selected {
				marker = "x"
			}
			if _, err := fmt.Fprintf(w, "  - [%s] %s\n", marker, optLabel); err != nil {
				return err
			}
		}
	}

	if f.Help != "" {
		if _, err := fmt.Fprintf(w, "  - *%s*\n", f.Help); err != nil {
			return err
		}
	}

	return nil
}

func mdWriteEmbedded(ctx context.Context, w io.Writer, embedded map[string][]Representation, opts EncodeOptions) error {
	slots := make([]string, 0, len(embedded))
	for slot := range embedded {
		slots = append(slots, slot)
	}
	sort.Strings(slots)
	for _, slot := range slots {
		reps := embedded[slot]
		if _, err := fmt.Fprintf(w, "## %s\n\n", slot); err != nil {
			return err
		}
		for _, r := range reps {
			if r.Kind != "" {
				if _, err := fmt.Fprintf(w, "### %s\n\n", r.Kind); err != nil {
					return err
				}
			}
			if r.State != nil {
				if err := mdWriteState(w, r.State); err != nil {
					return err
				}
			}
			if len(r.Links) > 0 {
				if err := mdWriteLinks(ctx, w, r.Links, opts); err != nil {
					return err
				}
			}
			if len(r.Actions) > 0 {
				if err := mdWriteActions(ctx, w, r.Actions, opts); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func mdWriteMeta(w io.Writer, meta map[string]any) error {
	if _, err := io.WriteString(w, "## Meta\n\n"); err != nil {
		return err
	}
	keys := make([]string, 0, len(meta))
	for k := range meta {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if _, err := fmt.Fprintf(w, "- **%s:** %v\n", k, meta[k]); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(w, "\n"); err != nil {
		return err
	}
	return nil
}
