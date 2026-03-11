# RFC: `hyper` - Hypermedia Representations and Affordances for Go

- Status: Draft
- Intended Audience: Library authors and application developers
- Package: `hyper`
- Language: Go

## 1. Abstract

This document specifies `hyper`, a Go library for building hypermedia-driven
applications with first-class support for HTML, Markdown, JSON, and server-side
user interfaces enhanced with `htmx`.

`hyper` defines:

- a media-type-neutral representation model
- links and actions as first-class hypermedia controls
- pluggable outbound target resolution
- pluggable response codecs and submission codecs
- explicit support for embedded representations

`hyper` does not define:

- a router
- a template engine
- a persistence model
- a client-side framework

## 2. Goals

### 2.1 Primary Goals

The library SHALL:

1. model transferable representations rather than domain resources
2. expose links and actions as explicit hypermedia controls
3. allow one representation to be rendered as HTML, Markdown, or JSON
4. support embedded and fragment-oriented representations for `htmx`
5. separate response encoding from request submission decoding
6. allow URL generation to be delegated to an external routing library
7. remain idiomatic to Go and friendly to `net/http`

### 2.2 Non-Goals

The library SHALL NOT:

1. mandate a specific router
2. require code generation
3. require reflection for normal operation
4. assume that all media types are symmetric for encode and decode
5. assume that all actions are representable equally well in all media types

## 3. Design Principles

### 3.1 Representation-Centric

The top-level transferable value SHALL be a representation.

A representation describes:

- application state
- links to related representations
- actions that describe available state transitions
- embedded sub-representations

### 3.2 Hypermedia Controls First

Links and actions SHALL be explicit values in the model.

The library SHALL NOT require callers to encode control metadata inside
unstructured maps or ad hoc HTML.

### 3.3 Media-Type Neutral Core

The core model SHALL NOT contain HTML-specific concepts such as `<form>` as
primary abstractions.

Media-type-specific constructs MAY be derived from the core model by codecs.

### 3.4 HTML-First Pragmatism

The library SHALL support HTML as a first-class target format.

The library SHOULD support patterns common in `htmx` applications, including:

- full-document responses
- fragment responses
- embedded controls
- server-driven redirects or UI updates

### 3.5 External URL Resolution

The core model SHALL describe targets abstractly.

Target resolution SHALL occur through an interface so that routing libraries
such as `github.com/dhamidi/dispatch` can be used without modification.

## 4. Terminology

### 4.1 Representation

A transferable hypermedia value describing current application state and the
available transitions from that state.

### 4.2 Link

A hypermedia control for navigation or retrieval.

### 4.3 Action

A hypermedia control describing a transition, optionally parameterized by
fields.

### 4.4 Field

Input metadata for an action.

### 4.5 Embedded Representation

A representation nested inside another representation.

### 4.6 Codec

A media-type-specific component used to encode representations or decode
submissions.

## 5. Package Overview

The package namespace SHOULD be `hyper`.

The package SHOULD expose these core concepts:

- `Representation`
- `Node`
- `Value`
- `Link`
- `Action`
- `Field`
- `Target`
- `Resolver`
- `RepresentationCodec`
- `SubmissionCodec`
- `Renderer`

## 6. Core Model

### 6.1 Representation

The primary value SHALL be:

```go
type Representation struct {
    Kind     string
    Self     *Target
    State    Node
    Links    []Link
    Actions  []Action
    Embedded map[string][]Representation
    Meta     map[string]any
}
```

### 6.1.1 Semantics

- `Kind` SHOULD be an application-defined semantic label
- `Self` MAY identify the representation's canonical target
- `State` SHOULD contain the primary application state
- `Links` SHALL represent navigational controls
- `Actions` SHALL represent available transitions
- `Embedded` MAY contain named related or fragment representations
- `Meta` MAY contain application-specific metadata

### 6.1.2 Requirements

- Implementations SHOULD treat a `Representation` as immutable during encoding
- Codecs MUST NOT mutate the caller's representation
- Embedded representations MAY themselves contain links, actions, and embedded
  representations

### 6.2 Node

