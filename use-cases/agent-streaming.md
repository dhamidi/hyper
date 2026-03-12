# Use Case: AI Agent Token Streaming via SSE

This document demonstrates how hyper's SSE streaming support enables an AI agent to stream token-by-token responses to a client using `text/event-stream`.

## Scenario

An AI chat API exposes a `/conversations/:id/messages` endpoint. When a user submits a message, the server streams the assistant's response token-by-token as Server-Sent Events. Each event is a full hyper `Representation`, preserving hypermedia controls throughout the stream.

## Server Handler

```go
func handleCreateMessage(w http.ResponseWriter, r *http.Request) {
    renderer := hyper.Renderer{
        Codecs: []hyper.RepresentationCodec{
            hyper.EventStreamCodec{},
            hyper.JSONCodec(),
        },
    }

    // Check if the client wants a stream.
    accept := r.Header.Get("Accept")
    if accept == "text/event-stream" {
        streamResponse(w, r, renderer)
        return
    }

    // Fallback: synchronous response via polling.
    job := startGeneration(r)
    renderer.Respond(w, r, http.StatusAccepted, hyper.Representation{
        Kind: "generation-job",
        State: hyper.Object{
            "status": hyper.Scalar{V: "pending"},
        },
        Meta: map[string]any{"poll-interval": 2},
    })
}

func streamResponse(w http.ResponseWriter, r *http.Request, renderer hyper.Renderer) {
    reps := make(chan hyper.Representation)

    go func() {
        defer close(reps)

        // Stream-open event with available actions.
        reps <- hyper.Representation{
            Kind: "stream-open",
            State: hyper.Object{
                "status": hyper.Scalar{V: "generating"},
            },
            Actions: []hyper.Action{
                hyper.NewAction("Cancel", "POST", hyper.Path("conversations", "123", "cancel")),
            },
            Meta: map[string]any{"eventID": "0"},
        }

        // Simulate token generation.
        tokens := []string{"Hello", " there", "!", " How", " can", " I", " help", "?"}
        for i, token := range tokens {
            reps <- hyper.Representation{
                Kind: "token",
                State: hyper.Object{
                    "text":  hyper.Scalar{V: token},
                    "index": hyper.Scalar{V: i},
                },
                Meta: map[string]any{"eventID": fmt.Sprintf("%d", i+1)},
            }
        }

        // Stream-close event with final links.
        reps <- hyper.Representation{
            Kind: "stream-close",
            State: hyper.Object{
                "status":      hyper.Scalar{V: "complete"},
                "totalTokens": hyper.Scalar{V: len(tokens)},
            },
            Links: []hyper.Link{
                hyper.NewLink("self", hyper.Path("conversations", "123", "messages", "456")),
                hyper.NewLink("conversation", hyper.Path("conversations", "123")),
            },
            Meta: map[string]any{"eventID": "final"},
        }
    }()

    renderer.RespondStream(w, r, reps)
}
```

## Client Usage

### Go Client with FetchStream

```go
client, _ := hyper.NewClient("http://localhost:8080",
    hyper.WithAccept("text/event-stream"),
)

target := hyper.Path("conversations", "123", "messages")
ch, err := client.FetchStream(ctx, target)
if err != nil {
    log.Fatal(err)
}

var fullText strings.Builder
for resp := range ch {
    rep := resp.Representation
    switch rep.Kind {
    case "stream-open":
        fmt.Println("Generation started...")
    case "token":
        if state, ok := rep.State.(hyper.Object); ok {
            if text, ok := state["text"].(hyper.Scalar); ok {
                fmt.Print(text.V)
                fullText.WriteString(fmt.Sprint(text.V))
            }
        }
    case "stream-close":
        fmt.Println("\nGeneration complete.")
        // Follow links from the final event.
        for _, link := range rep.Links {
            fmt.Printf("  %s: %s\n", link.Rel, link.Target.URL)
        }
    }
}
```

### Browser Client with EventSource

Because hyper SSE uses standard `text/event-stream`, the browser's native `EventSource` API works out of the box:

```javascript
const source = new EventSource('/conversations/123/messages');

source.addEventListener('stream-open', (e) => {
    const rep = JSON.parse(e.data);
    console.log('Stream started:', rep.state.status);
});

source.addEventListener('token', (e) => {
    const rep = JSON.parse(e.data);
    document.getElementById('output').textContent += rep.state.text;
});

source.addEventListener('stream-close', (e) => {
    const rep = JSON.parse(e.data);
    console.log('Complete. Tokens:', rep.state.totalTokens);

    // Follow hypermedia links from the final event.
    for (const link of rep.links || []) {
        console.log(`${link.rel}: ${link.href}`);
    }
    source.close();
});
```

## Progressive Enhancement

The same action can support both polling and streaming:

```go
action := hyper.Action{
    Name:   "Generate",
    Rel:    "create",
    Method: "POST",
    Target: hyper.Path("conversations", "123", "messages"),
    Hints:  map[string]any{
        "async":  true,
        "stream": true,
    },
}
```

- Clients that understand SSE send `Accept: text/event-stream` and get real-time token streaming.
- Clients that do not understand SSE omit the Accept header (or send `application/json`) and get a job representation to poll.
- Both paths return full hyper `Representation` values with links, actions, and state — the hypermedia contract is preserved regardless of delivery mechanism.

## Key Properties

1. **Each event is a full Representation**: Tokens carry state, and lifecycle events carry links and actions. No out-of-band metadata.
2. **Standard SSE**: Works with browser `EventSource`, `curl`, and any SSE client library.
3. **Reconnection**: The `id:` field on each event enables `Last-Event-ID`-based reconnection per the SSE spec.
4. **Codec pluggability**: `EventStreamCodec` is registered like any other codec — no special-casing in the Renderer.
