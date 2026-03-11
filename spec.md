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

Target resolution SHALL occur through an interface so that any routing library
can be used without modification.

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

#### Embedded Representations for Item Lists

When state contains a list of independently addressable items (e.g. contacts,
orders, articles), implementations SHOULD prefer `Embedded` over
`State: Collection{...}`. Embedded representations carry their own `Self`,
`Kind`, `Links`, and `Actions`, which enables machine clients to navigate to,
inspect, and act on individual items without out-of-band knowledge.

`Collection` remains appropriate for flat value lists where items are not
independently addressable — for example, tags, categories, or enumerated
labels.

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

#### Recommended Link Rel: `"actions"`

A `Link` with `rel: "actions"` MAY point to a resource that enumerates
available actions for the current resource or collection.

When the available actions are known at response time, servers SHOULD embed
them directly in the `Representation`'s `Actions` array rather than
requiring a separate fetch. The `"actions"` link is primarily useful when:

- The set of available actions is large and would bloat the primary
  representation.
- Actions are computed lazily or depend on additional authorization checks.
- The server wants to offer a stable, cacheable "action catalog" separate
  from the resource state.

When a client follows an `"actions"` link, the response SHOULD be a
`Representation` whose `Actions` array contains the available actions for
the originating resource. Clients MAY merge these actions with any actions
already present on the originating representation.

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

#### Action.Rel Vocabulary

`Action.Rel` is an open string vocabulary — any string value is acceptable.

The spec RECOMMENDS the following well-known rels for CRUD operations:

| Rel        | Description                                      |
|------------|--------------------------------------------------|
| `create`   | Create a new resource                            |
| `update`   | Update an existing resource                      |
| `delete`   | Delete an existing resource                      |
| `search`   | Search or query a collection                     |

These well-known rels carry standard semantics that generic clients MAY
optimize for — for example, a CLI client might map `create` to a `POST`
workflow with field prompts, or a web UI might render `delete` with a
destructive button style.

All other `Action.Rel` values are domain-specific. Clients SHOULD treat
unknown rels as opaque identifiers and surface them by name — for example,
as subcommands in a CLI, or as buttons in a UI. No namespacing convention
(e.g., `x-archive`) is required. The `Action.Name` field provides a
human-readable label, and `Action.Hints` provides sufficient context for
clients to render domain-specific actions appropriately.

The client algorithm for rendering actions is simple: render all `Actions`
in the `Representation` as available commands or buttons. Well-known rels
(`create`, `update`, `delete`, `search`) MAY receive special treatment
(e.g., mapping to HTTP verbs, special icons, or specific UI patterns), but
all rels are valid and renderable.

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

### 7.3.1 Recommended Field Type Vocabulary

The following base vocabulary for `Field.Type` is RECOMMENDED. These values
align with HTML input types, providing a shared vocabulary that both HTML
codecs and non-HTML clients (CLI tools, mobile apps) can interpret:

| Type       | Description                                  |
|------------|----------------------------------------------|
| `text`     | Single-line text input                       |
| `email`    | Email address input                          |
| `tel`      | Telephone number input                       |
| `number`   | Numeric input                                |
| `date`     | Date input                                   |
| `url`      | URL input                                    |
| `password` | Masked text input                            |
| `hidden`   | Hidden input (not displayed to the user)     |
| `textarea` | Multi-line text input                        |
| `select`   | Selection from `Options`                     |
| `checkbox` | Boolean toggle                               |

Codecs MAY support additional type values beyond this list. Unknown types
SHOULD be treated as `text` by codecs that do not recognize them.

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

### 11.4 Codec-Specific Hints

`Action.Hints` MAY carry codec-specific or framework-specific hint keys.

The core model does not define or require any specific keys. Codecs MAY
interpret hint keys that are relevant to their output format. See the
Interaction Points section for concrete examples of how different front-end
frameworks use `Hints`.

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

### 13.3 JSON Wire Format

A JSON codec SHOULD encode a `Representation` as a JSON object with the
following top-level keys:

```json
{
  "kind": "contact",
  "self": { "href": "/contacts/42" },
  "state": { ... },
  "links": [ ... ],
  "actions": [ ... ],
  "embedded": { ... },
  "meta": { ... }
}
```

