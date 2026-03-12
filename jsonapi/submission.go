package jsonapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/dhamidi/hyper"
)

// SubmissionCodec decodes JSON:API request bodies into target values.
// It implements hyper.SubmissionCodec.
type SubmissionCodec struct{}

// MediaTypes returns the media types supported by this codec.
func (c SubmissionCodec) MediaTypes() []string {
	return []string{"application/vnd.api+json"}
}

// Decode reads a JSON:API document from r and populates dst.
// dst must be a *map[string]any; the map is populated with the flattened
// attributes from the JSON:API data object plus the "id" if present.
func (c SubmissionCodec) Decode(_ context.Context, r io.Reader, dst any, _ hyper.DecodeOptions) error {
	target, ok := dst.(*map[string]any)
	if !ok {
		return fmt.Errorf("jsonapi: Decode target must be *map[string]any, got %T", dst)
	}

	var doc Document
	if err := json.NewDecoder(r).Decode(&doc); err != nil {
		return fmt.Errorf("jsonapi: %w", err)
	}

	result := MapToSubmission(doc)
	if result == nil {
		result = make(map[string]any)
	}
	*target = result
	return nil
}