Structured representation state SHALL be modeled as a node.

```go
type Node interface {
    isNode()
}
```

Recommended node forms:

```go
type Object map[string]Value
type Collection []Value

func (Object) isNode() {}
func (Collection) isNode() {}
```

### 6.3 Value

Leaf values SHALL be modeled independently from structured nodes.

```go
type Value interface {
    isValue()
}
```

Recommended value forms:

```go
type Scalar struct{ V any }

type RichText struct {
    MediaType string
    Source    string
}

func (Scalar) isValue() {}
func (RichText) isValue() {}
```

### 6.3.1 RichText

`RichText` is intended for content whose semantic source may be rendered
differently by different codecs.

Examples include:

- `text/markdown`
- `text/plain`
- `text/html`

Codecs MAY:

- render `text/markdown` as HTML
- preserve `text/markdown` as Markdown
- surface rich text as typed JSON

## 7. Hypermedia Controls

### 7.1 Link

```go
type Link struct {
    Rel    string
    Target Target
    Title  string
    Type   string
}
```

#### Requirements

- `Rel` SHALL identify the control semantics
- `Target` SHALL identify the destination
- `Type` MAY indicate the expected response media type

### 7.2 Action

```go
type Action struct {
    Name     string
    Rel      string
    Method   string
    Target   Target
    Consumes []string
    Produces []string
    Fields   []Field
    Hints    map[string]any
}
```

#### Semantics

- `Name` SHOULD be a stable application-defined identifier
- `Rel` SHOULD describe the semantic relationship of the action
- `Method` SHALL be an HTTP method name
- `Consumes` SHOULD list accepted submission media types
- `Produces` MAY list likely response media types
- `Fields` MAY describe input required by the action
- `Hints` MAY contain codec-specific or UI-specific metadata

#### Design Note

An action is the core semantic primitive.

An HTML codec MAY render an action with fields as a `<form>`, but the core
model SHALL NOT require a separate `Form` type.

### 7.3 Field

```go
type Field struct {
    Name     string
    Type     string
    Value    any
    Required bool
    ReadOnly bool
    Label    string
    Help     string
    Options  []Option
    Error    string
}

type Option struct {
    Value    string
    Label    string
    Selected bool
}
```

#### Requirements

- `Name` SHALL identify the submitted field name
- `Type` SHOULD identify the intended input control type
- `Error` MAY contain a validation message from a failed submission
- `Options` MAY describe enumerated choices

#### Semantics

`Field` describes the input contract of an action. It does not prescribe a
specific rendering strategy.

## 8. Targets and URL Resolution

### 8.1 Target

```go
type Target struct {
    Href  string
    Route *RouteRef
}

type RouteRef struct {
    Name   string
    Params map[string]string
}
```

### 8.1.1 Requirements

- Exactly one of `Href` or `Route` SHOULD be set
- `Href` SHALL represent a directly specified target
- `Route` SHALL represent an abstract, named route target

### 8.2 Resolver

```go
type Resolver interface {
    ResolveTarget(context.Context, Target) (string, error)
}
```

### 8.2.1 Semantics

A resolver SHALL:

1. return `Href` directly when present
2. resolve `Route` when present
3. fail when neither form is present

### 8.2.2 Dispatch Compatibility

The resolver seam SHALL be narrow enough to support
`github.com/dhamidi/dispatch` without modification.

Example adapter:

```go
type DispatchResolver struct {
    Router interface {
        Path(string, dispatch.Params) (string, error)
    }
}

func (r DispatchResolver) ResolveTarget(_ context.Context, t hyper.Target) (string, error) {
    if t.Href != "" {
        return t.Href, nil
    }
    if t.Route == nil {
        return "", errors.New("hyper: missing target")
    }
    return r.Router.Path(t.Route.Name, dispatch.Params(t.Route.Params))
}
```

## 9. Media Type Components

### 9.1 RepresentationCodec

```go
type RepresentationCodec interface {
    MediaTypes() []string
    Encode(context.Context, io.Writer, Representation, EncodeOptions) error
}
```

### 9.2 SubmissionCodec

