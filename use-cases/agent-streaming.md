# Use Case: AI Agent Streaming via SSE

This document explores how `hyper` supports an AI chat application where a server streams token-by-token responses as Server-Sent Events. It exercises the proposed `text/event-stream` support end-to-end — from resource discovery through streaming, error handling, and reconnection — and identifies gaps in the current spec.

## Scenario

An AI chat platform exposes agents that hold conversations. A user submits a prompt, and the server streams back the response as a sequence of SSE events. Each event is a full `hyper.Representation`, preserving hypermedia controls throughout the stream. Clients that do not support SSE fall back to a polling-based path using the same action.

## Actors & Resources

| Resource | Path | Description |
|----------|------|-------------|
| Agent | `/agents/{id}` | An AI agent with a model, system prompt, and conversation history |
| Conversation | `/agents/{id}/conversations/{cid}` | A conversation thread within an agent |
| Message | `/agents/{id}/conversations/{cid}/messages/{mid}` | An individual message (user or assistant) |

## Interaction 1: Fetch Agent

The client discovers an agent by following a link from the API root.

### Go code

```go
agentRep := hyper.Representation{
    Kind: "agent",
    Self: hyper.Path("agents", "a1").Ptr(),
    State: hyper.Object{
        "name":   hyper.Scalar{V: "CodeAssist"},
        "model":  hyper.Scalar{V: "gpt-4o"},
        "status": hyper.Scalar{V: "ready"},
    },
    Links: []hyper.Link{
        {Rel: "conversations", Target: hyper.Path("agents", "a1", "conversations"), Title: "Conversations"},
    },
    Actions: []hyper.Action{
        {
            Name:   "StartConversation",
            Rel:    "create",
            Method: "POST",
            Target: hyper.Path("agents", "a1", "conversations"),
            Fields: []hyper.Field{
                {Name: "title", Type: "text", Label: "Conversation title"},
            },
        },
    },
}
```

### JSON wire format

```json
{
  "kind": "agent",
  "self": {"href": "/agents/a1"},
  "state": {
    "name": "CodeAssist",
    "model": "gpt-4o",
    "status": "ready"
  },
  "links": [
    {"rel": "conversations", "href": "/agents/a1/conversations", "title": "Conversations"}
  ],
  "actions": [
    {
      "name": "StartConversation",
      "rel": "create",
      "method": "POST",
      "href": "/agents/a1/conversations",
      "fields": [
        {"name": "title", "type": "text", "label": "Conversation title"}
      ]
    }
  ]
}
```

**Types exercised:** `Representation`, `Object`, `Scalar`, `Link`, `Action`, `Field`, `Target`.

## Interaction 2: Start Conversation

The client submits the `StartConversation` action. The server returns a new conversation representation.

### Go code

```go
convRep := hyper.Representation{
    Kind: "conversation",
    Self: hyper.Path("agents", "a1", "conversations", "c42").Ptr(),
    State: hyper.Object{
        "title":     hyper.Scalar{V: "Help with Go generics"},
        "createdAt": hyper.Scalar{V: "2026-03-12T10:00:00Z"},
        "messages":  hyper.Scalar{V: 0},
    },
    Links: []hyper.Link{
        {Rel: "agent", Target: hyper.Path("agents", "a1"), Title: "CodeAssist"},
        {Rel: "messages", Target: hyper.Path("agents", "a1", "conversations", "c42", "messages")},
    },
    Actions: []hyper.Action{
        {
            Name:     "SubmitPrompt",
            Rel:      "create",
            Method:   "POST",
            Target:   hyper.Path("agents", "a1", "conversations", "c42", "messages"),
            Produces: []string{"text/event-stream", "application/json"},
            Fields: []hyper.Field{
                {Name: "content", Type: "textarea", Required: true, Label: "Your message"},
            },
            Hints: map[string]any{
                "async":  true,
                "stream": true,
            },
        },
    },
}
```

### JSON wire format

