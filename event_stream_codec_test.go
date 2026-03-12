package hyper

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEventStreamCodec_MediaTypes(t *testing.T) {
	codec := EventStreamCodec{}
	mts := codec.MediaTypes()
	if len(mts) != 1 || mts[0] != "text/event-stream" {
		t.Errorf("MediaTypes() = %v, want [text/event-stream]", mts)
	}
}

func TestEventStreamCodec_EncodeEvent(t *testing.T) {
	codec := EventStreamCodec{}
	var buf bytes.Buffer

	rep := Representation{
		Kind:  "job-progress",
		State: Object{"progress": Scalar{V: 50}},
		Meta:  map[string]any{"eventID": "evt-1"},
	}

	err := codec.EncodeEvent(context.Background(), &buf, rep, EncodeOptions{})
	if err != nil {
		t.Fatalf("EncodeEvent() error = %v", err)
	}

	output := buf.String()

	if !strings.Contains(output, "event: job-progress\n") {
		t.Errorf("output missing event field: %s", output)
	}
	if !strings.Contains(output, "id: evt-1\n") {
		t.Errorf("output missing id field: %s", output)
	}
	if !strings.Contains(output, "data: ") {
		t.Errorf("output missing data field: %s", output)
	}
	// Must end with double newline (blank line terminates event).
	if !strings.HasSuffix(output, "\n\n") {
		t.Errorf("output should end with blank line: %q", output)
	}
}

func TestEventStreamCodec_Encode_SingleEvent(t *testing.T) {
	codec := EventStreamCodec{}
	var buf bytes.Buffer

	rep := Representation{
		Kind:  "test",
		State: Object{"key": Scalar{V: "value"}},
	}

	err := codec.Encode(context.Background(), &buf, rep, EncodeOptions{})
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	if !strings.Contains(buf.String(), "event: test\n") {
		t.Errorf("Encode should produce SSE event: %s", buf.String())
	}
}

func TestDecodeEvent(t *testing.T) {
	input := "event: job-progress\nid: evt-1\ndata: {\"kind\":\"job-progress\",\"state\":{\"progress\":50}}\n\n"
	reader := bufio.NewReader(strings.NewReader(input))

	rep, err := DecodeEvent(reader)
	if err != nil {
		t.Fatalf("DecodeEvent() error = %v", err)
	}

	if rep.Kind != "job-progress" {
		t.Errorf("Kind = %q, want %q", rep.Kind, "job-progress")
	}
	if rep.Meta["eventID"] != "evt-1" {
		t.Errorf("Meta[eventID] = %v, want %q", rep.Meta["eventID"], "evt-1")
	}
	state, ok := rep.State.(Object)
	if !ok {
		t.Fatalf("State is %T, want Object", rep.State)
	}
	if v, ok := state["progress"].(Scalar); !ok || v.V != 50.0 {
		t.Errorf("State[progress] = %v, want 50", state["progress"])
	}
}

func TestDecodeEvent_MultipleEvents(t *testing.T) {
	input := "event: open\ndata: {\"kind\":\"open\"}\n\nevent: close\ndata: {\"kind\":\"close\"}\n\n"
	reader := bufio.NewReader(strings.NewReader(input))

	rep1, err := DecodeEvent(reader)
	if err != nil {
		t.Fatalf("DecodeEvent (1) error = %v", err)
	}
	if rep1.Kind != "open" {
		t.Errorf("rep1.Kind = %q, want %q", rep1.Kind, "open")
	}

	rep2, err := DecodeEvent(reader)
	if err != nil {
		t.Fatalf("DecodeEvent (2) error = %v", err)
	}
	if rep2.Kind != "close" {
		t.Errorf("rep2.Kind = %q, want %q", rep2.Kind, "close")
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	codec := EventStreamCodec{}
	var buf bytes.Buffer

	original := Representation{
		Kind:  "stream-open",
		State: Object{"message": Scalar{V: "hello"}},
		Links: []Link{{Rel: "self", Target: Target{URL: mustParseURL("http://example.com/stream")}}},
		Meta:  map[string]any{"eventID": "1"},
	}

	err := codec.EncodeEvent(context.Background(), &buf, original, EncodeOptions{})
	if err != nil {
		t.Fatalf("EncodeEvent() error = %v", err)
	}

	reader := bufio.NewReader(&buf)
	decoded, err := DecodeEvent(reader)
	if err != nil {
		t.Fatalf("DecodeEvent() error = %v", err)
	}

	if decoded.Kind != original.Kind {
		t.Errorf("Kind = %q, want %q", decoded.Kind, original.Kind)
	}
	if len(decoded.Links) != 1 || decoded.Links[0].Rel != "self" {
		t.Errorf("Links not preserved: %v", decoded.Links)
	}
	if decoded.Meta["eventID"] != "1" {
		t.Errorf("Meta[eventID] = %v, want %q", decoded.Meta["eventID"], "1")
	}
}

// flusherRecorder implements http.ResponseWriter and http.Flusher.
type flusherRecorder struct {
	*httptest.ResponseRecorder
	flushed int
}

func (f *flusherRecorder) Flush() {
	f.flushed++
	f.ResponseRecorder.Flush()
}

func TestRenderer_RespondStream(t *testing.T) {
	renderer := Renderer{
		Codecs: []RepresentationCodec{EventStreamCodec{}},
	}

	rec := &flusherRecorder{ResponseRecorder: httptest.NewRecorder()}
	req := httptest.NewRequest("GET", "/stream", nil)
	req.Header.Set("Accept", "text/event-stream")

	reps := make(chan Representation, 2)
	reps <- Representation{Kind: "stream-open", State: Object{"status": Scalar{V: "started"}}}
	reps <- Representation{Kind: "stream-close", State: Object{"status": Scalar{V: "done"}}}
	close(reps)

	err := renderer.RespondStream(rec, req, reps)
	if err != nil {
		t.Fatalf("RespondStream() error = %v", err)
	}

	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want %q", ct, "text/event-stream")
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Cache-Control = %q, want %q", cc, "no-cache")
	}

	body := rec.Body.String()
	if !strings.Contains(body, "event: stream-open") {
		t.Errorf("body missing stream-open event: %s", body)
	}
	if !strings.Contains(body, "event: stream-close") {
		t.Errorf("body missing stream-close event: %s", body)
	}
	if rec.flushed < 2 {
		t.Errorf("Flush called %d times, want at least 2", rec.flushed)
	}
}

