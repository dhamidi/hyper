package hyper

import (
	"bytes"
	"context"
	"io"
	"testing"
)

func TestFormSubmissionCodec_MediaTypes(t *testing.T) {
	codec := FormSubmissionCodec()
	mts := codec.MediaTypes()
	if len(mts) != 1 || mts[0] != "application/x-www-form-urlencoded" {
		t.Fatalf("MediaTypes() = %v, want [application/x-www-form-urlencoded]", mts)
	}
}

func TestFormSubmissionCodec_RoundTrip(t *testing.T) {
	codec := FormSubmissionCodec()

	input := map[string]any{
		"name": "Alice",
		"age":  "30",
	}

	// Encode
	r, err := codec.Encode(input)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	// Decode
	var got map[string]any
	if err := codec.Decode(context.Background(), r, &got, DecodeOptions{}); err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if got["name"] != "Alice" {
		t.Errorf("name = %v, want Alice", got["name"])
	}
	if got["age"] != "30" {
		t.Errorf("age = %v, want 30", got["age"])
	}
}

func TestFormSubmissionCodec_Encode(t *testing.T) {
	codec := FormSubmissionCodec()
	r, err := codec.Encode(map[string]any{"key": "value"})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	body, _ := io.ReadAll(r)
	if string(body) != "key=value" {
		t.Errorf("Encode = %q, want %q", body, "key=value")
	}
}

func TestFormSubmissionCodec_Decode_InvalidTarget(t *testing.T) {
	codec := FormSubmissionCodec()
	var s string
	err := codec.Decode(context.Background(), bytes.NewBufferString("k=v"), &s, DecodeOptions{})
	if err == nil {
		t.Fatal("expected error for invalid target type")
	}
}

func TestFormSubmissionCodec_Decode_MultipleValues(t *testing.T) {
	codec := FormSubmissionCodec()
	var got map[string]any
	err := codec.Decode(context.Background(), bytes.NewBufferString("tag=a&tag=b"), &got, DecodeOptions{})
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	tags, ok := got["tag"].([]string)
	if !ok || len(tags) != 2 {
		t.Fatalf("tag = %v (%T), want []string{a, b}", got["tag"], got["tag"])
	}
}