```go
type SubmissionCodec interface {
    MediaTypes() []string
    Decode(context.Context, io.Reader, any, DecodeOptions) error
}
```

### 9.3 Rationale

Implementations SHALL separate response encoding from request decoding because
HTML applications commonly:

- respond with `text/html`
- submit with `application/x-www-form-urlencoded`
- upload with `multipart/form-data`

A single symmetric codec abstraction SHALL NOT be required.

### 9.4 Suggested Shared Options

Implementations SHOULD provide shared option structs so codecs can receive
request and rendering context without depending on global state.

Suggested shape:

```go
type EncodeOptions struct {
    Request  *http.Request
    Resolver Resolver
    Mode     RenderMode
}

type DecodeOptions struct {
    Request *http.Request
}

type RenderMode uint8

const (
    RenderDocument RenderMode = iota
    RenderFragment
)
```

## 10. Renderer

### 10.1 Renderer API

The library SHOULD expose a renderer that can negotiate or force response
formats.

Suggested shape:

```go
type Renderer struct {
    Codecs   []RepresentationCodec
    Resolver Resolver
}

func (r Renderer) Respond(http.ResponseWriter, *http.Request, int, Representation) error
func (r Renderer) RespondAs(http.ResponseWriter, *http.Request, int, string, Representation) error
```

### 10.2 Semantics

`Respond` SHALL:

1. inspect the request `Accept` header
2. choose the best available codec
3. encode the representation
4. write the chosen response media type

`RespondAs` SHALL:

1. bypass normal content negotiation
2. select a codec matching the requested media type
3. encode the representation using that codec

The renderer SHOULD set the `Content-Type` response header accordingly.

## 11. HTML Codec

### 11.1 HTML Role

HTML SHALL be treated as a first-class target format.

### 11.2 Action Rendering

An HTML codec SHOULD:

- render `Link` as `<a>`
- render a fieldless `GET` action as a link or button
- render an action with fields as a `<form>`
- render non-`GET` actions using a form submission mechanism appropriate for
  the host application

### 11.3 Embedded Representations

An HTML codec SHOULD support:

- full-document rendering
- fragment rendering
- nested rendering of embedded representations

### 11.4 HTMX Hints

HTML-specific hint keys MAY be carried in `Action.Hints`.

Examples include:

- `hx-target`
- `hx-swap`
- `hx-push-url`
- `hx-select`

The core model SHALL NOT require these keys, but an HTML codec MAY interpret
them.

## 12. Markdown Codec

### 12.1 Markdown Role

Markdown SHOULD be treated as a read-oriented alternate representation.

### 12.2 Limitations

A Markdown codec MAY degrade actions, because Markdown does not natively model
interactive form controls.

A Markdown codec MAY:

- render links normally
- render actions as prose, lists, or reference blocks
- omit UI-specific hints

## 13. JSON Codec

### 13.1 JSON Role

JSON support SHOULD preserve hypermedia semantics rather than reducing a
representation to plain state only.

### 13.2 Minimum Behavior

A JSON codec SHOULD preserve:

- state
- links
- actions
- embedded representations

## 14. Examples

### 14.1 Representation with an Update Action

```go
rep := hyper.Representation{
    Kind: "contact",
    Self: &hyper.Target{
        Route: &hyper.RouteRef{
            Name: "contacts.show",
            Params: map[string]string{"id": "42"},
        },
    },
    State: hyper.Object{
        "id":    hyper.Scalar{V: 42},
        "name":  hyper.Scalar{V: "Ada"},
        "email": hyper.Scalar{V: "ada@example.com"},
        "bio": hyper.RichText{
            MediaType: "text/markdown",
            Source:    "Ada Lovelace wrote the first algorithm.",
        },
    },
    Actions: []hyper.Action{
        {
            Name:   "Save",
            Rel:    "update",
            Method: "PUT",
            Target: hyper.Target{
                Route: &hyper.RouteRef{
                    Name: "contacts.update",
                    Params: map[string]string{"id": "42"},
                },
            },
            Consumes: []string{"application/x-www-form-urlencoded"},
            Produces: []string{"text/html", "text/markdown"},
            Fields: []hyper.Field{
                {Name: "name", Type: "text", Value: "Ada", Required: true},
                {Name: "email", Type: "email", Value: "ada@example.com", Required: true},
            },
        },
    },
}
```