Keys that are empty or absent in the source representation MAY be omitted
from the JSON output.

#### 13.3.1 State Encoding

State values SHALL be encoded according to their type:

- `Scalar` — encoded as the underlying JSON value (string, number, boolean,
  or null).
- `RichText` — encoded as an object with a type discriminator:
  `{"_type": "richtext", "mediaType": "text/markdown", "source": "..."}`.
- `Object` — encoded as a JSON object whose keys map to encoded values.
- `Collection` — encoded as a JSON array of encoded values.

#### 13.3.2 Link Encoding

Each `Link` SHALL be encoded as a JSON object:

```json
{
  "rel": "author",
  "href": "/users/7",
  "title": "Author Profile",
  "type": "text/html"
}
```

The `href` key SHALL contain the resolved target URL. The `title` and `type`
keys MAY be omitted when empty.

#### 13.3.3 Action Encoding

Each `Action` SHALL be encoded as a JSON object:

```json
{
  "name": "Save",
  "rel": "update",
  "method": "PUT",
  "href": "/contacts/42",
  "consumes": ["application/json"],
  "produces": ["application/json"],
  "fields": [ ... ],
  "hints": { ... }
}
```

The `href` key SHALL contain the resolved target URL. Keys with empty or
zero values MAY be omitted.

#### 13.3.4 Field Encoding

Each `Field` SHALL be encoded as a JSON object:

```json
{
  "name": "email",
  "type": "email",
  "value": "ada@example.com",
  "required": true,
  "readOnly": false,
  "label": "Email Address",
  "help": "Your primary email",
  "options": [{"value": "a", "label": "A", "selected": false}],
  "error": ""
}
```

Boolean fields that are `false` and string fields that are empty MAY be
omitted.

#### 13.3.5 Embedded Representation Encoding

The `embedded` key SHALL be a JSON object whose keys are slot names and
whose values are arrays of encoded representations (following the same
top-level structure recursively).

#### 13.3.6 Target Encoding

In JSON output, targets SHALL be represented as resolved URL strings under
the `href` key. The `self` field SHALL be encoded as
`{"href": "<resolved-url>"}` or omitted when absent.

#### 13.3.7 Divergence from Existing Formats

The `hyper` JSON wire format is intentionally self-contained and does not
conform to HAL, Siren, or JSON:API. Key differences:

- HAL uses `_links` and `_embedded`; `hyper` uses `links` and `embedded`.
- Siren separates `properties` from `entities`; `hyper` uses `state` for all
  application data and `embedded` for sub-representations.
- JSON:API mandates `data`, `attributes`, and `relationships`; `hyper` uses a
  flat structure with `kind` and `state`.

Implementations that need interoperability with these formats SHOULD provide
separate codec implementations.

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

### 14.5 Root Representation (API Entry Point)

A discovery-driven client needs a well-known entry point from which to
begin navigating the API. A "root" representation serves this purpose: it
carries top-level links and actions that bootstrap navigation, and it may
have no meaningful `State`.

```go
rep := hyper.Representation{
    Kind: "root",
    Self: &hyper.Target{Href: "/"},
    Links: []hyper.Link{
        {Rel: "contacts", Target: hyper.Target{Href: "/contacts"}, Title: "Contacts"},
        {Rel: "settings", Target: hyper.Target{Href: "/settings"}, Title: "Settings"},
    },
    Actions: []hyper.Action{
        {
            Name:   "Search",
            Rel:    "search",
            Method: "GET",
            Target: hyper.Target{Href: "/search"},
            Fields: []hyper.Field{
                {Name: "q", Type: "text", Label: "Query"},
            },
        },
    },
}
```

The root representation is not a special type — it is a normal
`Representation` whose purpose is to expose the top-level navigation graph.
Machine clients (CLI tools, mobile apps, third-party integrations) SHOULD
use the root representation as their starting point and follow links
rather than hard-coding endpoint URLs.

## 15. Interaction Points

This section shows how `hyper` integrates with the broader Go ecosystem
through short, focused examples. Each subsection poses a concrete question
and answers it with minimal code.

### 15.1 Routing with `github.com/dhamidi/dispatch`

