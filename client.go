package hyper

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
)

// HTTPDoer executes an HTTP request and returns the response.
// *http.Client satisfies this interface.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
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
		SubmissionCodecs: []SubmissionCodec{JSONSubmissionCodec(), FormSubmissionCodec()},
		BaseURL:          u,
		Accept:           "application/vnd.api+json",
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
		// Select submission codec and encode body (§11.4.2)
		codec, ct := c.selectSubmissionCodec(action.Consumes)
		contentType = ct
		if codec != nil {
			encoded, encErr := codec.Encode(values)
			if encErr != nil {
				return nil, fmt.Errorf("hyper: encode submission: %w", encErr)
			}
			body = encoded
		} else {
			var buf bytes.Buffer
			enc := json.NewEncoder(&buf)
			enc.SetEscapeHTML(false)
			if err := enc.Encode(values); err != nil {
				return nil, fmt.Errorf("hyper: encode submission: %w", err)
			}
			body = &buf
		}
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
				codec, _ := c.selectSubmissionCodec(action.Consumes)
				if codec != nil {
					encoded, encErr := codec.Encode(values)
					if encErr != nil {
						return nil, fmt.Errorf("hyper: encode submission retry: %w", encErr)
					}
					retryBody = encoded
				} else {
					var buf bytes.Buffer
					enc := json.NewEncoder(&buf)
					enc.SetEscapeHTML(false)
					if err := enc.Encode(values); err != nil {
						return nil, fmt.Errorf("hyper: encode submission retry: %w", err)
					}
					retryBody = &buf
				}
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

// FetchStream sends a GET with Accept: text/event-stream and returns a
// channel of Response values, one per SSE event. The channel is closed when
// the stream ends or the context is cancelled.
func (c *Client) FetchStream(ctx context.Context, target Target) (<-chan *Response, error) {
	u, err := c.resolveTarget(target)
	if err != nil {
		return nil, fmt.Errorf("hyper: resolve target: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("hyper: create request: %w", err)
	}

	req.Header.Set("Accept", "text/event-stream")

	if err := c.attachCredential(ctx, req); err != nil {
		return nil, err
	}

	resp, err := c.Transport.Do(req)
	if err != nil {
		return nil, fmt.Errorf("hyper: execute request: %w", err)
	}

	ct := resp.Header.Get("Content-Type")
	mt, _, _ := mime.ParseMediaType(ct)
	if mt != "text/event-stream" {
		// Not a stream response — decode as a single response and return.
		singleResp, err := c.decodeResponse(resp)
		ch := make(chan *Response, 1)
		if err == nil {
			ch <- singleResp
		}
		close(ch)
		return ch, err
	}

	ch := make(chan *Response)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		reader := bufio.NewReader(resp.Body)
		for {
			rep, err := DecodeEvent(reader)
			if err != nil {
				return
			}

			r := &Response{
				Representation: rep,
				StatusCode:     resp.StatusCode,
				Header:         resp.Header,
			}

			select {
			case <-ctx.Done():
				return
			case ch <- r:
			}
		}
	}()

	return ch, nil
}

// Follow is a convenience for Fetch that takes a Link instead of a Target.
func (c *Client) Follow(ctx context.Context, link Link) (*Response, error) {
	return c.Fetch(ctx, link.Target)
}

