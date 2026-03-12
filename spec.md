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
    Hints    map[string]any
}
```

### 6.1.1 Semantics

- `Kind` SHOULD be an application-defined semantic label
- `Self` MAY identify the representation's canonical target
- `State` SHOULD contain the primary application state
- `Links` SHALL represent navigational controls
- `Actions` SHALL represent available transitions
- `Embedded` MAY contain named related or fragment representations
- `Meta` MAY contain application-specific metadata that is independent of
  rendering — for example, pagination totals, cache directives, or
  domain-specific annotations. Rendering directives (codec-specific or
  UI-specific hints) SHOULD use `Hints` instead.
- `Hints` MAY contain codec-specific or UI-specific rendering directives for
  the representation as a whole. This parallels `Action.Hints` (§7.2) but
  applies at the representation level. Common uses include htmx attributes
  (`hx-trigger`, `hx-swap`, `hx-target`) that apply to the representation's
  root element.

#### Pagination Metadata

For paginated list representations, the following `Meta` keys are
RECOMMENDED:

| Key            | Type  | Description                              |
|----------------|-------|------------------------------------------|
| `total_count`  | `int` | Total number of items across all pages   |
| `page_size`    | `int` | Number of items per page                 |
| `page_count`   | `int` | Total number of pages                    |
| `current_page` | `int` | The current page number (1-based)        |

All pagination `Meta` keys are OPTIONAL. Servers that cannot efficiently
compute totals (e.g., cursor-based pagination) MAY omit `total_count` and
`page_count`.

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

### 6.3.2 RichText Convenience Constructors

The library SHALL provide convenience constructors for common `RichText`
media types:

```go
// Markdown returns a RichText value with MediaType "text/markdown".
//
//   hyper.Markdown("Ada Lovelace wrote the first algorithm.")
//   // equivalent to:
//   // hyper.RichText{MediaType: "text/markdown", Source: "Ada Lovelace wrote the first algorithm."}
func Markdown(source string) RichText

