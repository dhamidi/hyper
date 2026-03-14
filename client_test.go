package hyper

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// mockDoer records the request and returns a canned response.
type mockDoer struct {
	handler func(*http.Request) (*http.Response, error)
}

func (m *mockDoer) Do(req *http.Request) (*http.Response, error) {
	return m.handler(req)
}

func jsonResponse(status int, body map[string]any) *http.Response {
	b, _ := json.Marshal(body)
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(b)),
	}
}

func TestResponse_IsSuccess(t *testing.T) {
	tests := []struct {
		code int
		want bool
	}{
		{200, true},
		{201, true},
		{299, true},
		{300, false},
		{400, false},
		{500, false},
	}
	for _, tt := range tests {
		r := &Response{StatusCode: tt.code}
		if got := r.IsSuccess(); got != tt.want {
			t.Errorf("IsSuccess(%d) = %v, want %v", tt.code, got, tt.want)
		}
	}
}

func TestResponse_IsError(t *testing.T) {
	tests := []struct {
		code int
		want bool
	}{
		{200, false},
		{300, false},
		{400, true},
		{404, true},
		{500, true},
		{599, true},
	}
	for _, tt := range tests {
		r := &Response{StatusCode: tt.code}
		if got := r.IsError(); got != tt.want {
			t.Errorf("IsError(%d) = %v, want %v", tt.code, got, tt.want)
		}
	}
}

func TestNewClient_Defaults(t *testing.T) {
	c, err := NewClient("http://example.com/api")
	if err != nil {
		t.Fatal(err)
	}
	if c.BaseURL.String() != "http://example.com/api" {
		t.Errorf("BaseURL = %q", c.BaseURL)
	}
	if c.Transport == nil {
		t.Error("Transport is nil")
	}
	if len(c.Codecs) == 0 {
		t.Error("Codecs is empty")
	}
	if len(c.SubmissionCodecs) == 0 {
		t.Error("SubmissionCodecs is empty")
	}
	if c.Accept != "application/json" {
		t.Errorf("Accept = %q", c.Accept)
	}
}

func TestWithTransport(t *testing.T) {
	mock := &mockDoer{}
	c, err := NewClient("http://example.com", WithTransport(mock))
	if err != nil {
		t.Fatal(err)
	}
	if c.Transport != mock {
		t.Error("Transport not set")
	}
}

func TestWithCredentials(t *testing.T) {
	store := &mockCredentialStore{}
	c, err := NewClient("http://example.com", WithCredentials(store))
	if err != nil {
		t.Fatal(err)
	}
	if c.Credentials != store {
		t.Error("Credentials not set")
	}
}

func TestFetch_SendsGETWithAccept(t *testing.T) {
	var gotReq *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotReq = r
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"kind": "root"})
	}))
	defer srv.Close()

	c, _ := NewClient(srv.URL)
	resp, err := c.Fetch(context.Background(), Path())
	if err != nil {
		t.Fatal(err)
	}

	if gotReq.Method != "GET" {
		t.Errorf("method = %q, want GET", gotReq.Method)
	}
	if got := gotReq.Header.Get("Accept"); got != "application/json" {
		t.Errorf("Accept = %q", got)
	}
	if resp.StatusCode != 200 {
		t.Errorf("StatusCode = %d", resp.StatusCode)
	}
	if resp.Representation.Kind != "root" {
		t.Errorf("Kind = %q", resp.Representation.Kind)
	}
}

func TestFetch_ResolvesRelativeURL(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"kind": "item"})
	}))
	defer srv.Close()

	c, _ := NewClient(srv.URL + "/api")
	target := MustParseTarget("/api/items/1")
	_, err := c.Fetch(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}

	if gotPath != "/api/items/1" {
		t.Errorf("path = %q, want /api/items/1", gotPath)
	}
}

func TestSubmit_SendsMethodWithBody(t *testing.T) {
	var gotMethod string
	var gotBody map[string]any
	var gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"kind": "created"})
	}))
	defer srv.Close()

	c, _ := NewClient(srv.URL)
	action := Action{
		Method: "POST",
		Target: MustParseTarget(srv.URL + "/items"),
	}
	resp, err := c.Submit(context.Background(), action, map[string]any{"name": "test"})
	if err != nil {
		t.Fatal(err)
	}

	if gotMethod != "POST" {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q", gotContentType)
	}
	if gotBody["name"] != "test" {
		t.Errorf("body name = %v", gotBody["name"])
	}
	if resp.Representation.Kind != "created" {
		t.Errorf("Kind = %q", resp.Representation.Kind)
	}
}

