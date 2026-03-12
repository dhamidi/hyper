package hyper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// HTTPDoer executes an HTTP request and returns the response.
// *http.Client satisfies this interface.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// Credential represents an authentication credential with its placement strategy.
type Credential struct {
	// Scheme determines how the credential is attached to requests.
	// Well-known values: "bearer", "apikey-header", "apikey-query".
	Scheme string

	// Value is the credential value (token, key, etc.).
	Value string

	// Header is the header name for "apikey-header" scheme.
	Header string

	// Param is the query parameter name for "apikey-query" scheme.
	Param string
}

// BearerToken returns a Credential that attaches as "Authorization: Bearer <token>".
func BearerToken(token string) Credential {
	return Credential{Scheme: "bearer", Value: token}
}

// APIKeyHeader returns a Credential that attaches the key in the named header.
func APIKeyHeader(header, key string) Credential {
	return Credential{Scheme: "apikey-header", Header: header, Value: key}
}

// APIKeyQuery returns a Credential that appends the key as a query parameter.
func APIKeyQuery(param, key string) Credential {
	return Credential{Scheme: "apikey-query", Param: param, Value: key}
}

// CredentialStore retrieves and persists authentication credentials.
type CredentialStore interface {
	// Credential returns the current credential for the given base URL.
	Credential(ctx context.Context, baseURL *url.URL) (Credential, error)

	// Store persists a credential for the given base URL.
	Store(ctx context.Context, baseURL *url.URL, cred Credential) error

	// Delete removes the stored credential for the given base URL.
	Delete(ctx context.Context, baseURL *url.URL) error
}

// Response wraps a decoded Representation with HTTP metadata.
type Response struct {
	// Representation is the decoded hypermedia representation.
	Representation Representation

	// StatusCode is the HTTP status code.
	StatusCode int

	// Header contains the response headers.
	Header http.Header

	// Body is the raw response body.
	Body io.ReadCloser
}

// IsSuccess returns true if StatusCode is in the 2xx range.
func (r *Response) IsSuccess() bool {
	return r.StatusCode >= 200 && r.StatusCode < 300
}

// IsError returns true if StatusCode is 4xx or 5xx.
func (r *Response) IsError() bool {
	return r.StatusCode >= 400 && r.StatusCode < 600
}

// Client provides programmatic access to hyper APIs.
type Client struct {
	// Transport executes HTTP requests. Defaults to http.DefaultClient.
	Transport HTTPDoer

	// Codecs maps media types to RepresentationCodecs for decoding responses.
	Codecs []RepresentationCodec

	// SubmissionCodecs maps media types to SubmissionCodecs for encoding request bodies.
	SubmissionCodecs []SubmissionCodec

	// Credentials provides authentication credentials for requests.
	Credentials CredentialStore

	// BaseURL is the root URL of the API.
	BaseURL *url.URL

	// Accept is the Accept header value sent with requests.
	Accept string

	// OnUnauthorized is called when a request receives a 401 response.
	OnUnauthorized func(ctx context.Context, resp *Response) (*Credential, error)
}

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithTransport sets the HTTP transport.
func WithTransport(t HTTPDoer) ClientOption {
	return func(c *Client) { c.Transport = t }
}

// WithCredentials sets the credential store.
func WithCredentials(cs CredentialStore) ClientOption {
	return func(c *Client) { c.Credentials = cs }
}

// WithStaticCredential wraps a single Credential in a read-only CredentialStore.
func WithStaticCredential(cred Credential) ClientOption {
	return func(c *Client) { c.Credentials = &staticCredentialStore{cred: cred} }
}

// WithCodec adds a RepresentationCodec.
func WithCodec(codec RepresentationCodec) ClientOption {
	return func(c *Client) { c.Codecs = append(c.Codecs, codec) }
}

// WithSubmissionCodec adds a SubmissionCodec.
func WithSubmissionCodec(codec SubmissionCodec) ClientOption {
	return func(c *Client) { c.SubmissionCodecs = append(c.SubmissionCodecs, codec) }
}

// WithAccept sets the Accept header value.
func WithAccept(accept string) ClientOption {
	return func(c *Client) { c.Accept = accept }
}

