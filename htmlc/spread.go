package htmlc

import (
	"fmt"
	"html"
	"sort"
	"strings"
)

// SpreadAttributes renders a map of key-value pairs as HTML attributes.
// This supports the v-bind map spreading convention: when v-bind receives
// a map[string]any, each key-value pair becomes an HTML attribute.
//
// Keys are sorted alphabetically for deterministic output. Values are
// HTML-escaped. Boolean true values render the attribute name only (e.g.,
// "disabled"), and boolean false values omit the attribute entirely.
func SpreadAttributes(attrs map[string]any) string {
	if len(attrs) == 0 {
		return ""
	}

	keys := make([]string, 0, len(attrs))
	for k := range attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for _, k := range keys {
		v := attrs[k]
		switch val := v.(type) {
		case bool:
			if val {
				if b.Len() > 0 {
					b.WriteByte(' ')
				}
				b.WriteString(html.EscapeString(k))
			}
			// false: omit entirely
		default:
			if b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(html.EscapeString(k))
			b.WriteString(`="`)
			b.WriteString(html.EscapeString(fmt.Sprint(v)))
			b.WriteByte('"')
		}
	}
	return b.String()
}