// noFlushWriter is an http.ResponseWriter that does NOT implement http.Flusher.
type noFlushWriter struct {
	header http.Header
	code   int
	body   bytes.Buffer
}

func (w *noFlushWriter) Header() http.Header         { return w.header }
func (w *noFlushWriter) Write(b []byte) (int, error)  { return w.body.Write(b) }
func (w *noFlushWriter) WriteHeader(code int)          { w.code = code }

func TestRenderer_RespondStream_NoFlusher(t *testing.T) {
	renderer := Renderer{
		Codecs: []RepresentationCodec{EventStreamCodec{}},
	}

	w := &noFlushWriter{header: make(http.Header)}
	req := httptest.NewRequest("GET", "/stream", nil)
	reps := make(chan Representation)
	close(reps)

	err := renderer.RespondStream(w, req, reps)
	if err == nil {
		t.Error("RespondStream() should error without Flusher")
	}
}

func TestRenderer_RespondStream_NoCodec(t *testing.T) {
	renderer := Renderer{
		Codecs: []RepresentationCodec{JSONCodec()},
	}

	rec := &flusherRecorder{ResponseRecorder: httptest.NewRecorder()}
	req := httptest.NewRequest("GET", "/stream", nil)
	reps := make(chan Representation)
	close(reps)

	err := renderer.RespondStream(rec, req, reps)
	if err == nil {
		t.Error("RespondStream() should error without StreamingCodec")
	}
}

func TestClient_FetchStream(t *testing.T) {
	// Set up SSE server.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Error("Expected Accept: text/event-stream")
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "no flusher", 500)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		flusher.Flush()

		codec := EventStreamCodec{}
		events := []Representation{
			{Kind: "stream-open", State: Object{"status": Scalar{V: "started"}}},
			{Kind: "token", State: Object{"text": Scalar{V: "Hello"}}},
			{Kind: "stream-close", State: Object{"status": Scalar{V: "done"}}},
		}
		for _, rep := range events {
			codec.EncodeEvent(r.Context(), w, rep, EncodeOptions{})
			codec.Flush(w)
			flusher.Flush()
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	ch, err := client.FetchStream(context.Background(), Path())
	if err != nil {
		t.Fatalf("FetchStream() error = %v", err)
	}

	var kinds []string
	for resp := range ch {
		kinds = append(kinds, resp.Representation.Kind)
	}

	if len(kinds) != 3 {
		t.Fatalf("got %d events, want 3: %v", len(kinds), kinds)
	}
	if kinds[0] != "stream-open" || kinds[1] != "token" || kinds[2] != "stream-close" {
		t.Errorf("kinds = %v, want [stream-open token stream-close]", kinds)
	}
}

func TestClient_FetchStream_NonSSEResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		io.WriteString(w, `{"kind":"fallback","state":{"ok":true}}`)
	}))
	defer server.Close()

	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	ch, err := client.FetchStream(context.Background(), Path())
	if err != nil {
		t.Fatalf("FetchStream() error = %v", err)
	}

	var responses []*Response
	for resp := range ch {
		responses = append(responses, resp)
	}

	if len(responses) != 1 {
		t.Fatalf("got %d responses, want 1", len(responses))
	}
	if responses[0].Representation.Kind != "fallback" {
		t.Errorf("Kind = %q, want %q", responses[0].Representation.Kind, "fallback")
	}
}

func TestEventStreamCodec_ImplementsStreamingCodec(t *testing.T) {
	var _ StreamingCodec = EventStreamCodec{}
}