// PlainText returns a RichText value with MediaType "text/plain".
//
//   hyper.PlainText("Hello, world.")
//   // equivalent to:
//   // hyper.RichText{MediaType: "text/plain", Source: "Hello, world."}
func PlainText(source string) RichText
```

These are thin wrappers that produce the same `RichText` values manual
construction produces. They are particularly useful inside `StateFrom` calls:

```go
hyper.StateFrom(
    "id", c.ID,
    "name", c.Name,
    "bio", hyper.Markdown(c.Bio),
)
```

### 6.4 State Construction Helpers

Building `Object` values manually requires wrapping every leaf in `Scalar`,
which obscures intent:

```go
State: hyper.Object{
    "id":    hyper.Scalar{V: c.ID},
    "name":  hyper.Scalar{V: c.Name},
    "email": hyper.Scalar{V: c.Email},
}
```

The library SHALL provide a convenience constructor that eliminates this
ceremony:

```go
// StateFrom builds an Object from alternating key-value pairs.
// Values that do not implement the Value interface are automatically
// wrapped in Scalar{V: v}. Values that already implement Value
// (e.g. RichText) are used as-is.
//
// Panics if len(pairs) is odd or if any key is not a string.
//
//   hyper.StateFrom("id", 42, "name", "Ada")
//   // equivalent to:
//   // hyper.Object{"id": hyper.Scalar{V: 42}, "name": hyper.Scalar{V: "Ada"}}
func StateFrom(pairs ...any) Object
```

`StateFrom` is a thin wrapper — it produces the same `Object` map that manual
construction produces. Codecs and renderers see no difference.

Because `RichText` implements the `Value` interface, the `Markdown()` and
`PlainText()` convenience constructors (§6.3.2) compose naturally with
`StateFrom`:

```go
hyper.StateFrom(
    "id", c.ID,
    "name", c.Name,
    "bio", hyper.Markdown(c.Bio),
)
```

The `Markdown(c.Bio)` call produces a `RichText` value, which `StateFrom`
passes through as-is (it already implements `Value`). Scalar values like
`c.ID` and `c.Name` are wrapped in `Scalar{V: ...}` automatically.

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

#### Pagination Links

For paginated collections, implementations SHOULD use the following
IANA-registered link relations (RFC 8288):

| Rel     | Description                              |
|---------|------------------------------------------|
| `next`  | Link to the next page in the series      |
| `prev`  | Link to the previous page in the series  |
| `first` | Link to the first page in the series     |
| `last`  | Link to the last page in the series      |

These are standard IANA link relations. Clients MAY use the presence of a
`next` link to determine whether more pages are available. The absence of
a `next` link indicates the current page is the last.

Pagination links SHOULD be expressed via `RouteRef` with `Query` parameters
(see §8.1), avoiding manual URL construction. Using the `Route` convenience
constructor (§8.1.2):

```go
hyper.Link{
    Rel:    "next",
    Target: hyper.Route("contacts.list").WithQuery(url.Values{"page": {"3"}}),
}
```

Or using the `WithQuery` convenience method on `URL`-based targets:

```go
hyper.Link{
    Rel:    "next",
    Target: hyper.Path("contacts").WithQuery(url.Values{"page": {"3"}}),
}
```

### 7.1.1 Link Convenience Constructor

The library SHALL provide a convenience constructor for the common case
where only `Rel` and `Target` are needed:

```go
// NewLink constructs a Link with the given rel and target.
// Optional fields (Title, Type) can be set on the returned value.
//
//   hyper.NewLink("next", hyper.Route("contacts.list").WithQuery(url.Values{"page": {"3"}}))
//   // equivalent to:
//   // hyper.Link{Rel: "next", Target: hyper.Route("contacts.list").WithQuery(url.Values{"page": {"3"}})}
func NewLink(rel string, target Target) Link
```

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

#### Async Action Conventions

An `Action` MAY produce an asynchronous result — that is, the action's
response is a job representation rather than the final result. This section
defines lightweight conventions for signaling and handling async actions.

##### Action-Level Signaling

`Action.Hints` MAY include `"async": true` to signal that the action's
response will be an async job representation rather than the final result.

When `"async": true` is present, clients SHOULD expect the response to be a
job representation with state transitions, and MAY automatically poll for
completion.

Actions without `"async": true` in `Hints` behave synchronously as before.
This convention is opt-in and backward compatible.

##### Job Representation Conventions

When an action produces an async job, the response SHOULD follow these
conventions:

- **Kind**: The `Representation.Kind` SHOULD indicate a job type (e.g.,
  `"export-job"`, or any domain-specific kind).
- **Status state key**: The `State` object SHOULD include a `"status"` key
  with one of: `"pending"`, `"processing"`, `"complete"`, `"failed"`.
  APIs MAY use additional status values beyond this recommended set.
- **Poll interval hint**: The `Representation.Meta` MAY include a
  `"poll-interval"` key (integer, seconds) to suggest how frequently clients
  should re-fetch the resource. The poll interval belongs on the job
  representation, not on the action that created it, because the server
  knows the appropriate interval based on the job's current state.
- **Progress hint**: The `State` object MAY include a `"progress"` key
  (number, 0-100) to indicate completion percentage.
- **Dynamic links**: `Links` MAY appear or disappear as the job's status
  changes. For example, a `Link` with `rel: "download"` appears only when
  status is `"complete"`. Clients MUST NOT cache or assume a fixed set of
  links for job representations.

##### Error State

When status is `"failed"`, the `State` object SHOULD include an `"error"`
key with a human-readable error message.

The job representation MAY include an `Action` with `rel: "retry"` to allow
resubmission.

##### Design Rationale

- **Opt-in via Hints.** The `"async": true` hint preserves backward
  compatibility — actions without it behave synchronously as before.
- **Lightweight conventions, not strict schema.** The `"status"` key and
  its values are recommended, not required.
- **Dynamic links for result delivery.** Rather than inventing a new
  "result" concept, the existing `Links` mechanism handles it — a
  `download` link appears when the job completes. This is consistent with
  the hypermedia model.
- **No WebSocket/SSE requirement.** Polling via re-fetch is the simplest
  pattern and works across all clients. The convention does not preclude
  servers from offering WebSocket or SSE alternatives, but the baseline is
  simple HTTP polling.

#### Design Note

An action is the core semantic primitive.

An HTML codec MAY render an action with fields as a `<form>`, but the core
model SHALL NOT require a separate `Form` type.

### 7.2.1 Action Convenience Constructor

The library SHALL provide a convenience constructor for actions with the
three most common fields:

```go
// NewAction constructs an Action with the given name, HTTP method, and target.
// Optional fields (Rel, Consumes, Produces, Fields, Hints) can be set on the
// returned value.
//
//   hyper.NewAction("Save", "PUT", hyper.Route("contacts.update", "id", "42"))
//   // equivalent to:
//   // hyper.Action{Name: "Save", Method: "PUT", Target: hyper.Route("contacts.update", "id", "42")}
func NewAction(name, method string, target Target) Action
```

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

#### Multi-Value Semantics

For multi-value field types (`checkbox-group`, `multiselect`), multiple
`Option` entries MAY have `Selected: true` simultaneously. The submitted
value SHALL be the set of all selected option values.

For multi-value field types, `Field.Value` MAY be a slice (e.g., `[]string`)
representing the currently selected values. Codecs SHOULD use `Options` with
`Selected` flags as the canonical source when both `Value` and `Options` are
present.

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
| `checkbox-group` | Multiple selections from `Options`; each selected option contributes a value |
| `multiselect` | Selection of multiple values from `Options` (rendered as a multi-select list or similar) |

The `checkbox-group` and `multiselect` types indicate that the field accepts
zero or more values. Codecs SHOULD render `checkbox-group` as a group of
checkboxes and `multiselect` as a `<select multiple>` element (or equivalent
in non-HTML codecs).

Codecs MAY support additional type values beyond this list. Unknown types
SHOULD be treated as `text` by codecs that do not recognize them.

### 7.3.2 Field Convenience Constructor

The library SHALL provide a convenience constructor for fields with name
and type:

```go
// NewField constructs a Field with the given name and type.
// Optional fields (Value, Required, Label, Help, Options, Error, ReadOnly)
// can be set on the returned value.
//
//   hyper.NewField("email", "email")
//   // equivalent to:
//   // hyper.Field{Name: "email", Type: "email"}
func NewField(name, fieldType string) Field
```

### 7.4 Field Convenience Functions

Fields for a resource are often defined once (schema) and then reused in
multiple contexts: create actions (no values), update actions (current values),
and validation error responses (submitted values plus errors). The library
SHALL provide convenience functions for deriving context-specific field slices
from a shared definition:

```go
// WithValues returns a shallow copy of fields with Value populated from the
// given map. Fields whose Name does not appear in the map retain their
// existing Value. The original slice is not modified.
//
// Used for update actions where fields should show current values.
func WithValues(fields []Field, values map[string]any) []Field