```json
{
  "kind": "conversation",
  "self": {"href": "/agents/a1/conversations/c42"},
  "state": {
    "title": "Help with Go generics",
    "createdAt": "2026-03-12T10:00:00Z",
    "messages": 0
  },
  "links": [
    {"rel": "agent", "href": "/agents/a1", "title": "CodeAssist"},
    {"rel": "messages", "href": "/agents/a1/conversations/c42/messages"}
  ],
  "actions": [
    {
      "name": "SubmitPrompt",
      "rel": "create",
      "method": "POST",
      "href": "/agents/a1/conversations/c42/messages",
      "produces": ["text/event-stream", "application/json"],
      "fields": [
        {"name": "content", "type": "textarea", "required": true, "label": "Your message"}
      ],
      "hints": {
        "async": true,
        "stream": true
      }
    }
  ]
}
```

**Types exercised:** `Representation`, `Object`, `Scalar`, `Link`, `Action`, `Field`, `Hints`, `Action.Produces`.

The `SubmitPrompt` action advertises both `text/event-stream` and `application/json` in `Produces`, signaling that the client can choose between streaming and non-streaming responses via content negotiation. The `Hints` map carries `async: true` and `stream: true` so that UI clients can render appropriate affordances (e.g., a streaming indicator).

## Interaction 3: Submit Prompt (Non-Streaming)

A client that does not support SSE submits the prompt with `Accept: application/json`. The server returns an accepted status with a polling hint.

### Go code

```go
pendingRep := hyper.Representation{
    Kind: "message",
    Self: hyper.Path("agents", "a1", "conversations", "c42", "messages", "m7").Ptr(),
    State: hyper.Object{
        "role":    hyper.Scalar{V: "assistant"},
        "status":  hyper.Scalar{V: "generating"},
        "content": hyper.Scalar{V: ""},
    },
    Links: []hyper.Link{
        {Rel: "conversation", Target: hyper.Path("agents", "a1", "conversations", "c42")},
    },
    Meta: map[string]any{
        "poll-interval": 2,
    },
}
```

### JSON wire format

```json
{
  "kind": "message",
  "self": {"href": "/agents/a1/conversations/c42/messages/m7"},
  "state": {
    "role": "assistant",
    "status": "generating",
    "content": ""
  },
  "links": [
    {"rel": "conversation", "href": "/agents/a1/conversations/c42"}
  ],
  "meta": {
    "poll-interval": 2
  }
}
```

The client polls `GET /agents/a1/conversations/c42/messages/m7` at the interval specified in `Meta["poll-interval"]` until `state.status` becomes `"complete"`.

**Types exercised:** `Representation`, `Object`, `Scalar`, `Link`, `Meta`.

## Interaction 4: Submit Prompt (Streaming)

The same `SubmitPrompt` action, but the client sends `Accept: text/event-stream`. The server responds with a stream of SSE events, each containing a JSON-encoded `Representation`.

### Server handler