func TestSubmit_GETEncodesValuesAsQuery(t *testing.T) {
	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"kind": "search"})
	}))
	defer srv.Close()

	c, _ := NewClient(srv.URL)
	action := Action{
		Method: "GET",
		Target: MustParseTarget(srv.URL + "/search"),
	}
	_, err := c.Submit(context.Background(), action, map[string]any{"q": "hello"})
	if err != nil {
		t.Fatal(err)
	}

	if gotQuery.Get("q") != "hello" {
		t.Errorf("query q = %q", gotQuery.Get("q"))
	}
}

func TestFollow_DelegatesToFetch(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"kind": "followed"})
	}))
	defer srv.Close()

	c, _ := NewClient(srv.URL)
	link := Link{
		Rel:    "next",
		Target: MustParseTarget(srv.URL + "/page/2"),
	}
	resp, err := c.Follow(context.Background(), link)
	if err != nil {
		t.Fatal(err)
	}

	if gotPath != "/page/2" {
		t.Errorf("path = %q", gotPath)
	}
	if resp.Representation.Kind != "followed" {
		t.Errorf("Kind = %q", resp.Representation.Kind)
	}
}

func TestCredential_Bearer(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer srv.Close()

	c, _ := NewClient(srv.URL, WithStaticCredential(BearerToken("mytoken")))
	_, err := c.Fetch(context.Background(), Path())
	if err != nil {
		t.Fatal(err)
	}

	if gotAuth != "Bearer mytoken" {
		t.Errorf("Authorization = %q", gotAuth)
	}
}

func TestCredential_APIKeyHeader(t *testing.T) {
	var gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-API-Key")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer srv.Close()

	c, _ := NewClient(srv.URL, WithStaticCredential(APIKeyHeader("X-API-Key", "secret")))
	_, err := c.Fetch(context.Background(), Path())
	if err != nil {
		t.Fatal(err)
	}

	if gotHeader != "secret" {
		t.Errorf("X-API-Key = %q", gotHeader)
	}
}

func TestCredential_APIKeyQuery(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("api_key")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer srv.Close()

	c, _ := NewClient(srv.URL, WithStaticCredential(APIKeyQuery("api_key", "mykey")))
	_, err := c.Fetch(context.Background(), Path())
	if err != nil {
		t.Fatal(err)
	}

	if gotQuery != "mykey" {
		t.Errorf("api_key = %q", gotQuery)
	}
}

func TestOnUnauthorized_RetryOn401(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if r.Header.Get("Authorization") != "Bearer fresh-token" {
			w.WriteHeader(401)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"kind": "authed"})
	}))
	defer srv.Close()

	c, _ := NewClient(srv.URL)
	c.OnUnauthorized = func(ctx context.Context, resp *Response) (*Credential, error) {
		cred := BearerToken("fresh-token")
		return &cred, nil
	}

	resp, err := c.Fetch(context.Background(), Path())
	if err != nil {
		t.Fatal(err)
	}

	if callCount != 2 {
		t.Errorf("call count = %d, want 2", callCount)
	}
	if resp.Representation.Kind != "authed" {
		t.Errorf("Kind = %q", resp.Representation.Kind)
	}
}

func TestDecodeResponse_UsesContentTypeCodec(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"kind": "json-decoded"})
	}))
	defer srv.Close()

	c, _ := NewClient(srv.URL)
	resp, err := c.Fetch(context.Background(), Path())
	if err != nil {
		t.Fatal(err)
	}
	if resp.Representation.Kind != "json-decoded" {
		t.Errorf("Kind = %q, want json-decoded", resp.Representation.Kind)
	}
}

func TestDecodeResponse_FallsBackToJSONForUnknownContentType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-unknown")
		json.NewEncoder(w).Encode(map[string]any{"kind": "fallback"})
	}))
	defer srv.Close()

	c, _ := NewClient(srv.URL)
	resp, err := c.Fetch(context.Background(), Path())
	if err != nil {
		t.Fatal(err)
	}
	if resp.Representation.Kind != "fallback" {
		t.Errorf("Kind = %q, want fallback", resp.Representation.Kind)
	}
}

func TestDecodeResponse_HandlesContentTypeWithParams(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(map[string]any{"kind": "with-params"})
	}))
	defer srv.Close()

	c, _ := NewClient(srv.URL)
	resp, err := c.Fetch(context.Background(), Path())
	if err != nil {
		t.Fatal(err)
	}
	if resp.Representation.Kind != "with-params" {
		t.Errorf("Kind = %q, want with-params", resp.Representation.Kind)
	}
}