// WithErrors returns a shallow copy of fields with Value and Error populated
// from the given maps. Fields whose Name does not appear in a map retain
// their existing Value or Error. The original slice is not modified.
//
// Used for validation error responses where fields should show submitted
// values and per-field error messages.
func WithErrors(fields []Field, values map[string]any, errors map[string]string) []Field
```

These functions encourage defining field metadata once:

```go
var contactFields = []Field{
    {Name: "name", Type: "text", Label: "Name", Required: true},
    {Name: "email", Type: "email", Label: "Email", Required: true},
    {Name: "phone", Type: "tel", Label: "Phone"},
}

// Create action — fields with no values (user fills them in)
action.Fields = contactFields

// Update action — fields pre-populated with current values
action.Fields = hyper.WithValues(contactFields, map[string]any{
    "name": c.Name, "email": c.Email, "phone": c.Phone,
})

// Validation error — fields with submitted values and error messages
action.Fields = hyper.WithErrors(contactFields, submittedValues, validationErrors)
```

Both functions return new slices and do not mutate the original fields.

## 8. Targets and URL Resolution

### 8.1 Target

```go
type Target struct {
    URL   *url.URL
    Route *RouteRef
}

type RouteRef struct {
    Name   string
    Params map[string]string
    Query  url.Values
}
```

`Query` MAY contain query parameters to be appended to the resolved URL.
When both `Params` (path parameters) and `Query` (query parameters) are
present, the resolver SHALL first resolve the path using `Params`, then
append `Query` as the URL query string. `Query` is also valid on
`URL`-based targets; resolvers SHALL append `Query` to the URL when
non-nil.

### 8.1.1 Requirements

- Exactly one of `URL` or `Route` SHOULD be set
- `URL` SHALL represent a directly specified target
- `Route` SHALL represent an abstract, named route target
- `Query` MAY be set on either `URL` or `Route` targets to append query parameters

### 8.1.2 Convenience Constructors

The library SHALL provide convenience constructors for common URL construction
patterns:

```go
// Path constructs a Target from path segments.
// Each segment is path-escaped individually.
//
//   hyper.Path("contacts", "42")        → /contacts/42
//   hyper.Path("contacts", id, "notes") → /contacts/{id}/notes
//   hyper.Path()                        → /
func Path(segments ...string) Target

