package hyper

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
)

type formSubCodec struct{}

// FormSubmissionCodec returns a SubmissionCodec for application/x-www-form-urlencoded bodies.
func FormSubmissionCodec() SubmissionCodec { return formSubCodec{} }

func (formSubCodec) MediaTypes() []string { return []string{"application/x-www-form-urlencoded"} }

// Encode serializes values as application/x-www-form-urlencoded.
func (formSubCodec) Encode(values map[string]any) (io.Reader, error) {
	form := url.Values{}
	for k, v := range values {
		form.Set(k, fmt.Sprintf("%v", v))
	}
	return bytes.NewBufferString(form.Encode()), nil
}

// Decode parses application/x-www-form-urlencoded data into a *map[string]any target.
func (formSubCodec) Decode(_ context.Context, r io.Reader, dst any, _ DecodeOptions) error {
	target, ok := dst.(*map[string]any)
	if !ok {
		return fmt.Errorf("form: Decode target must be *map[string]any, got %T", dst)
	}

	body, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("form: read body: %w", err)
	}

	parsed, err := url.ParseQuery(string(body))
	if err != nil {
		return fmt.Errorf("form: parse: %w", err)
	}

	result := make(map[string]any, len(parsed))
	for k, v := range parsed {
		if len(v) == 1 {
			result[k] = v[0]
		} else {
			result[k] = v
		}
	}
	*target = result
	return nil
}