```go
func handleSubmitPrompt(w http.ResponseWriter, r *http.Request) {
    renderer := hyper.Renderer{
        Codecs: []hyper.RepresentationCodec{
            hyper.EventStreamCodec{},
            hyper.JSONCodec(),
        },
    }

    if r.Header.Get("Accept") == "text/event-stream" {
        streamPromptResponse(w, r, renderer)
        return
    }

    // Non-streaming fallback (Interaction 3)
    renderer.Respond(w, r, http.StatusAccepted, pendingRep)
}

func streamPromptResponse(w http.ResponseWriter, r *http.Request, renderer hyper.Renderer) {
    reps := make(chan hyper.Representation)

    go func() {
        defer close(reps)

        // Event 1: stream-open
        reps <- hyper.Representation{
            Kind: "stream-open",
            State: hyper.Object{
                "status": hyper.Scalar{V: "generating"},
            },
            Links: []hyper.Link{
                {Rel: "cancel", Target: hyper.Path("agents", "a1", "conversations", "c42", "messages", "m7", "cancel")},
            },
            Actions: []hyper.Action{
                {
                    Name:   "Cancel",
                    Rel:    "cancel",
                    Method: "POST",
                    Target: hyper.Path("agents", "a1", "conversations", "c42", "messages", "m7", "cancel"),
                },
            },
            Meta: map[string]any{"eventID": "0"},
        }

        // Events 2..N: agent-chunk with accumulated text
        tokens := generateTokens(r.Context()) // returns []string
        var accumulated strings.Builder
        for i, token := range tokens {
            accumulated.WriteString(token)
            reps <- hyper.Representation{
                Kind: "agent-chunk",
                State: hyper.Object{
                    "delta":       hyper.Scalar{V: token},
                    "accumulated": hyper.Scalar{V: accumulated.String()},
                    "tokenIndex":  hyper.Scalar{V: i},
                },
                Meta: map[string]any{"eventID": fmt.Sprintf("%d", i+1)},
            }
        }

        // Final event: agent-response with complete state
        reps <- hyper.Representation{
            Kind: "agent-response",
            Self: hyper.Path("agents", "a1", "conversations", "c42", "messages", "m7").Ptr(),
            State: hyper.Object{
                "role":    hyper.Scalar{V: "assistant"},
                "status":  hyper.Scalar{V: "complete"},
                "content": hyper.RichText{MediaType: "text/markdown", Source: accumulated.String()},
            },
            Links: []hyper.Link{
                {Rel: "self", Target: hyper.Path("agents", "a1", "conversations", "c42", "messages", "m7")},
                {Rel: "conversation", Target: hyper.Path("agents", "a1", "conversations", "c42")},
            },
            Actions: []hyper.Action{
                {
                    Name:     "FollowUp",
                    Rel:      "create",
                    Method:   "POST",
                    Target:   hyper.Path("agents", "a1", "conversations", "c42", "messages"),
                    Produces: []string{"text/event-stream", "application/json"},
                    Fields: []hyper.Field{
                        {Name: "content", Type: "textarea", Required: true, Label: "Your message"},
                    },
                    Hints: map[string]any{"async": true, "stream": true},
                },
            },
            Embedded: map[string][]hyper.Representation{
                "tool-calls": {
                    {
                        Kind: "tool-result",
                        State: hyper.Object{
                            "tool":   hyper.Scalar{V: "code-search"},
                            "query":  hyper.Scalar{V: "Go generics constraints"},
                            "status": hyper.Scalar{V: "success"},
                        },
                    },
                },
            },
            Meta: map[string]any{"eventID": "final"},
        }
    }()

    renderer.RespondStream(w, r, reps)
}
```

### SSE wire format: stream-open

```
event: stream-open
id: 0
data: {"kind":"stream-open","state":{"status":"generating"},"links":[{"rel":"cancel","href":"/agents/a1/conversations/c42/messages/m7/cancel"}],"actions":[{"name":"Cancel","rel":"cancel","method":"POST","href":"/agents/a1/conversations/c42/messages/m7/cancel"}]}

```

### SSE wire format: agent-chunk

```
event: agent-chunk
id: 1
data: {"kind":"agent-chunk","state":{"delta":"Hello","accumulated":"Hello","tokenIndex":0}}

event: agent-chunk
id: 2
data: {"kind":"agent-chunk","state":{"delta":", I can","accumulated":"Hello, I can","tokenIndex":1}}

event: agent-chunk
id: 3
data: {"kind":"agent-chunk","state":{"delta":" help with","accumulated":"Hello, I can help with","tokenIndex":2}}

```

### SSE wire format: agent-response (final)

```
event: agent-response
id: final
data: {"kind":"agent-response","self":{"href":"/agents/a1/conversations/c42/messages/m7"},"state":{"role":"assistant","status":"complete","content":{"_type":"richtext","mediaType":"text/markdown","source":"Hello, I can help with Go generics..."}},"links":[{"rel":"self","href":"/agents/a1/conversations/c42/messages/m7"},{"rel":"conversation","href":"/agents/a1/conversations/c42"}],"actions":[{"name":"FollowUp","rel":"create","method":"POST","href":"/agents/a1/conversations/c42/messages","produces":["text/event-stream","application/json"],"fields":[{"name":"content","type":"textarea","required":true,"label":"Your message"}],"hints":{"async":true,"stream":true}}],"embedded":{"tool-calls":[{"kind":"tool-result","state":{"tool":"code-search","query":"Go generics constraints","status":"success"}}]}}

```