// Pathf constructs a Target from a format string.
// The format string is processed with fmt.Sprintf, then parsed as a URL path.
//
//   hyper.Pathf("/contacts/%d", c.ID)   → /contacts/42
func Pathf(format string, args ...any) Target

// ParseTarget parses a raw URL string into a Target.
// Returns an error if the URL is malformed.
func ParseTarget(rawURL string) (Target, error)

// MustParseTarget is like ParseTarget but panics on error.
// Suitable for static URLs known at compile time.
//
//   hyper.MustParseTarget("/contacts")
func MustParseTarget(rawURL string) Target

// Route constructs a route-based Target from a route name and alternating
// key-value path parameter pairs.
// Panics if len(params) is odd.
//
//   hyper.Route("contacts.show", "id", "42")
//   // equivalent to:
//   // hyper.Target{Route: &hyper.RouteRef{
//   //     Name: "contacts.show",
//   //     Params: map[string]string{"id": "42"},
//   // }}
//
//   hyper.Route("contacts.list")
//   // equivalent to:
//   // hyper.Target{Route: &hyper.RouteRef{Name: "contacts.list"}}
func Route(name string, params ...string) Target
```

`Route` is the highest-impact convenience constructor — route-based targets
appear in nearly every handler and require 4+ lines of nested struct
initialization each time. With zero params (e.g., `Route("contacts.list")`),
it produces a `Target` with `Route: &RouteRef{Name: "contacts.list"}` and
an empty `Params` map.

A `WithQuery` method SHALL be provided to return a copy of the `Target` with
the given query parameters set. This works with both `URL`-based and
`Route`-based targets:

```go
// WithQuery returns a copy of the Target with the given query parameters.
//
//   hyper.Path("contacts").WithQuery(url.Values{"page": {"2"}})
//   → /contacts?page=2
func (t Target) WithQuery(q url.Values) Target
```

A `Ptr` method SHALL be provided to obtain a pointer to a `Target`, for use
in fields typed `*Target` (e.g. `Representation.Self`):

```go
func (t Target) Ptr() *Target
```

### 8.2 Resolver

```go
type Resolver interface {
    ResolveTarget(context.Context, Target) (*url.URL, error)
}
```

### 8.2.1 Semantics

A resolver SHALL:

1. return `URL` directly when present — if `Query` is non-nil, append it as
   the URL query string
2. resolve `Route` when present — substitute `Params` into the named route's
   path template, then append `Query` as the URL query string (if non-nil)
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
func (r Renderer) RespondWithMode(http.ResponseWriter, *http.Request, int, Representation, RenderMode) error
```

### 10.2 Semantics

`Respond` SHALL:

1. inspect the request `Accept` header
2. choose the best available codec
3. encode the representation using `RenderDocument` as the default `RenderMode`
4. write the chosen response media type

`RespondAs` SHALL:

1. bypass normal content negotiation
2. select a codec matching the requested media type
3. encode the representation using that codec

`RespondWithMode` SHALL behave identically to `Respond` except that the
supplied `RenderMode` is passed through to the codec via `EncodeOptions.Mode`,
overriding the default. This allows handlers to emit a fragment (e.g. for htmx
partial responses) by passing `RenderFragment`.

The renderer SHOULD set the `Content-Type` response header accordingly.

### 10.3 Non-Representation Responses

Not every HTTP response in a `hyper` application is a `Representation`.
Handlers MAY write responses directly to the `http.ResponseWriter` without
using `Renderer.Respond` for cases including but not limited to:

- trivial fragment responses (e.g., a single validation message string)
- empty responses (e.g., 204 No Content after a delete)
- binary or streamed content (e.g., file downloads)
- redirect responses (3xx status codes)

