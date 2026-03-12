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
//
// dst may be one of:
//   - *map[string]any: populated with the flattened submission from MapToSubmission
//   - *Submission: populated with the structured submission from MapSubmission
//
// For *map[string]any targets, the map uses underscore-prefixed keys for
// JSON:API structural members (_type, _relationships, _null, _relationship_data)
// to avoid conflicts with user attributes.
//
// For *Submission targets, all members are available as typed fields.
func (c SubmissionCodec) Decode(_ context.Context, r io.Reader, dst any, _ hyper.DecodeOptions) error {
	var doc Document
	if err := json.NewDecoder(r).Decode(&doc); err != nil {
		return fmt.Errorf("jsonapi: %w", err)
	}

	switch target := dst.(type) {
	case *map[string]any:
		result := MapToSubmission(doc)
		if result == nil {
			result = make(map[string]any)
		}
		*target = result
	case *Submission:
		result := MapSubmission(doc)
		if result == nil {
			result = &Submission{}
		}
		*target = *result
	default:
		return fmt.Errorf("jsonapi: Decode target must be *map[string]any or *Submission, got %T", dst)
	}
	return nil
}
