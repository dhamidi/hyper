# Proposal 005: Streaming Refinements

- **Status:** Draft
- **Date:** 2026-03-17
- **Author:** hyper contributors

## 1. Problem Statement

Hyper's SSE streaming API works well for simple GET-based event streams: a client
calls `FetchStream`, reads representations from a channel, and the channel closes
when the stream ends. On the server side, `Renderer.RespondStream` accepts a
channel of representations and writes them as SSE events with automatic flushing.

However, the agent-streaming use case (`use-cases/agent-streaming.md`) revealed
several gaps when streaming is used for **POST-based interactions** (submitting a
prompt and receiving a streamed response), **error handling** (distinguishing
normal completion from failure), and **codec-level control** (SSE `retry`,
`event` type overrides). Specifically:

1. **Error signaling is absent.** Both `FetchStream` and `SubmitStream` return a
   bare `<-chan *Response` that closes on both success and error. The caller
   cannot distinguish "stream completed normally" from "stream terminated due to
   a network error or server failure" after the channel closes.

2. **`Kind` does double-duty for SSE event type.** `Representation.Kind` maps
   directly to the SSE `event:` field. When the application-level semantic kind
   (e.g., `"message"`) should differ from the SSE transport-level event type
   (e.g., `"agent-chunk"`), there is no override mechanism.

3. **No SSE `retry` directive support.** The SSE specification defines a `retry:`
   field that instructs the client how long to wait before reconnecting.
   `EventStreamCodec.EncodeEvent` does not emit `retry:`, and there is no way to
   set it through hyper.

4. **`http.Flusher` dependency is fragile.** `Renderer.RespondStream` checks for
   `http.Flusher` at request time, which means a misconfigured middleware stack
   only fails when a real streaming request arrives. There is no way to verify
   streaming capability at setup time.

5. **`Embedded` semantics in streaming are undefined.** The spec does not
   clarify whether intermediate chunk events should carry `Embedded`
   representations or whether `Embedded` should be reserved for final/complete
   events.

6. **`StreamingCodec.Encode` semantics need documentation.** The relationship
   between `Encode` (single-event-then-close) and `EncodeEvent` (one event in an
   ongoing stream) is implemented correctly but not documented.

## 2. Background

### 2.1 Current Streaming API Surface

**Server side:**

| Component | Signature | Role |
|---|---|---|
| `Renderer.RespondStream` | `(w, req, <-chan Representation) error` | Writes SSE events from a channel; handles flushing |
| `EventStreamCodec.EncodeEvent` | `(ctx, w, Representation, EncodeOptions) error` | Writes one SSE event (event/id/data fields) |
| `EventStreamCodec.Encode` | `(ctx, w, Representation, EncodeOptions) error` | Writes one event then flushes (single-event stream) |
| `EventStreamCodec.Flush` | `(w) error` | Flushes a `*bufio.Writer` if applicable |

**Client side:**

| Component | Signature | Role |
|---|---|---|
| `Client.FetchStream` | `(ctx, Target) (<-chan *Response, error)` | GET with `Accept: text/event-stream`; channel of decoded events |
| `Client.SubmitStream` | `(ctx, Action, map[string]any) (<-chan *Response, error)` | POST/PUT with `Accept: text/event-stream`; channel of decoded events |

**Wire format (current):**

```
event: agent-chunk
id: ev-3
data: {"kind":"agent-chunk","state":{"token":"Hello","accumulated":"Hello"}}

event: agent-chunk
id: ev-4
data: {"kind":"agent-chunk","state":{"token":" world","accumulated":"Hello world"}}

event: agent-response
id: final
data: {"kind":"agent-response","state":{"content":"Hello world"},"links":[...]}

```

### 2.2 Agent-Streaming Use Case Requirements

The agent-streaming use case (`use-cases/agent-streaming.md`) describes an AI
chat platform where a client POSTs a user prompt and receives a token-by-token
SSE stream. Key requirements from the spec feedback section:

- **POST + SSE:** The client submits form data and receives an SSE response
  (`SubmitStream` — already implemented).
- **Error detection:** The client needs to know whether the stream ended
  normally or was interrupted by an error.
- **Event type control:** Some representations have a semantic `Kind` that
  differs from the desired SSE event type.
- **Reconnection:** The SSE `retry:` directive should be settable to control
  client reconnection behavior.
