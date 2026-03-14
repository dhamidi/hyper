# How to Integrate with htmx

This guide shows how to use hyper with htmx for server-rendered hypermedia.

## Using Action Hints for htmx attributes

Actions support a `Hints` map that can carry htmx attributes:

```go
import "github.com/dhamidi/hyper"

action := hyper.Action{
    Name:   "Search",
    Method: "GET",
    Target: hyper.Path("contacts"),
    Hints: map[string]any{
        "hx-get":     "/contacts",
        "hx-target":  "#results",
        "hx-trigger": "keyup changed delay:300ms",
    },
}
```

## RenderMode for partial responses

Use `RenderFragment` to return only the fragment (no full page layout):

```go
renderer.RespondWithMode(w, req, 200, rep, hyper.RenderFragment)
```

This allows htmx to swap in partial HTML without a full page reload.

## Dual-hinting pattern

Serve both htmx-powered HTML and CLI clients from the same representation
by including both generic and htmx-specific hints:

```go
Hints: map[string]any{
    "confirm":     true,       // generic: any client can interpret
    "destructive": true,       // generic
    "hx-confirm":  "Are you sure?", // htmx-specific
}
```

The HTML codec reads the `hx-*` keys; the CLI codec reads the generic keys.

## Method override for PUT, DELETE, and PATCH

HTML forms only support `GET` and `POST`. When an action uses `PUT`, `DELETE`,
or `PATCH`, the HTML codec automatically renders the form with `method="POST"`
and adds a hidden `<input type="hidden" name="_method" value="PUT">` field
carrying the original method.

To interpret this field on the server, use the `methodoverride` middleware:

```go
import "github.com/dhamidi/hyper/methodoverride"

handler = methodoverride.Wrap(handler)
```

This middleware reads the `_method` form field and overrides the request method
accordingly, so your handlers receive the correct HTTP method.