### 14.2 Embedded Inline Field Editor

```go
rep := hyper.Representation{
    Kind: "contact",
    State: hyper.Object{
        "name":  hyper.Scalar{V: "Ada"},
        "email": hyper.Scalar{V: "ada@example.com"},
    },
    Embedded: map[string][]hyper.Representation{
        "email_editor": {
            {
                Kind: "contact-email-editor",
                Actions: []hyper.Action{
                    {
                        Name:   "Save Email",
                        Rel:    "update-email",
                        Method: "PUT",
                        Target: hyper.Target{
                            Route: &hyper.RouteRef{
                                Name: "contacts.email.update",
                                Params: map[string]string{"id": "42"},
                            },
                        },
                        Consumes: []string{"application/x-www-form-urlencoded"},
                        Fields: []hyper.Field{
                            {
                                Name:     "email",
                                Type:     "email",
                                Value:    "ada@example.com",
                                Required: true,
                                Error:    "Email is invalid.",
                            },
                        },
                        Hints: map[string]any{
                            "hx-target": "#contact-email",
                            "hx-swap":   "outerHTML",
                        },
                    },
                },
            },
        },
    },
}
```

### 14.3 Handler with Negotiated HTML or Markdown and Form Decoding

```go
func (a *App) NewContact(w http.ResponseWriter, r *http.Request) {
    switch r.Method {
    case http.MethodGet:
        rep := a.newContactRepresentation(ContactInput{}, nil)
        _ = a.Renderer.Respond(w, r, http.StatusOK, rep)
        return

    case http.MethodPost:
        var in ContactInput
        if err := a.FormCodec.Decode(r.Context(), r.Body, &in, hyper.DecodeOptions{
            Request: r,
        }); err != nil {
            http.Error(w, "bad request", http.StatusBadRequest)
            return
        }

        errs := validate(in)
        if len(errs) > 0 {
            rep := a.newContactRepresentation(in, errs)
            _ = a.Renderer.Respond(w, r, http.StatusUnprocessableEntity, rep)
            return
        }

        contact, err := a.Store.Create(r.Context(), in)
        if err != nil {
            http.Error(w, "server error", http.StatusInternalServerError)
            return
        }

        location, err := a.Resolver.ResolveTarget(r.Context(), hyper.Target{
            Route: &hyper.RouteRef{
                Name: "contacts.show",
                Params: map[string]string{"id": strconv.FormatInt(contact.ID, 10)},
            },
        })
        if err != nil {
            http.Error(w, "server error", http.StatusInternalServerError)
            return
        }

        http.Redirect(w, r, location, http.StatusSeeOther)
        return
    }
}
```

### 14.4 Explicit Response Format

```go
func (a *App) ContactPreview(w http.ResponseWriter, r *http.Request) {
    rep := a.contactPreviewRepresentation(...)

    if r.URL.Query().Get("format") == "md" {
        _ = a.Renderer.RespondAs(w, r, http.StatusOK, "text/markdown", rep)
        return
    }

    _ = a.Renderer.Respond(w, r, http.StatusOK, rep)
}
```

## 15. Compliance Requirements

A conforming implementation of `hyper` MUST:

1. expose a representation-centric core model
2. model links and actions explicitly
3. support embedded representations
4. support pluggable target resolution
5. support pluggable response codecs
6. support pluggable submission codecs

A conforming implementation SHOULD:

1. provide an HTML codec
2. provide a Markdown codec
3. provide a JSON hypermedia codec
4. provide renderer helpers for negotiated and explicit response formats
5. support field-level validation feedback in action fields

## 16. Open Questions

The following questions remain open for later revisions:

1. whether `Hints` should remain a plain map or become typed codec extensions
2. whether `Object` and `Collection` should remain minimal or grow helper APIs
3. whether JSON support should target a specific hypermedia media type first
4. how much fragment-targeting behavior should be standardized for `htmx`