**How do I resolve `hyper.Target` route references using the `dispatch` router?**

Implement a `Resolver` adapter that delegates named-route resolution to
`dispatch.Router.Path`:

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

### 15.2 Routing with `net/http`'s `http.ServeMux`

**How do I use `hyper.Target` with the standard library's `http.ServeMux`,
which does not support reverse routing?**

Because `ServeMux` has no named routes, use direct `Href` targets instead
of `RouteRef`:

```go
// HrefResolver resolves targets that carry a direct Href.
// It returns an error for RouteRef targets because ServeMux
// does not support reverse routing.
type HrefResolver struct{}

func (HrefResolver) ResolveTarget(_ context.Context, t hyper.Target) (string, error) {
    if t.Href != "" {
        return t.Href, nil
    }
    return "", errors.New("hyper: ServeMux does not support named routes")
}
```

Build representations with `Href`-based targets:

```go
func contactHandler(w http.ResponseWriter, r *http.Request) {
    rep := hyper.Representation{
        Kind: "contact",
        Self: &hyper.Target{Href: "/contacts/42"},
        State: hyper.Object{
            "name": hyper.Scalar{V: "Ada"},
        },
        Actions: []hyper.Action{
            {
                Name:   "Save",
                Rel:    "update",
                Method: "PUT",
                Target: hyper.Target{Href: "/contacts/42"},
                Fields: []hyper.Field{
                    {Name: "name", Type: "text", Value: "Ada", Required: true},
                },
            },
        },
    }
    _ = renderer.Respond(w, r, http.StatusOK, rep)
}
```

For named-route support with the standard library, a small route registry
helper can map route names to URL patterns at startup.

### 15.3 HTMX with `html/template`

**How do I render a `hyper.Action` as an htmx-enhanced form using Go's
`html/template`?**

Populate the `Hints` map with htmx attributes when building the action:

```go
action := hyper.Action{
    Name:   "Save Email",
    Rel:    "update-email",
    Method: "PUT",
    Target: hyper.Target{Href: "/contacts/42/email"},
    Fields: []hyper.Field{
        {Name: "email", Type: "email", Value: "ada@example.com", Required: true},
    },
    Hints: map[string]any{
        "hx-target": "#contact-email",
        "hx-swap":   "outerHTML",
    },
}
```

Render the hints in a template:

```html
<form method="{{.Method}}" action="{{.ResolvedTarget}}"
  {{- range $k, $v := .Hints }}
    {{$k}}="{{$v}}"
  {{- end }}>
  {{range .Fields}}
    <label>{{.Label}}
      <input type="{{.Type}}" name="{{.Name}}" value="{{.Value}}">
    </label>
  {{end}}
  <button type="submit">{{.Name}}</button>
</form>
```

`Hints` is an open map — the core model imposes no fixed key set. Any
framework-specific attributes (e.g. `hx-target`, `hx-swap`, `hx-push-url`,
`hx-select`) are carried as plain key-value pairs.

### 15.4 Hotwire (Turbo + Stimulus)

**How do I integrate `hyper` representations with Hotwire's Turbo Frames
and Stimulus controllers?**

Use `Meta` or `Hints` to carry Turbo Frame IDs and Stimulus annotations:

```go
rep := hyper.Representation{
    Kind: "contact",
    Self: &hyper.Target{Href: "/contacts/42"},
    State: hyper.Object{
        "name": hyper.Scalar{V: "Ada"},
    },
    Meta: map[string]any{
        "turbo-frame": "contact_42",
    },
    Actions: []hyper.Action{
        {
            Name:   "Save",
            Method: "PUT",
            Target: hyper.Target{Href: "/contacts/42"},
            Hints: map[string]any{
                "data-controller": "form",
                "data-action":     "submit->form#save",
            },
        },
    },
}
```

Render in a template with Turbo Frame wrapping and Stimulus attributes:

```html
<turbo-frame id="{{.Meta.turbo-frame}}">
  <h2>{{.State.name}}</h2>
  {{range .Actions}}
  <form method="{{.Method}}" action="{{.ResolvedTarget}}"
    {{- range $k, $v := .Hints }}
      {{$k}}="{{$v}}"
    {{- end }}>
    {{range .Fields}}
      <input type="{{.Type}}" name="{{.Name}}" value="{{.Value}}">
    {{end}}
    <button type="submit">{{.Name}}</button>
  </form>
  {{end}}
</turbo-frame>
```