// NewClient creates a Client with sensible defaults.
func NewClient(baseURL string, opts ...ClientOption) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("hyper: parse base URL: %w", err)
	}

	c := &Client{
		Transport:        http.DefaultClient,
		Codecs:           []RepresentationCodec{JSONCodec()},
		SubmissionCodecs: []SubmissionCodec{JSONSubmissionCodec()},
		BaseURL:          u,
		Accept:           "application/json",
	}

	for _, opt := range opts {
		opt(c)
	}

	return c, nil
}

// Fetch sends a GET request to the given Target and decodes the response.
func (c *Client) Fetch(ctx context.Context, target Target) (*Response, error) {
	u, err := c.resolveTarget(target)
	if err != nil {
		return nil, fmt.Errorf("hyper: resolve target: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("hyper: create request: %w", err)
	}

	accept := c.Accept
	if accept == "" {
		accept = "application/json"
	}
	req.Header.Set("Accept", accept)

	if err := c.attachCredential(ctx, req); err != nil {
		return nil, err
	}

	resp, err := c.Transport.Do(req)
	if err != nil {
		return nil, fmt.Errorf("hyper: execute request: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized && c.OnUnauthorized != nil {
		wrapped := &Response{StatusCode: resp.StatusCode, Header: resp.Header, Body: resp.Body}
		cred, err := c.OnUnauthorized(ctx, wrapped)
		if err != nil {
			return nil, fmt.Errorf("hyper: on unauthorized: %w", err)
		}
		if cred != nil {
			req, err = http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
			if err != nil {
				return nil, fmt.Errorf("hyper: create retry request: %w", err)
			}
			req.Header.Set("Accept", accept)
			applyCredential(req, *cred)
			resp, err = c.Transport.Do(req)
			if err != nil {
				return nil, fmt.Errorf("hyper: execute retry request: %w", err)
			}
		}
	}

	return c.decodeResponse(resp)
}

// Submit executes an Action with the given field values.
func (c *Client) Submit(ctx context.Context, action Action, values map[string]any) (*Response, error) {
	u, err := c.resolveTarget(action.Target)
	if err != nil {
		return nil, fmt.Errorf("hyper: resolve action target: %w", err)
	}

	method := action.Method
	if method == "" {
		method = http.MethodPost
	}

	var body io.Reader
	var contentType string

	if strings.EqualFold(method, http.MethodGet) {
		// Encode values as query parameters
		if len(values) > 0 {
			q := u.Query()
			for k, v := range values {
				q.Set(k, fmt.Sprintf("%v", v))
			}
			u.RawQuery = q.Encode()
		}
	} else if len(values) > 0 {
		// Select submission codec and encode body
		contentType = c.selectSubmissionMediaType(action.Consumes)
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(values); err != nil {
			return nil, fmt.Errorf("hyper: encode submission: %w", err)
		}
		body = &buf
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, fmt.Errorf("hyper: create request: %w", err)
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	accept := c.Accept
	if accept == "" {
		accept = "application/json"
	}
	req.Header.Set("Accept", accept)

	if err := c.attachCredential(ctx, req); err != nil {
		return nil, err
	}

	resp, err := c.Transport.Do(req)
	if err != nil {
		return nil, fmt.Errorf("hyper: execute request: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized && c.OnUnauthorized != nil {
		wrapped := &Response{StatusCode: resp.StatusCode, Header: resp.Header, Body: resp.Body}
		cred, retryErr := c.OnUnauthorized(ctx, wrapped)
		if retryErr != nil {
			return nil, fmt.Errorf("hyper: on unauthorized: %w", retryErr)
		}
		if cred != nil {
			// Rebuild body for retry
			var retryBody io.Reader
			if !strings.EqualFold(method, http.MethodGet) && len(values) > 0 {
				var buf bytes.Buffer
				enc := json.NewEncoder(&buf)
				enc.SetEscapeHTML(false)
				if err := enc.Encode(values); err != nil {
					return nil, fmt.Errorf("hyper: encode submission retry: %w", err)
				}
				retryBody = &buf
			}
			req, err = http.NewRequestWithContext(ctx, method, u.String(), retryBody)
			if err != nil {
				return nil, fmt.Errorf("hyper: create retry request: %w", err)
			}
			if contentType != "" {
				req.Header.Set("Content-Type", contentType)
			}
			req.Header.Set("Accept", accept)
			applyCredential(req, *cred)
			resp, err = c.Transport.Do(req)
			if err != nil {
				return nil, fmt.Errorf("hyper: execute retry request: %w", err)
			}
		}
	}

	return c.decodeResponse(resp)
}

// Follow is a convenience for Fetch that takes a Link instead of a Target.
func (c *Client) Follow(ctx context.Context, link Link) (*Response, error) {
	return c.Fetch(ctx, link.Target)
}

// resolveTarget resolves a Target to an absolute URL against BaseURL.
func (c *Client) resolveTarget(t Target) (*url.URL, error) {
	if t.URL == nil {
		return c.BaseURL, nil
	}
	if t.URL.IsAbs() {
		return t.URL, nil
	}
	return c.BaseURL.ResolveReference(t.URL), nil
}

// attachCredential retrieves a credential from the store and applies it.
func (c *Client) attachCredential(ctx context.Context, req *http.Request) error {
	if c.Credentials == nil {
		return nil
	}
	cred, err := c.Credentials.Credential(ctx, c.BaseURL)
	if err != nil {
		return fmt.Errorf("hyper: get credential: %w", err)
	}
	if cred.Value == "" {
		return nil
	}
	applyCredential(req, cred)
	return nil
}

// applyCredential attaches a credential to a request based on its Scheme.
func applyCredential(req *http.Request, cred Credential) {
	switch cred.Scheme {
	case "apikey-header":
		req.Header.Set(cred.Header, cred.Value)
	case "apikey-query":
		q := req.URL.Query()
		q.Set(cred.Param, cred.Value)
		req.URL.RawQuery = q.Encode()
	default: // "bearer" or empty
		req.Header.Set("Authorization", "Bearer "+cred.Value)
	}
}

// selectSubmissionMediaType picks the media type for a submission body.
func (c *Client) selectSubmissionMediaType(consumes []string) string {
	if len(consumes) > 0 {
		// Try to match the first consumes entry against registered codecs
		for _, ct := range consumes {
			for _, sc := range c.SubmissionCodecs {
				for _, mt := range sc.MediaTypes() {
					if mt == ct {
						return mt
					}
				}
			}
		}
		// Fall back to the first consumes entry
		return consumes[0]
	}
	// Default to the first registered submission codec's media type
	if len(c.SubmissionCodecs) > 0 {
		mts := c.SubmissionCodecs[0].MediaTypes()
		if len(mts) > 0 {
			return mts[0]
		}
	}
	return "application/json"
}

// decodeResponse reads the HTTP response and decodes it into a Response.
func (c *Client) decodeResponse(resp *http.Response) (*Response, error) {
	bodyBytes, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("hyper: read response body: %w", err)
	}

	result := &Response{
		StatusCode: resp.StatusCode,
		Header:     resp.Header,
		Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
	}

	if len(bodyBytes) == 0 {
		return result, nil
	}

	// Decode the JSON wire format into a Representation
	var raw map[string]any
	if err := json.Unmarshal(bodyBytes, &raw); err != nil {
		// If not valid JSON, return the response without a decoded representation
		return result, nil
	}

	rep, err := decodeRepresentation(raw)
	if err != nil {
		return result, nil
	}
	result.Representation = rep
	return result, nil
}

// decodeRepresentation decodes a JSON wire format map into a Representation.
func decodeRepresentation(raw map[string]any) (Representation, error) {
	var rep Representation

	if kind, ok := raw["kind"].(string); ok {
		rep.Kind = kind
	}

	if self, ok := raw["self"].(map[string]any); ok {
		if href, ok := self["href"].(string); ok {
			u, err := url.Parse(href)
			if err == nil {
				rep.Self = &Target{URL: u}
			}
		}
	}

	if state, ok := raw["state"]; ok {
		rep.State = decodeNode(state)
	}

	if links, ok := raw["links"].([]any); ok {
		for _, l := range links {
			if lm, ok := l.(map[string]any); ok {
				link := decodeLink(lm)
				rep.Links = append(rep.Links, link)
			}
		}
	}

	if actions, ok := raw["actions"].([]any); ok {
		for _, a := range actions {
			if am, ok := a.(map[string]any); ok {
				action := decodeAction(am)
				rep.Actions = append(rep.Actions, action)
			}
		}
	}

	if embedded, ok := raw["embedded"].(map[string]any); ok {
		rep.Embedded = make(map[string][]Representation)
		for slot, reps := range embedded {
			if arr, ok := reps.([]any); ok {
				for _, r := range arr {
					if rm, ok := r.(map[string]any); ok {
						sub, _ := decodeRepresentation(rm)
						rep.Embedded[slot] = append(rep.Embedded[slot], sub)
					}
				}
			}
		}
	}

	if meta, ok := raw["meta"].(map[string]any); ok {
		rep.Meta = meta
	}

	if hints, ok := raw["hints"].(map[string]any); ok {
		rep.Hints = hints
	}

	return rep, nil
}

func decodeNode(v any) Node {
	switch val := v.(type) {
	case map[string]any:
		obj := make(Object, len(val))
		for k, v := range val {
			obj[k] = decodeScalarValue(v)
		}
		return obj
	case []any:
		col := make(Collection, len(val))
		for i, v := range val {
			col[i] = decodeScalarValue(v)
		}
		return col
	default:
		return nil
	}
}

func decodeScalarValue(v any) Value {
	if m, ok := v.(map[string]any); ok {
		if t, _ := m["_type"].(string); t == "richtext" {
			return RichText{
				MediaType: m["mediaType"].(string),
				Source:    m["source"].(string),
			}
		}
	}
	return Scalar{V: v}
}

func decodeLink(m map[string]any) Link {
	var link Link
	if rel, ok := m["rel"].(string); ok {
		link.Rel = rel
	}
	if href, ok := m["href"].(string); ok {
		u, err := url.Parse(href)
		if err == nil {
			link.Target = Target{URL: u}
		}
	}
	if title, ok := m["title"].(string); ok {
		link.Title = title
	}
	if t, ok := m["type"].(string); ok {
		link.Type = t
	}
	return link
}

func decodeAction(m map[string]any) Action {
	var action Action
	if name, ok := m["name"].(string); ok {
		action.Name = name
	}
	if rel, ok := m["rel"].(string); ok {
		action.Rel = rel
	}
	if method, ok := m["method"].(string); ok {
		action.Method = method
	}
	if href, ok := m["href"].(string); ok {
		u, err := url.Parse(href)
		if err == nil {
			action.Target = Target{URL: u}
		}
	}
	if consumes, ok := m["consumes"].([]any); ok {
		for _, c := range consumes {
			if s, ok := c.(string); ok {
				action.Consumes = append(action.Consumes, s)
			}
		}
	}
	if produces, ok := m["produces"].([]any); ok {
		for _, p := range produces {
			if s, ok := p.(string); ok {
				action.Produces = append(action.Produces, s)
			}
		}
	}
	if fields, ok := m["fields"].([]any); ok {
		for _, f := range fields {
			if fm, ok := f.(map[string]any); ok {
				action.Fields = append(action.Fields, decodeField(fm))
			}
		}
	}
	if hints, ok := m["hints"].(map[string]any); ok {
		action.Hints = hints
	}
	return action
}

func decodeField(m map[string]any) Field {
	var f Field
	if name, ok := m["name"].(string); ok {
		f.Name = name
	}
	if t, ok := m["type"].(string); ok {
		f.Type = t
	}
	if v, ok := m["value"]; ok {
		f.Value = v
	}
	if r, ok := m["required"].(bool); ok {
		f.Required = r
	}
	if r, ok := m["readOnly"].(bool); ok {
		f.ReadOnly = r
	}
	if l, ok := m["label"].(string); ok {
		f.Label = l
	}
	if h, ok := m["help"].(string); ok {
		f.Help = h
	}
	if e, ok := m["error"].(string); ok {
		f.Error = e
	}
	return f
}

// staticCredentialStore always returns the same credential.
type staticCredentialStore struct {
	cred Credential
}

func (s *staticCredentialStore) Credential(_ context.Context, _ *url.URL) (Credential, error) {
	return s.cred, nil
}

func (s *staticCredentialStore) Store(_ context.Context, _ *url.URL, _ Credential) error {
	return nil
}

func (s *staticCredentialStore) Delete(_ context.Context, _ *url.URL) error {
	return nil
}
