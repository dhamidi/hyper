package hyper

import (
	"fmt"
	"net/url"
	"strings"
)

// Path constructs a Target from path segments.
// Each segment is path-escaped individually.
//
//	hyper.Path("contacts", "42")        → /contacts/42
//	hyper.Path()                        → /
func Path(segments ...string) Target {
	if len(segments) == 0 {
		return Target{URL: &url.URL{Path: "/"}}
	}
	escaped := make([]string, len(segments))
	for i, s := range segments {
		escaped[i] = url.PathEscape(s)
	}
	return Target{URL: &url.URL{Path: "/" + strings.Join(escaped, "/")}}
}

// Pathf constructs a Target from a format string.
// The format string is processed with fmt.Sprintf, then parsed as a URL path.
//
//	hyper.Pathf("/contacts/%d", 42) → /contacts/42
func Pathf(format string, args ...any) Target {
	raw := fmt.Sprintf(format, args...)
	u, err := url.Parse(raw)
	if err != nil {
		panic(fmt.Sprintf("hyper.Pathf: invalid URL %q: %v", raw, err))
	}
	return Target{URL: u}
}

// ParseTarget parses a raw URL string into a Target.
// Returns an error if the URL is malformed.
func ParseTarget(rawURL string) (Target, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return Target{}, fmt.Errorf("hyper.ParseTarget: %w", err)
	}
	return Target{URL: u}, nil
}

// MustParseTarget is like ParseTarget but panics on error.
// Suitable for static URLs known at compile time.
func MustParseTarget(rawURL string) Target {
	t, err := ParseTarget(rawURL)
	if err != nil {
		panic(err)
	}
	return t
}

// Route constructs a route-based Target from a route name and alternating
// key-value path parameter pairs.
// Panics if len(params) is odd.
//
//	hyper.Route("contacts.show", "id", "42")
//	hyper.Route("contacts.list")
func Route(name string, params ...string) Target {
	if len(params)%2 != 0 {
		panic("hyper.Route: odd number of params")
	}
	ref := &RouteRef{Name: name}
	if len(params) > 0 {
		ref.Params = make(map[string]string, len(params)/2)
		for i := 0; i < len(params); i += 2 {
			ref.Params[params[i]] = params[i+1]
		}
	}
	return Target{Route: ref}
}

// WithQuery returns a copy of the Target with the given query parameters.
func (t Target) WithQuery(q url.Values) Target {
	t.Query = q
	return t
}

// Ptr returns a pointer to the Target.
func (t Target) Ptr() *Target {
	return &t
}
