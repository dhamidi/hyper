package hyper

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// mdRepCodec encodes Representation values as Markdown (§13).
type mdRepCodec struct{}

// MarkdownCodec returns a RepresentationCodec that encodes representations
// as Markdown. Links become [text](url) links, actions become descriptive
// blocks listing fields and their types, and state values are rendered as
// Markdown prose.
func MarkdownCodec() RepresentationCodec { return mdRepCodec{} }

func (mdRepCodec) MediaTypes() []string { return []string{"text/markdown"} }

func (c mdRepCodec) Encode(ctx context.Context, w io.Writer, rep Representation, opts EncodeOptions) error {
	if rep.Kind != "" {
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
	for k, v := range o {
		val := mdFormatValue(v)
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
		return val.Source
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
		if _, err := fmt.Fprintf(w, "- [%s](%s)\n", label, href); err != nil {
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

		if _, err := fmt.Fprintf(w, "## %s\n\n", a.Name); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "- **Endpoint:** `%s %s`\n", method, href); err != nil {
			return err
		}

		if len(a.Fields) > 0 {
			if _, err := io.WriteString(w, "- **Fields:**\n"); err != nil {
				return err
			}
			for _, f := range a.Fields {
				if err := mdWriteField(w, f); err != nil {
					return err
				}
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

	label := f.Label
	if label == "" {
		label = f.Name
	}

	var parts []string
	parts = append(parts, fmt.Sprintf("`%s`", fieldType))
	if f.Required {
		parts = append(parts, "required")
	}
	if f.ReadOnly {
		parts = append(parts, "read-only")
	}
	if f.Value != nil {
		parts = append(parts, fmt.Sprintf("default: `%v`", f.Value))
	}

	attrs := strings.Join(parts, ", ")
	if _, err := fmt.Fprintf(w, "  - **%s** (%s)\n", label, attrs); err != nil {
		return err
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
			if _, err := fmt.Fprintf(w, "    - [%s] %s\n", marker, optLabel); err != nil {
				return err
			}
		}
	}

	if f.Help != "" {
		if _, err := fmt.Fprintf(w, "    - *%s*\n", f.Help); err != nil {
			return err
		}
	}

	return nil
}

func mdWriteEmbedded(ctx context.Context, w io.Writer, embedded map[string][]Representation, opts EncodeOptions) error {
	for slot, reps := range embedded {
		if _, err := fmt.Fprintf(w, "## %s\n\n", slot); err != nil {
			return err
		}
		for _, r := range reps {
			// Render embedded representations with kind as h3
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