// resolveTarget resolves a Target to an absolute URL against BaseURL.
// Query parameters from Target.Query and Target.Route.Query are appended
// to the resolved URL (§8.1).
func (c *Client) resolveTarget(t Target) (*url.URL, error) {
	var resolved *url.URL
	if t.URL == nil {
		resolved = c.BaseURL
	} else if t.URL.IsAbs() {
		resolved = t.URL
	} else {
		resolved = c.BaseURL.ResolveReference(t.URL)
	}

	// Merge query parameters from Target.Query and Route.Query.
	var extra url.Values
	if t.Query != nil {
		extra = t.Query
	}
	if t.Route != nil && t.Route.Query != nil {
		if extra == nil {
			extra = t.Route.Query
		} else {
			for k, vs := range t.Route.Query {
				for _, v := range vs {
					extra.Add(k, v)
				}
			}
		}
	}

	if extra != nil {
		// Copy to avoid mutating the original URL.
		u := *resolved
		q := u.Query()
		for k, vs := range extra {
			for _, v := range vs {
				q.Add(k, v)
			}
		}
		u.RawQuery = q.Encode()
		resolved = &u
	}

	return resolved, nil
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

// selectSubmissionCodec picks a SubmissionCodec and media type for encoding.
// It returns the matched codec and media type string. If no codec matches,
// it returns the first registered codec with a fallback media type.
func (c *Client) selectSubmissionCodec(consumes []string) (SubmissionCodec, string) {
	if len(consumes) > 0 {
		for _, ct := range consumes {
			for _, sc := range c.SubmissionCodecs {
				for _, mt := range sc.MediaTypes() {
					if mt == ct {
						return sc, mt
					}
				}
			}
		}
	}
	// Default to the first registered submission codec
	if len(c.SubmissionCodecs) > 0 {
		mts := c.SubmissionCodecs[0].MediaTypes()
		if len(mts) > 0 {
			return c.SubmissionCodecs[0], mts[0]
		}
		return c.SubmissionCodecs[0], "application/json"
	}
	return nil, "application/json"
}

// decodeResponse reads the HTTP response and decodes it into a Response.
// It checks the Content-Type header and uses a matching RepresentationDecoder
// from Client.Codecs. If no matching decoder is found, it falls back to JSON.
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

	// Extract the media type from the Content-Type header.
	contentType := resp.Header.Get("Content-Type")
	mediaType := ""
	if contentType != "" {
		mt, _, _ := mime.ParseMediaType(contentType)
		mediaType = mt
	}

	// Try to find a matching RepresentationDecoder from registered codecs.
	if decoder := c.findDecoder(mediaType); decoder != nil {
		rep, err := decoder.DecodeRepresentation(context.Background(), bytes.NewReader(bodyBytes))
		if err != nil {
			// Decoder found but failed; return response without representation.
			return result, nil
		}
		result.Representation = rep
		return result, nil
	}

	// Fallback: decode as JSON for backward compatibility.
	var raw map[string]any
	if err := json.Unmarshal(bodyBytes, &raw); err != nil {
		return result, nil
	}

	rep, err := decodeRepresentation(raw)
	if err != nil {
		return result, nil
	}
	result.Representation = rep
	return result, nil
}

// findDecoder searches Client.Codecs for a RepresentationDecoder whose
// MediaTypes include the given media type.
func (c *Client) findDecoder(mediaType string) RepresentationDecoder {
	if mediaType == "" {
		return nil
	}
	for _, codec := range c.Codecs {
		dec, ok := codec.(RepresentationDecoder)
		if !ok {
			continue
		}
		for _, mt := range dec.MediaTypes() {
			if mt == mediaType {
				return dec
			}
		}
	}
	return nil
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
	if a, ok := m["accept"].(string); ok {
		f.Accept = a
	}
	if ms, ok := m["maxSize"].(float64); ok {
		f.MaxSize = int64(ms)
	}
	if mul, ok := m["multiple"].(bool); ok {
		f.Multiple = mul
	}
	if opts, ok := m["options"].([]any); ok {
		for _, o := range opts {
			if om, ok := o.(map[string]any); ok {
				var opt Option
				if v, ok := om["value"].(string); ok {
					opt.Value = v
				}
				if l, ok := om["label"].(string); ok {
					opt.Label = l
				}
				if s, ok := om["selected"].(bool); ok {
					opt.Selected = s
				}
				f.Options = append(f.Options, opt)
			}
		}
	}
	return f
}