func TestDecodeResponse_NonJSONBodyWithoutDecoder(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: hello\n\n"))
	}))
	defer srv.Close()

	c, _ := NewClient(srv.URL)
	resp, err := c.Fetch(context.Background(), Path())
	if err != nil {
		t.Fatal(err)
	}
	// No matching decoder and not valid JSON, so Representation should be empty
	if resp.Representation.Kind != "" {
		t.Errorf("Kind = %q, want empty", resp.Representation.Kind)
	}
}

// mockRepDecoder is a test codec that implements both RepresentationCodec and RepresentationDecoder.
type mockRepDecoder struct {
	mediaTypes []string
	decoded    Representation
}

func (m *mockRepDecoder) MediaTypes() []string { return m.mediaTypes }
func (m *mockRepDecoder) Encode(_ context.Context, _ io.Writer, _ Representation, _ EncodeOptions) error {
	return nil
}
func (m *mockRepDecoder) DecodeRepresentation(_ context.Context, r io.Reader) (Representation, error) {
	// Just return the preconfigured representation.
	io.ReadAll(r) // drain body
	return m.decoded, nil
}

func TestDecodeResponse_CustomDecoderByContentType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.Write([]byte(`{"not":"standard hyper json"}`))
	}))
	defer srv.Close()

	custom := &mockRepDecoder{
		mediaTypes: []string{"application/vnd.api+json"},
		decoded:    Representation{Kind: "custom-decoded"},
	}

	c, _ := NewClient(srv.URL, WithCodec(custom))
	resp, err := c.Fetch(context.Background(), Path())
	if err != nil {
		t.Fatal(err)
	}
	if resp.Representation.Kind != "custom-decoded" {
		t.Errorf("Kind = %q, want custom-decoded", resp.Representation.Kind)
	}
}

func TestDecodeField_FileFieldConstraints(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"actions": [{
				"name": "upload",
				"method": "POST",
				"fields": [{
					"name": "file",
					"type": "file",
					"required": true,
					"accept": "image/*",
					"maxSize": 10485760,
					"multiple": true
				}]
			}]
		}`))
	}))
	defer srv.Close()

	c, _ := NewClient(srv.URL)
	resp, err := c.Fetch(context.Background(), Path())
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Representation.Actions) != 1 {
		t.Fatalf("actions = %d, want 1", len(resp.Representation.Actions))
	}
	f := resp.Representation.Actions[0].Fields[0]
	if f.Accept != "image/*" {
		t.Errorf("Accept = %q, want image/*", f.Accept)
	}
	if f.MaxSize != 10485760 {
		t.Errorf("MaxSize = %d, want 10485760", f.MaxSize)
	}
	if !f.Multiple {
		t.Error("Multiple = false, want true")
	}
}

func TestDecodeField_Options(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"actions": [{
				"name": "choose",
				"method": "POST",
				"fields": [{
					"name": "color",
					"type": "select",
					"options": [
						{"value": "red", "label": "Red"},
						{"value": "blue", "label": "Blue", "selected": true}
					]
				}]
			}]
		}`))
	}))
	defer srv.Close()

	c, _ := NewClient(srv.URL)
	resp, err := c.Fetch(context.Background(), Path())
	if err != nil {
		t.Fatal(err)
	}
	f := resp.Representation.Actions[0].Fields[0]
	if len(f.Options) != 2 {
		t.Fatalf("Options length = %d, want 2", len(f.Options))
	}
	if f.Options[0].Value != "red" || f.Options[0].Label != "Red" {
		t.Errorf("Options[0] = %+v, want {red Red false}", f.Options[0])
	}
	if f.Options[1].Value != "blue" || !f.Options[1].Selected {
		t.Errorf("Options[1] = %+v, want {blue Blue true}", f.Options[1])
	}
}

// mockCredentialStore is a simple in-memory credential store for testing.
type mockCredentialStore struct {
	cred Credential
}

func (m *mockCredentialStore) Credential(_ context.Context, _ *url.URL) (Credential, error) {
	return m.cred, nil
}

func (m *mockCredentialStore) Store(_ context.Context, _ *url.URL, cred Credential) error {
	m.cred = cred
	return nil
}

func (m *mockCredentialStore) Delete(_ context.Context, _ *url.URL) error {
	m.cred = Credential{}
	return nil
}