The `Renderer` is intended for responses that carry a structured
`Representation`. Handlers that produce non-representational responses are
expected to use standard `net/http` patterns directly.

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
  "meta": { ... },
  "hints": { ... }
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
`{"href": "<resolved-url>"}` or omitted when absent. The `Target.URL` field
(a `*url.URL`) SHALL be serialized to its string form via `URL.String()`.

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
    Self: hyper.Route("contacts.show", "id", "42").Ptr(),
    State: hyper.StateFrom(
        "id", 42,
        "name", "Ada",
        "email", "ada@example.com",
        "bio", hyper.Markdown("Ada Lovelace wrote the first algorithm."),
    ),
    Actions: []hyper.Action{
        {
            Name:     "Save",
            Rel:      "update",
            Method:   "PUT",
            Target:   hyper.Route("contacts.update", "id", "42"),
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
    State: hyper.StateFrom(
        "name", "Ada",
        "email", "ada@example.com",
    ),
    Embedded: map[string][]hyper.Representation{
        "email_editor": {
            {
                Kind: "contact-email-editor",
                Actions: []hyper.Action{
                    {
                        Name:     "Save Email",
                        Rel:      "update-email",
                        Method:   "PUT",
                        Target:   hyper.Route("contacts.email.update", "id", "42"),
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

        resolved, err := a.Resolver.ResolveTarget(r.Context(),
            hyper.Route("contacts.show", "id", strconv.FormatInt(contact.ID, 10)),
        )
        if err != nil {
            http.Error(w, "server error", http.StatusInternalServerError)
            return
        }

        http.Redirect(w, r, resolved.String(), http.StatusSeeOther)
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
    Self: hyper.Path().Ptr(),
    Links: []hyper.Link{
        {Rel: "contacts", Target: hyper.MustParseTarget("/contacts"), Title: "Contacts"},
        {Rel: "settings", Target: hyper.MustParseTarget("/settings"), Title: "Settings"},
    },
    Actions: []hyper.Action{
        {
            Name:   "Search",
            Rel:    "search",
            Method: "GET",
            Target: hyper.MustParseTarget("/search"),
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

func (r DispatchResolver) ResolveTarget(_ context.Context, t hyper.Target) (*url.URL, error) {
    if t.URL != nil {
        return t.URL, nil
    }
    if t.Route == nil {
        return nil, errors.New("hyper: missing target")
    }
    s, err := r.Router.Path(t.Route.Name, dispatch.Params(t.Route.Params))
    if err != nil {
        return nil, err
    }
    return url.Parse(s)
}
```

### 15.2 Routing with `net/http`'s `http.ServeMux`

**How do I use `hyper.Target` with the standard library's `http.ServeMux`,
which does not support reverse routing?**

Because `ServeMux` has no named routes, use direct `URL` targets instead
of `RouteRef`:

```go
// URLResolver resolves targets that carry a direct URL.
// It returns an error for RouteRef targets because ServeMux
// does not support reverse routing.
type URLResolver struct{}

func (URLResolver) ResolveTarget(_ context.Context, t hyper.Target) (*url.URL, error) {
    if t.URL != nil {
        return t.URL, nil
    }
    return nil, errors.New("hyper: ServeMux does not support named routes")
}
```

Build representations with `URL`-based targets:

```go
func contactHandler(w http.ResponseWriter, r *http.Request) {
    rep := hyper.Representation{
        Kind: "contact",
        Self: hyper.Path("contacts", "42").Ptr(),
        State: hyper.Object{
            "name": hyper.Scalar{V: "Ada"},
        },
        Actions: []hyper.Action{
            {
                Name:   "Save",
                Rel:    "update",
                Method: "PUT",
                Target: hyper.Path("contacts", "42"),
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
    Target: hyper.Route("contacts.email.update", "id", "42"),
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
    Self: hyper.Path("contacts", "42").Ptr(),
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
            Target: hyper.Path("contacts", "42"),
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
    Target: hyper.Path("contacts", "42"),
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
2. ~~whether `Object` and `Collection` should remain minimal or grow helper APIs~~
   Resolved: `StateFrom` (§6.4) provides a convenience constructor for `Object`.
   `WithValues` and `WithErrors` (§7.4) provide field derivation helpers.
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
3. **Async action and job conventions.** Resolved in §7.2: `Action.Hints`
   MAY include `"async": true` to signal that the response is an async job.
   Job representations use a `"status"` state key with recommended values
   (`"pending"`, `"processing"`, `"complete"`, `"failed"`),
   `Meta.poll-interval` for polling guidance, and dynamic `Links` for result
   delivery (e.g., a `download` link appears on completion).
