// Package methodoverride provides HTTP middleware that reads a _method hidden
// form field from POST request bodies and overrides r.Method before passing the
// request to the next handler.  This enables HTML forms (which only support GET
// and POST) to submit PUT, PATCH, and DELETE requests.
package methodoverride

import (
	"bytes"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
)

// maxScan is the maximum number of bytes read from the request body when
// searching for the _method field.
const maxScan = 4096

// allowed is the set of HTTP methods that _method may override to.
var allowed = map[string]bool{
	"PUT":    true,
	"PATCH":  true,
	"DELETE": true,
}

// Wrap returns an http.Handler that checks POST requests for a _method
// form field and overrides r.Method accordingly before calling next.
//
// Only PUT, PATCH, and DELETE are accepted as override values.
// The request body remains fully readable by downstream handlers.
func Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			next.ServeHTTP(w, r)
			return
		}

		ct := r.Header.Get("Content-Type")
		mediaType, _, _ := mime.ParseMediaType(ct)

		var method string
		switch mediaType {
		case "application/x-www-form-urlencoded":
			method = scanURLEncoded(r)
		case "multipart/form-data":
			method = scanMultipart(r)
		}

		if method != "" {
			r.Method = method
		}

		next.ServeHTTP(w, r)
	})
}

// scanURLEncoded reads up to maxScan bytes from the body, searches for
// _method=<value>, and reassembles the body.
func scanURLEncoded(r *http.Request) string {
	buf, err := readPrefix(r)
	if err != nil || len(buf) == 0 {
		return ""
	}

	return extractFromURLEncoded(buf)
}

// extractFromURLEncoded looks for _method=VALUE in url-encoded form data.
func extractFromURLEncoded(buf []byte) string {
	s := string(buf)
	for s != "" {
		var pair string
		if i := strings.IndexByte(s, '&'); i >= 0 {
			pair, s = s[:i], s[i+1:]
		} else {
			pair, s = s, ""
		}

		key, value, _ := strings.Cut(pair, "=")
		if decodeComponent(key) == "_method" {
			v := strings.ToUpper(decodeComponent(value))
			if allowed[v] {
				return v
			}
			return ""
		}
	}
	return ""
}

// scanMultipart reads up to maxScan bytes from the body, searches for the
// _method part, and reassembles the body.
func scanMultipart(r *http.Request) string {
	buf, err := readPrefix(r)
	if err != nil || len(buf) == 0 {
		return ""
	}

	return extractFromMultipart(buf)
}

// extractFromMultipart scans raw multipart bytes for name="_method".
func extractFromMultipart(buf []byte) string {
	// Look for the Content-Disposition header that names _method.
	needle := []byte(`name="_method"`)
	idx := bytes.Index(buf, needle)
	if idx < 0 {
		return ""
	}

	// The part value starts after the first \r\n\r\n following the header.
	rest := buf[idx+len(needle):]
	sep := []byte("\r\n\r\n")
	start := bytes.Index(rest, sep)
	if start < 0 {
		return ""
	}
	rest = rest[start+len(sep):]

	// The value ends at the next \r\n (before the boundary).
	end := bytes.Index(rest, []byte("\r\n"))
	if end < 0 {
		end = len(rest)
	}

	v := strings.ToUpper(strings.TrimSpace(string(rest[:end])))
	if allowed[v] {
		return v
	}
	return ""
}

// readPrefix reads up to maxScan bytes from r.Body, then reassembles the body
// so downstream handlers can read it in full.
func readPrefix(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	buf := make([]byte, maxScan)
	n, err := io.ReadFull(r.Body, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return nil, err
	}
	buf = buf[:n]

	// Reassemble: prepend what we read back onto the body.
	r.Body = io.NopCloser(io.MultiReader(bytes.NewReader(buf), r.Body))
	return buf, nil
}

// decodeComponent URL-decodes a single form value component.
func decodeComponent(s string) string {
	v, err := url.QueryUnescape(s)
	if err != nil {
		return s
	}
	return v
}
