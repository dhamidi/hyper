package jsonapi

import (
	"bytes"
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

// Encode serializes submission values as a JSON:API document.
// The values map is wrapped in a JSON:API document structure.
// Use the "_type" key to set the resource type and "_id" for the resource ID.
// All other keys become resource attributes.
func (c SubmissionCodec) Encode(values map[string]any) (io.Reader, error) {
	resourceType := "unknown"
	if t, ok := values["_type"].(string); ok {
		resourceType = t
	}
	res := &Resource{
		Type:       resourceType,
		Attributes: make(map[string]any),
	}
	for k, v := range values {
		if k == "_type" || k == "_id" {
			continue
		}
		res.Attributes[k] = v
	}
	if id, ok := values["_id"]; ok {
		res.ID = fmt.Sprintf("%v", id)
	}
	doc := Document{
		Data: PrimaryData{One: res},
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(doc); err != nil {
		return nil, fmt.Errorf("jsonapi: %w", err)
	}
	return &buf, nil
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
