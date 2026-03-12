package jsonapi

import (
	"context"
	"encoding/json"
	"io"

	"github.com/dhamidi/hyper"
)

// RepresentationCodec encodes hyper.Representation values as JSON:API documents.
// It implements hyper.RepresentationCodec.
type RepresentationCodec struct{}

// MediaTypes returns the media types supported by this codec.
func (c RepresentationCodec) MediaTypes() []string {
	return []string{"application/vnd.api+json"}
}

// Encode writes the representation as a JSON:API document to w.
func (c RepresentationCodec) Encode(_ context.Context, w io.Writer, rep hyper.Representation, opts hyper.EncodeOptions) error {
	doc := MapRepresentation(rep, opts.Resolver)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(doc)
}