The pattern is the same as for htmx: framework-specific attributes flow
through the open `Hints` and `Meta` maps without any core model changes.

### 15.5 Component Abstraction with `github.com/dhamidi/htmlc`

**How do I render a `hyper.Representation` as an `htmlc` component tree?**

`htmlc` is a server-side Go template engine that uses Vue.js Single File
Component (`.vue`) syntax.  Author a `.vue` template for each
representation kind, convert `Representation` state into a
`map[string]any` scope, and render with the `htmlc.Engine`:

```vue
<!-- components/contact.vue -->
<template>
  <div class="contact">
    <h2>{{ name }}</h2>
    <p>{{ email }}</p>
    <slot name="email_editor"></slot>
  </div>
</template>
```

Convert a `hyper.Representation` into an `htmlc`-compatible scope:

```go
func representationToScope(r hyper.Representation) map[string]any {
    scope := map[string]any{"kind": r.Kind}
    if obj, ok := r.State.(hyper.Object); ok {
        for k, v := range obj {
            if s, ok := v.(hyper.Scalar); ok {
                scope[k] = s.V
            }
        }
    }
    // Embedded representations become nested scopes for slots.
    for slot, reps := range r.Embedded {
        var items []map[string]any
        for _, embedded := range reps {
            items = append(items, representationToScope(embedded))
        }
        scope[slot] = items
    }
    return scope
}
```

Use the engine in a handler:

```go
func contactHandler(eng *htmlc.Engine, w http.ResponseWriter, r *http.Request) {
    rep := buildContactRepresentation()
    scope := representationToScope(rep)
    _ = eng.RenderPage(w, "contact", scope)
}
```

### 15.6 CLI Client Hints

**How do I use `Action.Hints` to improve the experience for non-HTML clients
such as CLI tools?**

The `Hints` map already supports arbitrary metadata. CLI-oriented hint keys
allow a server to communicate interaction guidance that terminal clients can
use to improve usability:

```go
action := hyper.Action{
    Name:   "Delete Contact",
    Rel:    "delete",
    Method: "DELETE",
    Target: hyper.Target{Href: "/contacts/42"},
    Hints: map[string]any{
        "confirm":     "Are you sure you want to delete this contact?",
        "destructive": true,
        "hidden":      false,
    },
}
```

Suggested CLI-relevant hint keys:

| Key           | Type     | Description                                         |
|---------------|----------|-----------------------------------------------------|
| `confirm`     | `string` | Prompt text to display before executing the action   |
| `destructive` | `bool`   | Indicates the action has destructive side effects    |
| `hidden`      | `bool`   | Suppress the action from default action listings     |

These keys are conventions, not requirements. Codecs and clients that do not
recognize them SHOULD ignore them. HTML codecs might render `destructive`
actions with a warning style, while CLI clients might display a confirmation
prompt for actions carrying a `confirm` hint.

## 16. Compliance Requirements

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
6. define the extension interfaces specified in section 17

## 17. Extensibility (Draft)

This section is a **draft** and subject to change.

Existing Go types SHOULD be able to participate in the hypermedia system by
implementing narrow, opt-in interfaces. Each interface maps to a single concern
so that a type can adopt only the extension points that are relevant to it.

### 17.1 Extension Interfaces

#### 17.1.1 RepresentationProvider

A type that can present itself as a complete `Representation`.

```go
type RepresentationProvider interface {
    HyperRepresentation() Representation
}
```

When a codec or renderer encounters a value implementing
`RepresentationProvider`, it SHOULD call `HyperRepresentation()` to obtain the
full hypermedia representation, including state, links, actions, and embedded
representations.

#### 17.1.2 NodeProvider

A type that can express its state as a `Node`.

```go
type NodeProvider interface {
    HyperNode() Node
}
```

This allows domain types to supply structured state for the `State` field of a
`Representation` without being modified to implement `Node` directly.

#### 17.1.3 ValueProvider

A type that can express itself as a `Value`.

```go
type ValueProvider interface {
    HyperValue() Value
}
```

