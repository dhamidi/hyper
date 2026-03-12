package hyper

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// EventStreamCodec implements StreamingCodec for the text/event-stream
// media type (Server-Sent Events). Each SSE event carries a single
// hyper Representation encoded as JSON.
type EventStreamCodec struct{}

// MediaTypes returns the SSE media type.
func (EventStreamCodec) MediaTypes() []string {
	return []string{"text/event-stream"}
}

// Encode writes a single representation as one SSE event and terminates
// the stream. This satisfies RepresentationCodec for non-streaming use.
func (c EventStreamCodec) Encode(ctx context.Context, w io.Writer, rep Representation, opts EncodeOptions) error {
	if err := c.EncodeEvent(ctx, w, rep, opts); err != nil {
		return err
	}
	return c.Flush(w)
}

// EncodeEvent writes one SSE event without closing the stream.
// The event: field is set to Representation.Kind, the data: field
// contains the JSON-encoded representation, and the id: field is
// set from Meta["eventID"] if present.
func (c EventStreamCodec) EncodeEvent(ctx context.Context, w io.Writer, rep Representation, opts EncodeOptions) error {
	// Encode representation as JSON.
	jsonData, err := encodeRepresentation(ctx, rep, opts)
	if err != nil {
		return fmt.Errorf("event-stream: encode representation: %w", err)
	}

	raw, err := json.Marshal(jsonData)
	if err != nil {
		return fmt.Errorf("event-stream: marshal json: %w", err)
	}

	// Write event: field if Kind is set.
	if rep.Kind != "" {
		if _, err := fmt.Fprintf(w, "event: %s\n", rep.Kind); err != nil {
			return fmt.Errorf("event-stream: write event field: %w", err)
		}
	}

	// Write id: field if Meta["eventID"] is set.
	if rep.Meta != nil {
		if id, ok := rep.Meta["eventID"].(string); ok && id != "" {
			if _, err := fmt.Fprintf(w, "id: %s\n", id); err != nil {
				return fmt.Errorf("event-stream: write id field: %w", err)
			}
		}
	}

	// Write data: field(s). Each line of the JSON payload gets its own
	// data: prefix per the SSE spec.
	dataStr := string(raw)
	lines := strings.Split(dataStr, "\n")
	for _, line := range lines {
		if _, err := fmt.Fprintf(w, "data: %s\n", line); err != nil {
			return fmt.Errorf("event-stream: write data field: %w", err)
		}
	}

	// Blank line terminates the event.
	if _, err := fmt.Fprint(w, "\n"); err != nil {
		return fmt.Errorf("event-stream: write event terminator: %w", err)
	}

	return nil
}

// Flush ensures buffered data reaches the client. If w implements
// *bufio.Writer it calls Flush; otherwise it is a no-op.
func (EventStreamCodec) Flush(w io.Writer) error {
	if bw, ok := w.(*bufio.Writer); ok {
		return bw.Flush()
	}
	return nil
}

// DecodeEvent reads a single SSE event from the reader and decodes the
// data payload into a Representation. The event: field is mapped to
// Representation.Kind.
func DecodeEvent(r *bufio.Reader) (Representation, error) {
	var kind string
	var eventID string
	var dataLines []string

	for {
		line, err := r.ReadString('\n')
		if err != nil && len(line) == 0 {
			if err == io.EOF && len(dataLines) > 0 {
				break // process what we have
			}
			return Representation{}, err
		}

		line = strings.TrimRight(line, "\r\n")

		// Blank line signals end of event.
		if line == "" {
			if len(dataLines) > 0 {
				break
			}
			continue // skip leading blank lines
		}

		// Comment lines start with ':'
		if strings.HasPrefix(line, ":") {
			continue
		}

		field, value, _ := strings.Cut(line, ": ")
		switch field {
		case "event":
			kind = value
		case "id":
			eventID = value
		case "data":
			dataLines = append(dataLines, value)
		}
	}

	if len(dataLines) == 0 {
		return Representation{}, fmt.Errorf("event-stream: empty event data")
	}

	payload := strings.Join(dataLines, "\n")

	var raw map[string]any
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		return Representation{}, fmt.Errorf("event-stream: decode json: %w", err)
	}

	rep, err := decodeRepresentation(raw)
	if err != nil {
		return Representation{}, fmt.Errorf("event-stream: decode representation: %w", err)
	}

	// Override Kind from the event: field if present.
	if kind != "" {
		rep.Kind = kind
	}

	// Store eventID in Meta if present.
	if eventID != "" {
		if rep.Meta == nil {
			rep.Meta = make(map[string]any)
		}
		rep.Meta["eventID"] = eventID
	}

	return rep, nil
}
