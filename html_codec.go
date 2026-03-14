package hyper

import (
	"context"
	"fmt"
	"html/template"
	"io"
	"sort"
	"strings"
)

// htmlRepCodec encodes Representation values as semantic HTML (§12).
type htmlRepCodec struct{}

// HTMLCodec returns a RepresentationCodec that encodes representations
// as semantic HTML. Links become <a> tags, actions become <form> tags
// with input fields, and state values are rendered as definition lists.
//
// Since HTML forms only support GET and POST, actions with other methods
// (PUT, DELETE, PATCH) are rendered as POST forms with a hidden _method
// field containing the original method. Use the methodoverride middleware
// to interpret this field on the server side.
//
// When any field in an action has Type "file", the form element is rendered
// with enctype="multipart/form-data". The Accept field is emitted as the
// accept attribute, Multiple as the multiple boolean attribute, and MaxSize
// as a data-max-size attribute for client-side validation scripts.
//
// The codec interprets Hints on both Representation and Action values.
// String-valued hints are emitted as HTML attributes (e.g., "hx-target"
// becomes hx-target="…"). The bool hint "destructive" (true) adds
// class="destructive" to the element. The bool hint "hidden" (true)
// suppresses rendering of the action entirely. All attribute values are
// HTML-escaped to prevent XSS.
func HTMLCodec() RepresentationCodec { return htmlRepCodec{} }

func (htmlRepCodec) MediaTypes() []string { return []string{"text/html"} }

func (c htmlRepCodec) Encode(ctx context.Context, w io.Writer, rep Representation, opts EncodeOptions) error {
	if opts.Mode == RenderFragment {
		return writeFragment(ctx, w, rep, opts)
	}
	if _, err := io.WriteString(w, "<!DOCTYPE html>\n<html>\n<head>"); err != nil {
		return err
	}
	title := rep.Kind
	if title == "" {
		title = "Representation"
	}
	if err := writeEscaped(w, "<title>"+title+"</title>"); err != nil {
		return err
	}
	if _, err := io.WriteString(w, "</head>\n<body>\n"); err != nil {
		return err
	}
	if err := writeFragment(ctx, w, rep, opts); err != nil {
		return err
	}
	_, err := io.WriteString(w, "</body>\n</html>\n")
	return err
}

func writeFragment(ctx context.Context, w io.Writer, rep Representation, opts EncodeOptions) error {
	var articleAttrs strings.Builder
	if rep.Kind != "" {
		escapedKind := template.HTMLEscapeString(rep.Kind)
		fmt.Fprintf(&articleAttrs, " data-kind=%q", escapedKind)
	}
	articleAttrs.WriteString(hintAttrs(rep.Hints))
	if _, err := fmt.Fprintf(w, "<article%s>\n", articleAttrs.String()); err != nil {
		return err
	}

	if rep.Kind != "" {
		escaped := template.HTMLEscapeString(rep.Kind)
		if _, err := fmt.Fprintf(w, "<h1>%s</h1>\n", escaped); err != nil {
			return err
		}
	}

	if rep.State != nil {
		if err := writeState(w, rep.State); err != nil {
			return err
		}
	}

	if len(rep.Links) > 0 {
		if err := writeLinks(ctx, w, rep.Links, opts); err != nil {
			return err
		}
	}

	if len(rep.Actions) > 0 {
		if err := writeActions(ctx, w, rep.Actions, opts); err != nil {
			return err
		}
	}

	if len(rep.Embedded) > 0 {
		if err := writeEmbedded(ctx, w, rep.Embedded, opts); err != nil {
			return err
		}
	}

	_, err := io.WriteString(w, "</article>\n")
	return err
}

func writeState(w io.Writer, n Node) error {
	switch v := n.(type) {
	case Object:
		return writeObjectState(w, v)
	case Collection:
		return writeCollectionState(w, v)
	default:
		return fmt.Errorf("html: unsupported node type %T", n)
	}
}

func writeObjectState(w io.Writer, o Object) error {
	if _, err := io.WriteString(w, "<dl>\n"); err != nil {
		return err
	}
	for k, v := range o {
		escaped := template.HTMLEscapeString(k)
		if _, err := fmt.Fprintf(w, "<dt>%s</dt>\n", escaped); err != nil {
			return err
		}
		if err := writeValueDD(w, v); err != nil {
			return err
		}
	}
	_, err := io.WriteString(w, "</dl>\n")
	return err
}

func writeValueDD(w io.Writer, v Value) error {
	switch val := v.(type) {
	case Scalar:
		escaped := template.HTMLEscapeString(fmt.Sprintf("%v", val.V))
		_, err := fmt.Fprintf(w, "<dd>%s</dd>\n", escaped)
		return err
	case RichText:
		if val.MediaType == "text/html" {
			// Trust HTML content from RichText
			_, err := fmt.Fprintf(w, "<dd>%s</dd>\n", val.Source)
			return err
		}
		escaped := template.HTMLEscapeString(val.Source)
		_, err := fmt.Fprintf(w, "<dd><pre>%s</pre></dd>\n", escaped)
		return err
	default:
		return fmt.Errorf("html: unsupported value type %T", v)
	}
}

