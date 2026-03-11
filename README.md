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
