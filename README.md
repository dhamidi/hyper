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

## htmlc — Server-Side Vue Component Engine

The `htmlc/` directory contains a companion Go module (`github.com/dhamidi/htmlc`)
that provides a server-side template engine using Vue.js Single File Component
(`.vue`) syntax. It is designed to work with hyper as a custom `RepresentationCodec`.

### Quick Start

```go
engine, err := htmlc.New(htmlc.Options{
    ComponentDir: "components/",
})

// Render as a full HTML document
engine.RenderPage(w, "dashboard", scope)

// Render as a bare fragment (e.g., for htmx partials)
engine.RenderFragment(w, "task-list", scope)
```

### Supported Template Features

- **Text interpolation**: `{{ expression }}` with dot-path lookups
- **`v-for`**: `<template v-for="item in list">` for iteration
- **`v-if` / `v-else`**: conditional rendering
- **`:attr` binding**: `:href="expr"`, `:class="'prefix ' + value"`
- **`v-bind` spread**: `v-bind="mapExpr"` to spread a map as HTML attributes
- **`v-html`**: raw HTML insertion (trusted content)
- **Child components**: `<task-row v-bind="item">` renders another `.vue` component
- **Boolean attributes**: `:selected`, `:required`, `:checked` render or omit based on truthiness

### Component Not Found

When a component is not found, the error wraps `htmlc.ErrComponentNotFound`.
Use `htmlc.IsComponentNotFound(err)` to detect this for fallback logic:

```go
err := engine.RenderFragment(w, name, scope)
if err != nil && htmlc.IsComponentNotFound(err) {
    // Fall back to generic HTML codec
}
```

### Importing from Examples

Example apps import htmlc via a `replace` directive:

```
require github.com/dhamidi/htmlc v0.0.0
replace github.com/dhamidi/htmlc => ../../htmlc
```

See [`use-cases/htmlc-codec.md`](./use-cases/htmlc-codec.md) for the full
integration pattern with hyper's `RepresentationCodec`.

## Examples

### Task List

A task list web app demonstrating a custom `RepresentationCodec` backed by
`htmlc` for HTML rendering, with htmx integration and content negotiation.
Both HTML and JSON responses are driven through `hyper.Renderer`.

```bash
cd examples/tasklist
go run .
# Open http://localhost:8080
```

See [`examples/tasklist/README.md`](./examples/tasklist/README.md) for details.