This allows leaf domain values (e.g. custom identifiers, enumerations, money
types) to participate in `Object` or `Collection` containers without
wrapping in `Scalar`.

#### 17.1.4 LinkProvider

A type that can contribute navigational links.

```go
type LinkProvider interface {
    HyperLinks() []Link
}
```

When constructing a `Representation` from an existing domain type, the builder
or codec SHOULD merge links returned by `HyperLinks()` into the
representation's `Links` slice.

#### 17.1.5 ActionProvider

A type that can contribute available actions.

```go
type ActionProvider interface {
    HyperActions() []Action
}
```

When constructing a `Representation` from an existing domain type, the builder
or codec SHOULD merge actions returned by `HyperActions()` into the
representation's `Actions` slice.

### 17.2 Composition Rules

A single type MAY implement any combination of these interfaces. When a type
implements multiple provider interfaces, the following precedence SHOULD apply:

1. If a type implements `RepresentationProvider`, its
   `HyperRepresentation()` result SHOULD be used as the primary representation.
   Other provider interfaces on the same type MAY be ignored because the
   returned `Representation` is assumed to be complete.
2. If a type does not implement `RepresentationProvider` but implements
   `NodeProvider`, the result of `HyperNode()` SHOULD be used as the `State`
   of a constructed `Representation`.
3. `LinkProvider` and `ActionProvider` SHOULD be consulted independently to
   populate `Links` and `Actions` when building a representation from parts.

### 17.3 Discovery by Codecs and Renderers

Codecs and renderers SHOULD use Go type assertions to discover provider
interfaces on values passed to them. They MUST NOT require reflection or code
generation.

Example:

```go
func buildRepresentation(v any) Representation {
    if rp, ok := v.(RepresentationProvider); ok {
        return rp.HyperRepresentation()
    }

    rep := Representation{}

    if np, ok := v.(NodeProvider); ok {
        rep.State = np.HyperNode()
    }
    if lp, ok := v.(LinkProvider); ok {
        rep.Links = lp.HyperLinks()
    }
    if ap, ok := v.(ActionProvider); ok {
        rep.Actions = ap.HyperActions()
    }

    return rep
}
```

### 17.4 Requirements

- Extension interfaces MUST be optional; types that do not implement them
  SHALL continue to work through manual `Representation` construction.
- Provider methods MUST be safe to call concurrently and MUST NOT cause side
  effects.
- Provider methods SHOULD return new slices and structs rather than shared
  mutable state.
- Implementations MUST NOT use reflection to discover these interfaces;
  standard Go type assertions SHALL be used.

### 17.5 Compliance

A conforming implementation of `hyper` SHOULD:

1. define the extension interfaces listed above
2. check for provider interfaces via type assertion in codecs and renderers
3. document which interfaces are consulted at each stage of encoding

A conforming implementation MAY:

1. define additional provider interfaces beyond those listed here
2. accept provider interfaces on values nested inside `Object` and
   `Collection` containers

## 18. Open Questions

The following questions remain open for later revisions:

1. whether `Hints` should remain a plain map or become typed codec extensions
2. whether `Object` and `Collection` should remain minimal or grow helper APIs
3. whether JSON support should target a specific hypermedia media type first
4. whether additional provider interfaces (e.g. `EmbeddedProvider`,
   `MetaProvider`) should be added to the extensibility surface
5. whether provider interfaces should accept a `context.Context` parameter
6. whether `Hints` keys should follow a namespacing convention (e.g.
   `htmx:target` vs `hx-target`) to avoid collisions across frameworks

### 18.1 Resolved

The following questions have been resolved:

1. **`Action.Rel` vocabulary for domain-specific verbs.** Resolved in §7.2:
   `Action.Rel` is an open string vocabulary. The spec recommends well-known
   rels (`create`, `update`, `delete`, `search`) for CRUD operations. All
   other rels are domain-specific and clients should treat them as opaque
   identifiers. No namespace prefix is required.
2. **Actions discovery convention.** Resolved in §7.1: a `Link` with
   `rel: "actions"` MAY point to a resource enumerating available actions.
   Servers should prefer embedding actions directly in the `Representation`'s
   `Actions` array when possible.
