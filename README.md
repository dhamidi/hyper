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
