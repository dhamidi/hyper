package methodoverride

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// handler records the method and body seen by the downstream handler.
func echoHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		fmt.Fprintf(w, "method=%s body=%s", r.Method, body)
	}
}

func postRequest(contentType, body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	r.Header.Set("Content-Type", contentType)
	return r
}

func TestPOSTWithMethodPUT(t *testing.T) {
	h := Wrap(echoHandler())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, postRequest("application/x-www-form-urlencoded", "_method=PUT&title=hello"))
	if got := w.Body.String(); !strings.Contains(got, "method=PUT") {
		t.Fatalf("expected PUT, got: %s", got)
	}
}

func TestPOSTWithMethodDELETE(t *testing.T) {
	h := Wrap(echoHandler())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, postRequest("application/x-www-form-urlencoded", "_method=DELETE"))
	if got := w.Body.String(); !strings.Contains(got, "method=DELETE") {
		t.Fatalf("expected DELETE, got: %s", got)
	}
}

func TestPOSTWithMethodPATCH(t *testing.T) {
	h := Wrap(echoHandler())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, postRequest("application/x-www-form-urlencoded", "_method=PATCH"))
	if got := w.Body.String(); !strings.Contains(got, "method=PATCH") {
		t.Fatalf("expected PATCH, got: %s", got)
	}
}

func TestPOSTWithLowercaseMethod(t *testing.T) {
	h := Wrap(echoHandler())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, postRequest("application/x-www-form-urlencoded", "_method=put"))
	if got := w.Body.String(); !strings.Contains(got, "method=PUT") {
		t.Fatalf("expected PUT, got: %s", got)
	}
}

func TestPOSTWithInvalidOverrideGET(t *testing.T) {
	h := Wrap(echoHandler())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, postRequest("application/x-www-form-urlencoded", "_method=GET"))
	if got := w.Body.String(); !strings.Contains(got, "method=POST") {
		t.Fatalf("expected POST, got: %s", got)
	}
}

func TestPOSTWithoutMethodField(t *testing.T) {
	h := Wrap(echoHandler())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, postRequest("application/x-www-form-urlencoded", "title=hello&body=world"))
	if got := w.Body.String(); !strings.Contains(got, "method=POST") {
		t.Fatalf("expected POST, got: %s", got)
	}
}

func TestGETNotIntercepted(t *testing.T) {
	h := Wrap(echoHandler())
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/?_method=PUT", nil)
	h.ServeHTTP(w, r)
	if got := w.Body.String(); !strings.Contains(got, "method=GET") {
		t.Fatalf("expected GET, got: %s", got)
	}
}

func TestBodyPreservedAfterOverride(t *testing.T) {
	h := Wrap(echoHandler())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, postRequest("application/x-www-form-urlencoded", "_method=DELETE&id=42&confirm=yes"))
	got := w.Body.String()
	if !strings.Contains(got, "method=DELETE") {
		t.Fatalf("expected DELETE, got: %s", got)
	}
	if !strings.Contains(got, "_method=DELETE&id=42&confirm=yes") {
		t.Fatalf("body not preserved, got: %s", got)
	}
}

func TestMultipartWithMethodPUT(t *testing.T) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	writer.WriteField("_method", "PUT")
	writer.WriteField("title", "hello")
	writer.Close()

	r := httptest.NewRequest(http.MethodPost, "/", &buf)
	r.Header.Set("Content-Type", writer.FormDataContentType())

	h := Wrap(echoHandler())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if got := w.Body.String(); !strings.Contains(got, "method=PUT") {
		t.Fatalf("expected PUT, got: %s", got)
	}
}

func TestMethodFieldBeyondScanLimit(t *testing.T) {
	// Place _method after 4KB+ of padding so it is beyond the scan window.
	padding := strings.Repeat("x", maxScan+100)
	body := "padding=" + padding + "&_method=DELETE"
	h := Wrap(echoHandler())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, postRequest("application/x-www-form-urlencoded", body))
	if got := w.Body.String(); !strings.Contains(got, "method=POST") {
		t.Fatalf("expected POST (field beyond scan window), got: %s", got)
	}
}

func TestMethodFirstFieldAmongMany(t *testing.T) {
	body := "_method=PUT&a=1&b=2&c=3&d=4"
	h := Wrap(echoHandler())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, postRequest("application/x-www-form-urlencoded", body))
	got := w.Body.String()
	if !strings.Contains(got, "method=PUT") {
		t.Fatalf("expected PUT, got: %s", got)
	}
	if !strings.Contains(got, body) {
		t.Fatalf("body not preserved, got: %s", got)
	}
}
