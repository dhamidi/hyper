# hyper

Experimental hypermedia library design for Go.

This repository currently contains:

- [`spec.md`](./spec.md): the draft normative specification
- [`go.mod`](./go.mod): the initial module declaration for `github.com/dhamidi/hyper`

The current design centers on:

- `Representation` as the transferable value
- `Link` and `Action` as first-class hypermedia controls
- pluggable target resolution so routers such as `github.com/dhamidi/dispatch`
  can be used without modification
- separate response and submission codecs for HTML, Markdown, JSON, and form
  workflows
- automatic method override in HTML forms: actions using PUT, DELETE, or PATCH
  are rendered as POST forms with a hidden `_method` field, paired with the
  `methodoverride` middleware for server-side interpretation
- hint-driven HTML attributes: `Action.Hints` and `Representation.Hints` with
  string values are emitted as HTML attributes on the rendered element, enabling
  htmx integration (e.g., `hx-target`, `hx-swap`). The `"destructive"` bool
  hint adds `class="destructive"` and `"hidden"` (bool, true) suppresses
  rendering entirely
- file upload support in HTML forms: when an action contains a field with
  `Type: "file"`, the HTML codec sets `enctype="multipart/form-data"` on the
  form element. The `Accept` field constraint renders as an `accept` attribute,
  `Multiple` renders the `multiple` boolean attribute, and `MaxSize` is emitted
  as a `data-max-size` attribute for client-side validation
- streaming support: `Client.FetchStream` sends a GET with
  `Accept: text/event-stream` and returns a channel of responses decoded from
  SSE events. `Client.SubmitStream` does the same for action submissions (POST
  with a request body), enabling streaming interactions such as AI agent
  token streams. Both methods fall back to a single-response channel when the
  server responds with a non-SSE content type

## Examples

### Task List

A task list web app demonstrating the built-in HTML codec with a
typewriter-inspired design, htmx integration, and content negotiation.

```bash
cd examples/tasklist
go run .
# Open http://localhost:8080
```

See [`examples/tasklist/README.md`](./examples/tasklist/README.md) for details.