- **Streaming `Embedded`:** Tool-call results arrive as `Embedded` on
  intermediate chunks, but the cost/benefit tradeoff is unclear.

### 2.3 SSE Specification Reference

The [Server-Sent Events specification](https://html.spec.whatwg.org/multipage/server-sent-events.html)
defines four field types per event:

| Field | Purpose |
|---|---|
| `event:` | Event type string; defaults to `"message"` if omitted |
| `data:` | Payload (one or more lines) |
| `id:` | Last event ID for reconnection |
| `retry:` | Reconnection time in milliseconds |

The `retry:` field is sent once (typically in the first event or as a standalone
field) and applies to all subsequent reconnection attempts.

## 3. Proposal

### 3.1 `Stream` Wrapper Type

Replace the raw `<-chan *Response` return type from `FetchStream` and
`SubmitStream` with a `Stream` wrapper that provides error inspection and
explicit close:

```go
// Stream wraps a channel of SSE responses with error reporting and
// lifecycle management.
type Stream struct {
    ch     <-chan *Response
    err    error
    cancel context.CancelFunc
    done   chan struct{}
}

// Events returns a read-only channel of Response values. The channel is
// closed when the stream ends (either normally or due to an error).
func (s *Stream) Events() <-chan *Response

// Err returns the error that caused the stream to close, or nil if the
// stream completed normally. Err blocks until the stream is closed.
func (s *Stream) Err() error

// Close terminates the stream early, cancelling the underlying request
// context and closing the Events channel.
func (s *Stream) Close()
```

**Updated client signatures:**

```go
func (c *Client) FetchStream(ctx context.Context, target Target) (*Stream, error)
func (c *Client) SubmitStream(ctx context.Context, action Action, values map[string]any) (*Stream, error)
```

The initial `error` return still covers request-setup failures (DNS, TLS,
non-SSE response). The `Stream.Err()` method covers runtime failures (network
interruption, malformed events, server-sent error events).

**Migration path:** This is a breaking change to the return type. Since hyper is
pre-1.0, this is acceptable. Callers that currently range over the channel
update from:

```go
// Before
ch, err := client.FetchStream(ctx, target)
for resp := range ch { ... }

// After
stream, err := client.FetchStream(ctx, target)
for resp := range stream.Events() { ... }
if err := stream.Err(); err != nil { ... }
```

### 3.2 `Hints["sse-event"]` Convention

Define `Hints["sse-event"]` (type `string`) as a Tier 2 standard hint (per
Proposal 004) that overrides the SSE `event:` field. When present,
`EventStreamCodec.EncodeEvent` emits the hint value as the `event:` field
instead of `Representation.Kind`.

```go
rep := hyper.Representation{
    Kind: "message",   // application-level semantic type
    Hints: map[string]any{
        "sse-event": "agent-chunk",  // SSE transport-level event type
    },
    State: hyper.Object{
        "token":       hyper.Scalar{V: "Hello"},
        "accumulated": hyper.Scalar{V: "Hello"},
    },
}
```

Wire output:

```
event: agent-chunk
data: {"kind":"message","state":{"token":"Hello","accumulated":"Hello"}}

```

On the decoding side, `DecodeEvent` already sets `Kind` from the `event:` field.
When `Hints["sse-event"]` is in use, the JSON payload retains the original
`Kind` while the SSE framing uses the override. Clients that need the SSE event
type can read it from the `Response.Header` or from the representation's hint.

**Codec implementation change in `EncodeEvent`:**

```go
eventType := rep.Kind
if sseEvent, ok := rep.Hints["sse-event"].(string); ok && sseEvent != "" {
    eventType = sseEvent
}
if eventType != "" {
    fmt.Fprintf(w, "event: %s\n", eventType)
}
```

### 3.3 `Hints["retry"]` for SSE Reconnection Interval

Define `Hints["retry"]` (type `int`, milliseconds) as a Tier 2 standard hint.
When present on a representation, `EventStreamCodec.EncodeEvent` emits a
`retry:` field before the `data:` field.

```go
rep := hyper.Representation{
    Kind: "stream-open",
    Hints: map[string]any{
        "retry": 3000,  // reconnect after 3 seconds
    },
    State: hyper.Object{
        "status": hyper.Scalar{V: "connected"},
    },
}
```

Wire output:

```
event: stream-open
retry: 3000
data: {"kind":"stream-open","state":{"status":"connected"}}

```

This is typically set only on the first event in a stream. The codec emits
`retry:` whenever the hint is present; the application controls when to include
it.

### 3.4 `Renderer.CanStream` Helper

Add a helper method to `Renderer` that checks whether streaming is possible
before committing to the streaming code path:

```go
// CanStream reports whether the given ResponseWriter supports SSE streaming.
// It checks that the writer implements http.Flusher and that a StreamingCodec
// for "text/event-stream" is registered.
func (r Renderer) CanStream(w http.ResponseWriter) bool
```

This lets handlers degrade gracefully:

```go
if renderer.CanStream(w) {
    renderer.RespondStream(w, r, reps)
} else {
    // Fall back to polling-style single response.
    renderer.Respond(w, r, http.StatusOK, finalRep, hyper.RenderDocument)
}
```

### 3.5 `Embedded` in Streaming Contexts

Establish the following convention for `Embedded` in streaming:

- **Chunk events** (intermediate, partial results) SHOULD NOT carry `Embedded`
  representations. Chunks are lightweight deltas meant for real-time display;
  adding embedded sub-resources inflates payload size and creates ambiguity
  about whether embedded data is partial or complete.

- **Completion events** (final, full results like `agent-response`) MAY carry
  `Embedded` representations. At stream end, the representation is complete and
  embedded sub-resources (e.g., tool-call results, related entities) are final.

- **Error events** (`stream-error`) SHOULD NOT carry `Embedded`. Error events
  should be self-contained with their error information in `State`.

If an intermediate event must reference a sub-resource, prefer a `Link` over
`Embedded` — the client can fetch the full sub-resource if needed.

### 3.6 `StreamingCodec.Encode` Semantics

Document explicitly that `StreamingCodec.Encode` (inherited from
`RepresentationCodec`) writes a **single-event stream**: one SSE event followed
by a flush. This is the correct behavior for responses that happen to use the
`text/event-stream` content type but contain only one event (e.g., a non-streaming
fallback).

In contrast, `EncodeEvent` writes one event in an **ongoing stream** — no
flush, no stream termination. The caller (typically `Renderer.RespondStream`)
is responsible for flushing after each event and closing the stream when the
channel closes.

Add a doc comment to the `StreamingCodec` interface:

```go
// StreamingCodec extends RepresentationCodec for event-stream media types.
//
// Encode writes a single-event stream: one event followed by a flush. Use
// this when the response contains exactly one SSE event.
//
// EncodeEvent writes one event within an ongoing stream. The caller must
// call Flush after each event and is responsible for stream lifecycle.
type StreamingCodec interface {
    RepresentationCodec
    EncodeEvent(ctx context.Context, w io.Writer, rep Representation, opts EncodeOptions) error
    Flush(w io.Writer) error
}
```

## 4. Examples

### 4.1 `SubmitStream` with `Stream` Wrapper

```go
ctx := context.Background()

// Submit a prompt and receive a streaming response.
stream, err := client.SubmitStream(ctx, submitAction, map[string]any{
    "content": "Explain hypermedia in one paragraph.",
})
if err != nil {
    log.Fatalf("start stream: %v", err)
}
defer stream.Close()

for resp := range stream.Events() {
    switch resp.Representation.Kind {
    case "stream-open":
        fmt.Println("Stream started")
    case "agent-chunk":
        token := resp.Representation.State.(hyper.Object)["token"]
        fmt.Print(token.(hyper.Scalar).V)
    case "agent-response":
        fmt.Println("\n--- Stream complete ---")
    case "stream-error":
        msg := resp.Representation.State.(hyper.Object)["message"]
        fmt.Fprintf(os.Stderr, "Server error: %s\n", msg.(hyper.Scalar).V)
    }
}

// Check for transport-level errors (network failure, malformed SSE).
if err := stream.Err(); err != nil {
    log.Fatalf("stream error: %v", err)
}
```

### 4.2 Wire Format with `retry` and `sse-event` Overrides

```
event: stream-open
retry: 3000
id: ev-1
data: {"kind":"stream-open","state":{"status":"connected","model":"gpt-4"}}

event: agent-chunk
id: ev-2
data: {"kind":"message","state":{"token":"Hyper","accumulated":"Hyper"}}

event: agent-chunk
id: ev-3
data: {"kind":"message","state":{"token":"media","accumulated":"Hypermedia"}}

event: agent-response
id: final
data: {"kind":"agent-response","state":{"content":"Hypermedia is..."},"links":[{"rel":"self","href":"/agents/a1/conversations/c42/messages/m7"}]}

```

Note that `kind` in the JSON payload is `"message"` while the SSE `event:` field
is `"agent-chunk"` — this is the `Hints["sse-event"]` override in action.

### 4.3 Server-Side `CanStream` Fallback

```go
func handlePrompt(w http.ResponseWriter, r *http.Request) {
    if renderer.CanStream(w) {
        reps := make(chan hyper.Representation)
        go generateTokenStream(r.Context(), reps)
        renderer.RespondStream(w, r, reps)
    } else {
        // Client doesn't support streaming or middleware wraps the writer.
        result := generateFullResponse(r.Context())
        result.Meta = map[string]any{"poll-interval": 2}
        renderer.Respond(w, r, http.StatusOK, result, hyper.RenderDocument)
    }
}
```

## 5. Alternatives Considered

### 5.1 Error Channel vs Stream Wrapper

**Error channel approach:**

```go
func (c *Client) FetchStream(ctx, target) (<-chan *Response, <-chan error, error)
```

Pros: No new type; channels are idiomatic Go.

Cons: Three return values are awkward. The caller must select on both channels
simultaneously to avoid missing the error. There is no `Close()` mechanism for
early termination (requires context cancellation at the call site). Two channels
that interact create ordering concerns: does the error channel close before or
after the response channel?

**Decision:** The `Stream` wrapper is preferred. It provides a single return value
with a clear lifecycle (`Events` → `Err` → `Close`), avoids the dual-select
problem, and is extensible if future fields are needed (e.g., stream metadata).

### 5.2 `RenderStream` Mode vs Documenting `RenderDocument`

**Adding `RenderStream`:**

```go
const RenderStream RenderMode = 2
```

Pros: Codecs could optimize encoding for stream context (e.g., omit redundant
fields in chunk events).

Cons: Adds a third mode to every codec's `Encode` method. The current two modes
(`RenderDocument` and `RenderFragment`) have clear semantics — document is a
full page, fragment is a partial update. A stream event is conceptually a
complete, self-contained representation (it has `Kind`, `Self`, `State`, etc.),
making `RenderDocument` semantically correct. Adding `RenderStream` for a
theoretical optimization that no codec currently needs violates YAGNI.

**Decision:** Document that `RenderDocument` is the correct mode for stream
events. Each SSE event carries a self-contained representation. If a future
codec needs stream-specific behavior, `RenderStream` can be added at that time.

### 5.3 `Meta` vs `Hints` for SSE Fields

The original agent-streaming feedback proposed `Meta["sse-event"]` and
`Meta["retry"]` for SSE field overrides. This proposal uses `Hints` instead.

**Rationale:** `Meta` carries opaque metadata that is serialized into the JSON
payload and round-trips through encode/decode. SSE `retry:` and `event:` are
transport-level directives that should not appear in the JSON `data:` field.
`Hints` are codec directives that influence encoding but are not themselves
serialized into the output — exactly the right semantics for transport-level
SSE fields.

## 6. Open Questions

1. **Should `Stream` support backpressure?** The current design uses an unbuffered
   channel, which provides natural backpressure — the server blocks if the client
   is not reading. Should `Stream` expose a way to configure the channel buffer
   size, or is unbuffered always correct for SSE?

2. **Should there be a `StreamingSubmissionCodec`?** The current `SubmissionCodec`
   interface encodes form values for the request body. A streaming submission
   (e.g., uploading chunks) would need a different interface. This is not needed
   for the agent-streaming use case (which submits a single prompt), but may be
   relevant for file-upload streaming.

3. **Should `Stream.Err()` distinguish error types?** For example, should there
   be sentinel errors like `ErrStreamReset` (server closed unexpectedly) vs
   `ErrStreamTimeout` (context deadline)? Or is wrapping the underlying error
   sufficient?

4. **Should `DecodeEvent` interpret `retry:` fields?** Currently, `DecodeEvent`
   parses `event:`, `id:`, and `data:` but ignores `retry:`. Should it store
   the retry value somewhere (e.g., `Meta["retry"]`) so that client-side code
   can respect it?

5. **Should `Renderer.RespondStream` accept a `context.Context` parameter?**
   It currently uses `req.Context()`, which is correct. However, an explicit
   context parameter would let callers set stream-specific deadlines independent
   of the request context.