func writeCollectionState(w io.Writer, c Collection) error {
	if _, err := io.WriteString(w, "<ol>\n"); err != nil {
		return err
	}
	for _, v := range c {
		switch val := v.(type) {
		case Scalar:
			escaped := template.HTMLEscapeString(fmt.Sprintf("%v", val.V))
			if _, err := fmt.Fprintf(w, "<li>%s</li>\n", escaped); err != nil {
				return err
			}
		case RichText:
			escaped := template.HTMLEscapeString(val.Source)
			if _, err := fmt.Fprintf(w, "<li><pre>%s</pre></li>\n", escaped); err != nil {
				return err
			}
		default:
			return fmt.Errorf("html: unsupported value type %T", v)
		}
	}
	_, err := io.WriteString(w, "</ol>\n")
	return err
}

func writeLinks(ctx context.Context, w io.Writer, links []Link, opts EncodeOptions) error {
	if _, err := io.WriteString(w, "<nav>\n"); err != nil {
		return err
	}
	for _, l := range links {
		href, err := resolveTarget(ctx, l.Target, opts.Resolver)
		if err != nil {
			return fmt.Errorf("html: resolve link %q: %w", l.Rel, err)
		}
		label := l.Title
		if label == "" {
			label = l.Rel
		}
		escapedHref := template.HTMLEscapeString(href)
		escapedLabel := template.HTMLEscapeString(label)
		escapedRel := template.HTMLEscapeString(l.Rel)
		if _, err := fmt.Fprintf(w, "<a href=%q rel=%q>%s</a>\n", escapedHref, escapedRel, escapedLabel); err != nil {
			return err
		}
	}
	_, err := io.WriteString(w, "</nav>\n")
	return err
}

func writeActions(ctx context.Context, w io.Writer, actions []Action, opts EncodeOptions) error {
	for _, a := range actions {
		// "hidden" hint: skip rendering entirely
		if hidden, ok := a.Hints["hidden"]; ok {
			if b, ok := hidden.(bool); ok && b {
				continue
			}
		}

		href, err := resolveTarget(ctx, a.Target, opts.Resolver)
		if err != nil {
			return fmt.Errorf("html: resolve action %q: %w", a.Name, err)
		}

		method := a.Method
		if method == "" {
			method = "POST"
		}

		escapedHref := template.HTMLEscapeString(href)
		escapedName := template.HTMLEscapeString(a.Name)

		// HTML forms only support GET and POST. For other methods,
		// use POST with a hidden _method field (method override).
		// The server should use the methodoverride middleware to
		// interpret the _method field.
		formMethod := method
		needsMethodOverride := method != "GET" && method != "POST"
		if needsMethodOverride {
			formMethod = "POST"
		}

		// When any field is a file input, the form must use multipart encoding.
		hasFileField := false
		for _, f := range a.Fields {
			if f.Type == "file" {
				hasFileField = true
				break
			}
		}

		extra := hintAttrs(a.Hints)
		enctype := ""
		if hasFileField {
			enctype = ` enctype="multipart/form-data"`
		}
		if _, err := fmt.Fprintf(w, "<form method=%q action=%q%s%s>\n", formMethod, escapedHref, enctype, extra); err != nil {
			return err
		}
		if needsMethodOverride {
			escapedMethod := template.HTMLEscapeString(method)
			if _, err := fmt.Fprintf(w, "<input type=\"hidden\" name=\"_method\" value=%q>\n", escapedMethod); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w, "<h2>%s</h2>\n", escapedName); err != nil {
			return err
		}

		for _, f := range a.Fields {
			if err := writeField(w, f); err != nil {
				return err
			}
		}

		if _, err := fmt.Fprintf(w, "<button type=\"submit\">%s</button>\n", escapedName); err != nil {
			return err
		}
		if _, err := io.WriteString(w, "</form>\n"); err != nil {
			return err
		}
	}
	return nil
}

