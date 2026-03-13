// Package hyper provides a hypermedia representation model for building
// RESTful APIs and hypermedia-driven applications.
package hyper

import (
	"context"
	"io"
	"net/http"
	"net/url"
)

// Representation is the primary transferable hypermedia value.
type Representation struct {
	Kind     string                      // Application-defined semantic label
	Self     *Target                     // Canonical target URL
	State    Node                        // Primary application state
	Links    []Link                      // Navigational controls
	Actions  []Action                    // Available state transitions
	Embedded map[string][]Representation // Named nested representations
	Meta     map[string]any              // Application metadata
	Hints    map[string]any              // Codec/UI rendering directives
}

// Node represents structured representation state.
type Node interface {
	isNode()
}

// Object is a key-value map node.
type Object map[string]Value

func (Object) isNode() {}

// Collection is an ordered array node.
type Collection []Value

func (Collection) isNode() {}

// Value represents a leaf value in the model.
type Value interface {
	isValue()
}

// Scalar wraps any JSON-like scalar value.
type Scalar struct {
	V any
}

func (Scalar) isValue() {}

// RichText represents multi-format text content.
type RichText struct {
	MediaType string
	Source    string
}

func (RichText) isValue() {}

// Link is a navigational hypermedia control.
type Link struct {
	Rel    string // Semantics
	Target Target // Destination
	Title  string // Optional human-readable label
	Type   string // Optional expected media type
}

// Action is a state transition hypermedia control.
type Action struct {
	Name     string         // Identifier
	Rel      string         // Semantics
	Method   string         // HTTP method
	Target   Target         // Destination
	Consumes []string       // Accepted submission media types
	Produces []string       // Likely response media types
	Fields   []Field        // Input metadata
	Hints    map[string]any // Codec-specific metadata
}

// Field describes an action input.
type Field struct {
	Name     string   // Field name
	Type     string   // Input type
	Value    any      // Current value
	Required bool     // Is required
	ReadOnly bool     // Is read-only
	Label    string   // Human-readable label
	Help     string   // Help text
	Options  []Option // Enumerated choices
	Error    string   // Validation error message
	Accept   string   // Accepted MIME types (file fields), e.g. "image/*" or "image/jpeg,image/png"
	MaxSize  int64    // Maximum file size in bytes (file fields), 0 means no limit
	Multiple bool     // Whether the field accepts multiple files (file fields)
}

// Option represents an enumerated choice for a Field.
type Option struct {
	Value    string
	Label    string
	Selected bool
}

// Target is an abstract target designation.
type Target struct {
	URL   *url.URL   // Directly specified URL
	Route *RouteRef  // Named route reference
	Query url.Values // Query parameters
}

// RouteRef is a named route reference.
type RouteRef struct {
	Name   string
	Params map[string]string
	Query  url.Values
}

// Resolver resolves targets to URLs.
type Resolver interface {
	ResolveTarget(context.Context, Target) (*url.URL, error)
}

// RenderMode controls how a codec renders a representation.
type RenderMode uint8

const (
	RenderDocument RenderMode = iota
	RenderFragment
)

// EncodeOptions are passed to RepresentationCodec.Encode.
type EncodeOptions struct {
	Request  *http.Request
	Resolver Resolver
	Mode     RenderMode
}

// DecodeOptions are passed to SubmissionCodec.Decode.
type DecodeOptions struct {
	Request *http.Request
}

// RepresentationCodec encodes representations to a wire format.
type RepresentationCodec interface {
	MediaTypes() []string
	Encode(context.Context, io.Writer, Representation, EncodeOptions) error
}

// RepresentationDecoder decodes response bodies into Representations.
// Codecs that support both encoding and decoding implement both
// RepresentationCodec and RepresentationDecoder.
type RepresentationDecoder interface {
	MediaTypes() []string
	DecodeRepresentation(context.Context, io.Reader) (Representation, error)
}

// StreamingCodec extends RepresentationCodec with the ability to write
// a sequence of representations as a stream.
type StreamingCodec interface {
	RepresentationCodec
	EncodeEvent(context.Context, io.Writer, Representation, EncodeOptions) error
	Flush(io.Writer) error
}

// SubmissionCodec decodes submission bodies.
type SubmissionCodec interface {
	MediaTypes() []string
	Decode(context.Context, io.Reader, any, DecodeOptions) error
}
