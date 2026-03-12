package hyper

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// mockCodec is a test RepresentationCodec that records calls.
type mockCodec struct {
	mediaTypes []string
	encoded    []byte
	lastOpts   EncodeOptions
}

func (m *mockCodec) MediaTypes() []string { return m.mediaTypes }

func (m *mockCodec) Encode(_ context.Context, w io.Writer, _ Representation, opts EncodeOptions) error {
	m.lastOpts = opts
	_, err := w.Write(m.encoded)
	return err
}

func newMockCodec(mediaType string, body string) *mockCodec {
	return &mockCodec{
		mediaTypes: []string{mediaType},
		encoded:    []byte(body),
	}
}

func TestRespond_SelectsCodecBasedOnAccept(t *testing.T) {
	json := newMockCodec("application/json", `{"ok":true}`)
	xml := newMockCodec("application/xml", `<ok/>`)
	r := Renderer{Codecs: []RepresentationCodec{json, xml}}

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept", "application/xml")
	w := httptest.NewRecorder()

	err := r.Respond(w, req, http.StatusOK, Representation{})
	if err != nil {
		t.Fatal(err)
	}
	if w.Body.String() != "<ok/>" {
		t.Fatalf("expected xml body, got %q", w.Body.String())
	}
	if ct := w.Result().Header.Get("Content-Type"); ct != "application/xml" {
		t.Fatalf("expected Content-Type application/xml, got %q", ct)
	}
}

func TestRespond_FallsBackToFirstCodec(t *testing.T) {
	json := newMockCodec("application/json", `{"ok":true}`)
	r := Renderer{Codecs: []RepresentationCodec{json}}

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept", "text/html")
	w := httptest.NewRecorder()

	err := r.Respond(w, req, http.StatusOK, Representation{})
	if err != nil {
		t.Fatal(err)
	}
	if ct := w.Result().Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected fallback to application/json, got %q", ct)
	}
}

func TestRespond_NoAcceptHeader(t *testing.T) {
	json := newMockCodec("application/json", `{"ok":true}`)
	r := Renderer{Codecs: []RepresentationCodec{json}}

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	err := r.Respond(w, req, http.StatusOK, Representation{})
	if err != nil {
		t.Fatal(err)
	}
	if ct := w.Result().Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected application/json, got %q", ct)
	}
}

func TestRespondAs_ForcesMediaType(t *testing.T) {
	json := newMockCodec("application/json", `{"ok":true}`)
	xml := newMockCodec("application/xml", `<ok/>`)
	r := Renderer{Codecs: []RepresentationCodec{json, xml}}

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept", "application/json") // Accept says JSON
	w := httptest.NewRecorder()

	// But we force XML
	err := r.RespondAs(w, req, http.StatusOK, "application/xml", Representation{})
	if err != nil {
		t.Fatal(err)
	}
	if w.Body.String() != "<ok/>" {
		t.Fatalf("expected xml body, got %q", w.Body.String())
	}
	if ct := w.Result().Header.Get("Content-Type"); ct != "application/xml" {
		t.Fatalf("expected Content-Type application/xml, got %q", ct)
	}
}

func TestRespondWithMode_PassesModeToCodec(t *testing.T) {
	codec := newMockCodec("application/json", `{}`)
	r := Renderer{Codecs: []RepresentationCodec{codec}}

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	err := r.RespondWithMode(w, req, http.StatusOK, Representation{}, RenderFragment)
	if err != nil {
		t.Fatal(err)
	}
	if codec.lastOpts.Mode != RenderFragment {
		t.Fatalf("expected RenderFragment mode, got %v", codec.lastOpts.Mode)
	}
}

func TestRespond_SetsContentTypeHeader(t *testing.T) {
	codec := newMockCodec("application/json", `{}`)
	r := Renderer{Codecs: []RepresentationCodec{codec}}

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	r.Respond(w, req, http.StatusOK, Representation{})
	if ct := w.Result().Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", ct)
	}
}

func TestRespond_WritesStatusCode(t *testing.T) {
	codec := newMockCodec("application/json", `{}`)
	r := Renderer{Codecs: []RepresentationCodec{codec}}

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	r.Respond(w, req, http.StatusCreated, Representation{})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", w.Code)
	}
}

func TestRespond_Returns406WhenNoCodecs(t *testing.T) {
	r := Renderer{} // no codecs

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()

	err := r.Respond(w, req, http.StatusOK, Representation{})
	if err == nil {
		t.Fatal("expected error for 406")
	}
	if w.Code != http.StatusNotAcceptable {
		t.Fatalf("expected 406, got %d", w.Code)
	}
}

func TestRespondAs_Returns406ForUnknownMediaType(t *testing.T) {
	json := newMockCodec("application/json", `{}`)
	r := Renderer{Codecs: []RepresentationCodec{json}}

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	err := r.RespondAs(w, req, http.StatusOK, "text/html", Representation{})
	if err == nil {
		t.Fatal("expected error for unknown media type")
	}
	if w.Code != http.StatusNotAcceptable {
		t.Fatalf("expected 406, got %d", w.Code)
	}
}

func TestRespond_QualityValues(t *testing.T) {
	json := newMockCodec("application/json", `{"ok":true}`)
	xml := newMockCodec("application/xml", `<ok/>`)
	r := Renderer{Codecs: []RepresentationCodec{json, xml}}

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept", "application/json;q=0.5, application/xml;q=0.9")
	w := httptest.NewRecorder()

	err := r.Respond(w, req, http.StatusOK, Representation{})
	if err != nil {
		t.Fatal(err)
	}
	// XML should be preferred due to higher quality
	if w.Body.String() != "<ok/>" {
		t.Fatalf("expected xml body (higher q), got %q", w.Body.String())
	}
}

func TestRespond_DefaultModeIsRenderDocument(t *testing.T) {
	codec := newMockCodec("application/json", `{}`)
	r := Renderer{Codecs: []RepresentationCodec{codec}}

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	r.Respond(w, req, http.StatusOK, Representation{})
	if codec.lastOpts.Mode != RenderDocument {
		t.Fatalf("expected RenderDocument mode, got %v", codec.lastOpts.Mode)
	}
}