### Client code

```go
client, _ := hyper.NewClient("http://localhost:8080",
    hyper.WithCodec(hyper.EventStreamCodec{}),
    hyper.WithAccept("text/event-stream"),
)

// Submit the prompt action from Interaction 2
ch, err := client.FetchStream(ctx, action.Target)
if err != nil {
    log.Fatal(err)
}

var fullText strings.Builder
for resp := range ch {
    rep := resp.Representation
    switch rep.Kind {
    case "stream-open":
        fmt.Println("Stream started. Cancel available:", len(rep.Actions) > 0)
    case "agent-chunk":
        if state, ok := rep.State.(hyper.Object); ok {
            if delta, ok := state["delta"].(hyper.Scalar); ok {
                fmt.Print(delta.V)
                fullText.WriteString(fmt.Sprint(delta.V))
            }
        }
    case "agent-response":
        fmt.Println("\n--- Complete ---")
        for _, link := range rep.Links {
            fmt.Printf("  Link [%s]: %s\n", link.Rel, link.Target.URL)
        }
        for _, action := range rep.Actions {
            fmt.Printf("  Action [%s]: %s %s\n", action.Name, action.Method, action.Target.URL)
        }
        if toolCalls, ok := rep.Embedded["tool-calls"]; ok {
            for _, tc := range toolCalls {
                fmt.Printf("  Tool call: %v\n", tc.State)
            }
        }
    case "stream-error":
        if state, ok := rep.State.(hyper.Object); ok {
            fmt.Printf("Error: %v\n", state["message"])
        }
    }
}
```

**Types exercised:** `Representation`, `Object`, `Scalar`, `RichText`, `Link`, `Action`, `Field`, `Hints`, `Embedded`, `Meta`, `Action.Produces`, `StreamingCodec`.

## Interaction 5: Error During Stream

If an error occurs mid-stream (e.g., the model backend fails), the server emits a `stream-error` event with error details and a retry action.

### Go code

```go
errorRep := hyper.Representation{
    Kind: "stream-error",
    State: hyper.Object{
        "code":    hyper.Scalar{V: "model_unavailable"},
        "message": hyper.Scalar{V: "The model backend is temporarily unavailable"},
    },
    Actions: []hyper.Action{
        {
            Name:   "Retry",
            Rel:    "retry",
            Method: "POST",
            Target: hyper.Path("agents", "a1", "conversations", "c42", "messages"),
            Fields: []hyper.Field{
                {Name: "content", Type: "textarea", Required: true, Value: "Help with Go generics"},
            },
            Hints: map[string]any{"async": true, "stream": true},
        },
    },
    Meta: map[string]any{"eventID": "error-1"},
}
```

### SSE wire format

```
event: stream-error
id: error-1
data: {"kind":"stream-error","state":{"code":"model_unavailable","message":"The model backend is temporarily unavailable"},"actions":[{"name":"Retry","rel":"retry","method":"POST","href":"/agents/a1/conversations/c42/messages","fields":[{"name":"content","type":"textarea","required":true,"value":"Help with Go generics"}],"hints":{"async":true,"stream":true}}]}

```

The error representation carries a retry `Action` pre-populated with the original prompt, so the client can resubmit without re-constructing the request from scratch.

**Types exercised:** `Representation`, `Object`, `Scalar`, `Action`, `Field`, `Hints`, `Meta`.

## Interaction 6: Client Reconnection

SSE supports automatic reconnection via the `Last-Event-ID` header. When a client reconnects after a disconnect, it sends the last received event ID. The server uses this to resume the stream from the correct position.