// hintAttrs converts a Hints map into a string of HTML attributes.
// String values are emitted as key="escaped-value".
// The bool hint "destructive" (true) emits class="destructive".
// The "hidden" hint is handled by the caller and skipped here.
// Non-string, non-bool hints are silently skipped.
func hintAttrs(hints map[string]any) string {
	if len(hints) == 0 {
		return ""
	}

	// Sort keys for deterministic output
	keys := make([]string, 0, len(hints))
	for k := range hints {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for _, k := range keys {
		v := hints[k]
		switch val := v.(type) {
		case string:
			fmt.Fprintf(&b, " %s=%q", template.HTMLEscapeString(k), template.HTMLEscapeString(val))
		case bool:
			if k == "destructive" && val {
				b.WriteString(` class="destructive"`)
			}
			// other bool hints silently skipped
		default:
			// non-string, non-bool silently skipped
		}
	}
	return b.String()
}

func writeField(w io.Writer, f Field) error {
	escapedName := template.HTMLEscapeString(f.Name)

	if f.Type == "hidden" {
		val := ""
		if f.Value != nil {
			val = fmt.Sprintf("%v", f.Value)
		}
		_, err := fmt.Fprintf(w, "<input type=\"hidden\" name=%q value=%q>\n",
			escapedName, template.HTMLEscapeString(val))
		return err
	}

	// Label
	label := f.Label
	if label == "" {
		label = f.Name
	}
	escapedLabel := template.HTMLEscapeString(label)

	if _, err := fmt.Fprintf(w, "<label>%s\n", escapedLabel); err != nil {
		return err
	}

	if f.Type == "select" || len(f.Options) > 0 {
		if err := writeSelectField(w, f, escapedName); err != nil {
			return err
		}
	} else if f.Type == "textarea" {
		if err := writeTextareaField(w, f, escapedName); err != nil {
			return err
		}
	} else {
		if err := writeInputField(w, f, escapedName); err != nil {
			return err
		}
	}

	if f.Help != "" {
		escapedHelp := template.HTMLEscapeString(f.Help)
		if _, err := fmt.Fprintf(w, "<small>%s</small>\n", escapedHelp); err != nil {
			return err
		}
	}

	if f.Error != "" {
		escapedError := template.HTMLEscapeString(f.Error)
		if _, err := fmt.Fprintf(w, "<em>%s</em>\n", escapedError); err != nil {
			return err
		}
	}

	_, err := io.WriteString(w, "</label>\n")
	return err
}

func writeInputField(w io.Writer, f Field, escapedName string) error {
	inputType := f.Type
	if inputType == "" {
		inputType = "text"
	}

	var attrs strings.Builder
	fmt.Fprintf(&attrs, "type=%q name=%q", inputType, escapedName)

	if f.Value != nil {
		fmt.Fprintf(&attrs, " value=%q", template.HTMLEscapeString(fmt.Sprintf("%v", f.Value)))
	}
	if f.Required {
		attrs.WriteString(" required")
	}
	if f.ReadOnly {
		attrs.WriteString(" readonly")
	}
	if f.Accept != "" {
		fmt.Fprintf(&attrs, " accept=%q", template.HTMLEscapeString(f.Accept))
	}
	if f.Multiple {
		attrs.WriteString(" multiple")
	}
	if f.MaxSize > 0 {
		fmt.Fprintf(&attrs, " data-max-size=%q", fmt.Sprintf("%d", f.MaxSize))
	}

	_, err := fmt.Fprintf(w, "<input %s>\n", attrs.String())
	return err
}

func writeTextareaField(w io.Writer, f Field, escapedName string) error {
	var attrs strings.Builder
	fmt.Fprintf(&attrs, "name=%q", escapedName)
	if f.Required {
		attrs.WriteString(" required")
	}
	if f.ReadOnly {
		attrs.WriteString(" readonly")
	}

	val := ""
	if f.Value != nil {
		val = template.HTMLEscapeString(fmt.Sprintf("%v", f.Value))
	}
	_, err := fmt.Fprintf(w, "<textarea %s>%s</textarea>\n", attrs.String(), val)
	return err
}

func writeSelectField(w io.Writer, f Field, escapedName string) error {
	var attrs strings.Builder
	fmt.Fprintf(&attrs, "name=%q", escapedName)
	if f.Required {
		attrs.WriteString(" required")
	}
	if f.Multiple {
		attrs.WriteString(" multiple")
	}

	if _, err := fmt.Fprintf(w, "<select %s>\n", attrs.String()); err != nil {
		return err
	}
	for _, o := range f.Options {
		escapedValue := template.HTMLEscapeString(o.Value)
		escapedOptLabel := template.HTMLEscapeString(o.Label)
		if escapedOptLabel == "" {
			escapedOptLabel = escapedValue
		}
		if o.Selected {
			if _, err := fmt.Fprintf(w, "<option value=%q selected>%s</option>\n", escapedValue, escapedOptLabel); err != nil {
				return err
			}
		} else {
			if _, err := fmt.Fprintf(w, "<option value=%q>%s</option>\n", escapedValue, escapedOptLabel); err != nil {
				return err
			}
		}
	}
	_, err := io.WriteString(w, "</select>\n")
	return err
}

func writeEmbedded(ctx context.Context, w io.Writer, embedded map[string][]Representation, opts EncodeOptions) error {
	for slot, reps := range embedded {
		escapedSlot := template.HTMLEscapeString(slot)
		if _, err := fmt.Fprintf(w, "<section data-slot=%q>\n", escapedSlot); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "<h2>%s</h2>\n", escapedSlot); err != nil {
			return err
		}
		for _, r := range reps {
			if err := writeFragment(ctx, w, r, opts); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(w, "</section>\n"); err != nil {
			return err
		}
	}
	return nil
}

func writeEscaped(w io.Writer, s string) error {
	_, err := io.WriteString(w, s)
	return err
}