### Client-side reconnection (browser)

```javascript
const source = new EventSource('/agents/a1/conversations/c42/messages', {
    // Browser automatically sends Last-Event-ID on reconnection
});

source.addEventListener('agent-chunk', (e) => {
    // e.lastEventId is set automatically by the browser
    const rep = JSON.parse(e.data);
    document.getElementById('output').textContent = rep.state.accumulated;
    // Using accumulated text (not delta) makes reconnection idempotent —
    // the client always has the full text up to the current event.
});
```

### Server-side reconnection handling

```go
func streamWithReconnection(w http.ResponseWriter, r *http.Request, renderer hyper.Renderer) {
    lastEventID := r.Header.Get("Last-Event-ID")

    reps := make(chan hyper.Representation)
    go func() {
        defer close(reps)

        tokens, startIdx := resumeGeneration(lastEventID)
        var accumulated strings.Builder
        // Rebuild accumulated state up to the resume point
        accumulated.WriteString(getAccumulatedText(lastEventID))

        for i, token := range tokens {
            accumulated.WriteString(token)
            reps <- hyper.Representation{
                Kind: "agent-chunk",
                State: hyper.Object{
                    "delta":       hyper.Scalar{V: token},
                    "accumulated": hyper.Scalar{V: accumulated.String()},
                    "tokenIndex":  hyper.Scalar{V: startIdx + i},
                },
                Meta: map[string]any{"eventID": fmt.Sprintf("%d", startIdx+i+1)},
            }
        }

        // Final event (same as Interaction 4)
        reps <- hyper.Representation{
            Kind: "agent-response",
            Self: hyper.Path("agents", "a1", "conversations", "c42", "messages", "m7").Ptr(),
            State: hyper.Object{
                "role":    hyper.Scalar{V: "assistant"},
                "status":  hyper.Scalar{V: "complete"},
                "content": hyper.RichText{MediaType: "text/markdown", Source: accumulated.String()},
            },
            Links: []hyper.Link{
                {Rel: "conversation", Target: hyper.Path("agents", "a1", "conversations", "c42")},
            },
            Meta: map[string]any{"eventID": "final"},
        }
    }()

    renderer.RespondStream(w, r, reps)
}
```

The `accumulated` field in each `agent-chunk` state makes reconnection safe: a reconnecting client replaces its buffer with the accumulated text rather than appending deltas, avoiding duplication. The `Meta["eventID"]` on every event maps directly to the SSE `id:` field, enabling the browser's automatic `Last-Event-ID` reconnection.

**Types exercised:** `Representation`, `Object`, `Scalar`, `RichText`, `Link`, `Meta`.

## Progressive Enhancement

The `SubmitPrompt` action supports both modes through content negotiation:

```go
submitAction := hyper.Action{
    Name:     "SubmitPrompt",
    Rel:      "create",
    Method:   "POST",
    Target:   hyper.Path("agents", "a1", "conversations", "c42", "messages"),
    Produces: []string{"text/event-stream", "application/json"},
    Fields: []hyper.Field{
        {Name: "content", Type: "textarea", Required: true, Label: "Your message"},
    },
    Hints: map[string]any{
        "async":  true,
        "stream": true,
    },
}
```

| Client sends | Server returns | Delivery |
|---|---|---|
| `Accept: text/event-stream` | SSE stream of representations | Real-time token streaming |
| `Accept: application/json` (or omitted) | Single `Representation` with `Meta["poll-interval"]` | Polling |

Both paths return full `Representation` values — the hypermedia contract is preserved regardless of delivery mechanism.

## Spec Feedback

### `RepresentationCodec.Encode` vs `StreamingCodec`

The current `RepresentationCodec.Encode` signature writes a single representation. This is insufficient for streaming, which is why `StreamingCodec` was introduced with `EncodeEvent` and `Flush`. This split works well — but the spec should clarify that `StreamingCodec.Encode` (inherited from `RepresentationCodec`) writes a single-event stream (one event then closes), while `EncodeEvent` writes one event in an ongoing stream. The semantic distinction should be documented.

### `Renderer.RespondStream` and handler responsibility

`Renderer.RespondStream` currently accepts a `<-chan Representation` and handles the full write loop, including flushing. This is a good design — handlers should not write SSE directly. However, the spec should clarify:
- Whether `RespondStream` should accept a `context.Context` parameter (currently it uses `req.Context()`, which is correct but implicit).
- Whether `RespondStream` should return an error channel or just a single error (current design returns after stream closes, which blocks the handler goroutine).

### `Client.FetchStream` completion vs error signaling

`Client.FetchStream` returns `<-chan *Response` and closes the channel on stream end or error. There is no way for the caller to distinguish "stream completed normally" from "stream terminated due to network error" after the channel closes. Consider:
- Adding an error channel: `FetchStream(...) (<-chan *Response, <-chan error, error)`
- Or wrapping the channel in a `Stream` type with a `Stream.Err() error` method callable after the channel closes.

### `Representation.Kind` for SSE event type mapping

The current design maps `Representation.Kind` directly to the SSE `event:` field. This works well for this use case (`stream-open`, `agent-chunk`, `agent-response`, `stream-error`). However, there is a coupling concern: `Kind` serves double duty as both the application-level semantic label and the SSE transport-level event type. If a representation's `Kind` needs to differ from its SSE event type (e.g., all chunks are `Kind: "message"` but the SSE event type should be `"agent-chunk"`), there is no way to express this. Consider:
- A `Meta["sse-event"]` convention that overrides `Kind` for the SSE `event:` field.
- Or a dedicated `EventType` field on `Representation` (heavier but explicit).

### `text/event-stream` and `RenderMode`

The current `RenderMode` enum has `RenderDocument` and `RenderFragment`. Streaming does not fit neatly into either mode. `Renderer.RespondStream` currently hardcodes `RenderDocument` mode for all events. Consider:
- Adding `RenderStream RenderMode = 2` to signal to codecs that they are encoding within a stream context. This would let codecs optimize (e.g., omit redundant fields in chunk events).
- Or documenting that `RenderDocument` is correct for stream events since each event is a self-contained representation.

### `http.Flusher` dependency

`Renderer.RespondStream` checks for `http.Flusher` at runtime and returns an error if the writer does not implement it. This is correct but fragile — the failure happens at request time, not at configuration time. Consider:
- Documenting this requirement prominently.
- Or adding a `Renderer.CanStream(w http.ResponseWriter) bool` helper so handlers can check before committing to the streaming path.

### `Embedded` for tool-call results

Using `Embedded` to carry tool-call results within agent chunks works but raises a question: should intermediate chunk events carry embedded representations, or only the final `agent-response`? Sending `Embedded` on every chunk that triggers a tool call adds payload size. The spec should clarify whether `Embedded` is intended for use in streaming contexts and whether there are conventions for when to include vs. omit it.

### SSE `retry` directive

The SSE spec defines a `retry:` field that tells the client how long to wait before reconnecting. There is currently no way to set this through `hyper` — `EventStreamCodec.EncodeEvent` does not emit a `retry:` field. Consider:
- Supporting `Meta["retry"]` or `Hints["retry"]` to set the SSE reconnection interval.
- Or adding a `retry` field to the `EncodeEvent` output when `Meta["retry"]` is present.

### `SubmitStream` on `Client`

`Client.FetchStream` sends a GET request with `Accept: text/event-stream`. But the AI agent use case submits a prompt via POST. There is no `Client.SubmitStream` that combines `Submit` (POST with a body) and `FetchStream` (SSE response decoding). The client currently has to drop down to raw HTTP to POST with `Accept: text/event-stream` and read the SSE response. Consider adding:

```go
func (c *Client) SubmitStream(ctx context.Context, action Action, values map[string]any) (<-chan *Response, error)
```

This would encode the submission body, send the request with `Accept: text/event-stream`, and return a channel of decoded events — the same pattern as `FetchStream` but for state-changing actions.
