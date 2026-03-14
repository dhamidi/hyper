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
- `Client`
- `HTTPDoer`
- `CredentialStore`
- `Response`
- `FindLink`, `FindAction`, `FindEmbedded`, `ActionValues`

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
- **Polling as baseline, SSE as opt-in upgrade.** Polling via re-fetch is
  the simplest pattern and works across all clients. Servers MAY
  additionally offer `text/event-stream` for actions with
  `Hints["stream"] = true`. This allows progressive enhancement: clients
  that understand SSE get real-time updates; others fall back to polling.
  The convention does not preclude WebSocket alternatives.

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
    Accept   string   // Accepted MIME types (file upload fields)
    MaxSize  int64    // Maximum file size in bytes
    Multiple bool     // Whether the field accepts multiple files
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
- `Accept` MAY specify accepted MIME types for file upload fields (e.g., `"image/png, image/jpeg"`)
- `MaxSize` MAY specify the maximum file size in bytes for file upload fields
- `Multiple` MAY indicate that the field accepts multiple files

#### File Upload Fields

When `Type` is `"file"`, the `Accept`, `MaxSize`, and `Multiple` fields
provide additional constraints for file uploads. Codecs SHOULD encode these
fields when present and non-zero. Codecs SHALL decode these fields when
present in the wire format.

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
| `file`   | File upload input; see `Accept`, `MaxSize`, `Multiple` |

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

### 9.3 RepresentationDecoder

A `RepresentationDecoder` decodes response bodies into `Representation` values. Codecs that support both encoding (server-side) and decoding (client-side) implement both `RepresentationCodec` and `RepresentationDecoder`.

```go
type RepresentationDecoder interface {
    MediaTypes() []string
    DecodeRepresentation(context.Context, io.Reader) (Representation, error)
}
```

The `Client` uses type assertion to check whether a registered `RepresentationCodec` also implements `RepresentationDecoder`. This follows the asymmetric design principle (§9.4): codecs MAY opt into decoding without it being required.

### 9.4 Rationale

Implementations SHALL separate response encoding from request decoding because
HTML applications commonly:

- respond with `text/html`
- submit with `application/x-www-form-urlencoded`
- upload with `multipart/form-data`

A single symmetric codec abstraction SHALL NOT be required. The `RepresentationDecoder` interface (§9.3) extends this principle to client-side response decoding, allowing codecs to opt into decoding without requiring it.

### 9.5 Suggested Shared Options

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

### 9.6 StreamingCodec (optional extension)

A `StreamingCodec` extends `RepresentationCodec` with the ability to write
a sequence of representations as a stream.

```go
// StreamingCodec extends RepresentationCodec with the ability to write
// a sequence of representations as a stream.
type StreamingCodec interface {
    RepresentationCodec
    EncodeEvent(context.Context, io.Writer, Representation, EncodeOptions) error
    Flush(io.Writer) error
}
```

A `StreamingCodec` SHALL be usable as a regular `RepresentationCodec` (its
`Encode` method writes a single event and closes the stream). The
`EncodeEvent` method writes one event without closing, allowing handlers to
call it repeatedly. `Flush` ensures buffered data reaches the client.

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

### 10.4 Streaming Responses (text/event-stream)

A `RepresentationCodec` registered for `text/event-stream` MAY encode a
sequence of representations as Server-Sent Events (SSE). This is the
RECOMMENDED mechanism for server-push scenarios within hyper.

#### 10.4.1 Event Format

Each SSE event SHALL carry a single hyper `Representation` encoded in the
codec's secondary format (typically JSON). The event structure:

- `event:` field — the `Representation.Kind` value (e.g., `event: job-progress`)
- `data:` field — the JSON-encoded representation (one line, or multi-line with `data:` prefix per line)
- `id:` field (OPTIONAL) — server-assigned event ID for reconnection (`Last-Event-ID`)

#### 10.4.2 Stream Lifecycle

- The stream SHOULD begin with a representation of `Kind: "stream-open"` (or
  domain-specific equivalent) that carries `Links` and `Actions` available
  during the stream.
- The stream SHOULD end with a terminal representation (`Kind: "stream-close"`
  or domain-specific) that carries final `Links` (e.g., `rel: "download"` for
  completed jobs).
- If the connection drops, clients SHOULD reconnect using `Last-Event-ID` per
  the SSE spec.

#### 10.4.3 Content Negotiation

When `Accept: text/event-stream` is present and an `EventStreamCodec` is
registered, the `Renderer` negotiates normally. Handlers that support
streaming SHOULD check the negotiated media type and produce a stream
accordingly. Handlers that do not support streaming return a single
representation as usual.

#### 10.4.4 Relation to Async Actions

Actions with `Hints["async"] = true` MAY additionally include
`Hints["stream"] = true` to signal that the server supports SSE for this
action's job. Clients MAY request `Accept: text/event-stream` to receive
streaming progress instead of polling.

### 10.5 RespondStream API

The `Renderer` SHOULD provide a streaming variant that takes a channel or
iterator of `Representation` values and writes them as SSE events using an
`http.Flusher`.

```go
func (r Renderer) RespondStream(w http.ResponseWriter, req *http.Request, reps <-chan Representation) error
```

`RespondStream` SHALL:

1. Verify that the `http.ResponseWriter` implements `http.Flusher`
2. Find a `StreamingCodec` for `text/event-stream` among registered codecs
3. Set `Content-Type: text/event-stream` and `Cache-Control: no-cache`
4. For each `Representation` received from the channel, call `EncodeEvent`
   and `Flush`
5. Return when the channel is closed or the request context is cancelled

## 11. Client

The `Client` provides programmatic access to hyper APIs. It fetches `Representations`, follows `Links`, and submits `Actions`. All IO is mediated through interfaces so that transport, credential management, and codec selection can be replaced independently.

### 11.1 Client Struct

```go
type Client struct {
    // Transport executes HTTP requests. Defaults to http.DefaultClient.
    Transport HTTPDoer

    // Codecs maps media types to RepresentationCodecs for decoding responses.
    // The Client selects a codec based on the response Content-Type header.
    // At minimum, a JSON codec for "application/vnd.api+json" SHOULD be registered.
    Codecs []RepresentationCodec

    // SubmissionCodecs maps media types to SubmissionCodecs for encoding request bodies.
    // The Client selects a codec based on Action.Consumes.
    SubmissionCodecs []SubmissionCodec

    // Credentials provides authentication credentials for requests.
    // When non-nil, the Client calls Credentials.Credential before each request
    // and attaches the result according to the Credential's Scheme.
    // When nil, requests are sent without authentication.
    Credentials CredentialStore

    // BaseURL is the root URL of the API. All relative Target URLs
    // are resolved against this base. MUST be an absolute URL.
    BaseURL *url.URL

    // Accept is the Accept header value sent with GET requests.
    // Defaults to "application/vnd.api+json" if empty.
    Accept string

    // OnUnauthorized is called when a request receives a 401 response.
    // If it returns a non-nil Credential, the Client retries the request
    // with the new credential (exactly once). If nil, 401 responses are
    // returned as-is.
    OnUnauthorized func(ctx context.Context, resp *Response) (*Credential, error)
}
```

### 11.2 HTTPDoer Interface

The `HTTPDoer` interface abstracts HTTP request execution. The default implementation is `*http.Client` from `net/http`, which satisfies this interface.

```go
// HTTPDoer executes an HTTP request and returns the response.
// *http.Client satisfies this interface.
type HTTPDoer interface {
    Do(*http.Request) (*http.Response, error)
}
```

Callers MAY provide custom implementations for:
- logging and tracing
- retry with backoff
- circuit breaking
- testing (mock responses)

### 11.3 Credential Type and CredentialStore Interface

#### 11.3.1 Credential Type

The `Credential` type represents an authentication credential with its placement strategy.

```go
// Credential represents an authentication credential with its placement strategy.
type Credential struct {
    // Scheme determines how the credential is attached to requests.
    // Well-known values: "bearer", "apikey-header", "apikey-query".
    Scheme string

    // Value is the credential value (token, key, etc.).
    Value string

    // Header is the header name for "apikey-header" scheme.
    // Defaults to "Authorization" for "bearer".
    // Example: "X-API-Key" for API key auth.
    Header string

    // Param is the query parameter name for "apikey-query" scheme.
    // Example: "api_key".
    Param string
}
```

The Client attaches the credential based on `Scheme`:
- `"bearer"` → `Authorization: Bearer <Value>` (default when Scheme is empty)
- `"apikey-header"` → `<Header>: <Value>` (e.g., `X-API-Key: abc123`)
- `"apikey-query"` → appends `<Param>=<Value>` to the request URL query string

Convenience constructors:

```go
// BearerToken returns a Credential that attaches as "Authorization: Bearer <token>".
func BearerToken(token string) Credential

// APIKeyHeader returns a Credential that attaches the key in the named header.
func APIKeyHeader(header, key string) Credential

// APIKeyQuery returns a Credential that appends the key as a query parameter.
func APIKeyQuery(param, key string) Credential
```

#### 11.3.2 CredentialStore Interface

The `CredentialStore` interface abstracts credential retrieval and persistence. The Client calls `Credential` before each request to obtain the current credential. The Client calls `Store` after a successful login action to persist new credentials.

```go
// CredentialStore retrieves and persists authentication credentials.
type CredentialStore interface {
    // Credential returns the current credential for the given base URL.
    // Returns (Credential{}, nil) when no credential is stored.
    Credential(ctx context.Context, baseURL *url.URL) (Credential, error)

    // Store persists a credential for the given base URL.
    Store(ctx context.Context, baseURL *url.URL, cred Credential) error

    // Delete removes the stored credential for the given base URL.
    Delete(ctx context.Context, baseURL *url.URL) error
}
```

`hyper` SHOULD provide a default file-based implementation:

```go
// FileCredentialStore stores Credential values in a JSON file on disk.
// Each entry persists the Scheme, Value, Header, and Param fields so that
// the credential's placement strategy survives across process restarts.
// The default path is ~/.config/hyper/credentials.json.
type FileCredentialStore struct {
    Path string
}
```

### 11.4 Core Operations

#### 11.4.1 Fetch

`Fetch` sends a GET request to the given `Target` and decodes the response into a `Representation`.

```go
func (c *Client) Fetch(ctx context.Context, target Target) (*Response, error)
```

The method:
1. Resolves `target` to an absolute URL against `c.BaseURL`
2. Calls `c.Credentials.Credential` (if `c.Credentials` is non-nil) and attaches the credential to the request according to its `Scheme` (see §11.3.1)
3. Sets the `Accept` header to `c.Accept` (defaulting to `"application/vnd.api+json"`)
4. Executes the request via `c.Transport.Do`
5. If the response status is 401 and `c.OnUnauthorized` is non-nil, calls `c.OnUnauthorized`. If it returns a non-nil `Credential`, retries the request exactly once with the new credential attached
6. Parses the response `Content-Type` header and searches `c.Codecs` for a codec that implements `RepresentationDecoder` (§9.3) with a matching media type. If found, uses that decoder; otherwise falls back to JSON decoding for backward compatibility
7. Decodes the response body into a `Representation`
8. Returns a `*Response` containing the decoded `Representation`, HTTP status code, and response headers

#### 11.4.2 Submit

`Submit` executes an `Action` with the given field values.

```go
func (c *Client) Submit(ctx context.Context, action Action, values map[string]any) (*Response, error)
```

The method:
1. Resolves `action.Target` to an absolute URL against `c.BaseURL`
2. If `action.Method` is `GET`, encodes `values` as query parameters and sends a GET request
3. Otherwise, selects a `SubmissionCodec` from `c.SubmissionCodecs` based on `action.Consumes` (defaulting to `"application/vnd.api+json"` when `Consumes` is empty and `values` is non-empty)
4. Encodes `values` into the request body using the selected codec
5. Sets the `Content-Type` header to the selected media type
6. Calls `c.Credentials.Credential` (if `c.Credentials` is non-nil) and attaches the credential to the request according to its `Scheme` (see §11.3.1)
7. Sets the `Accept` header
8. Executes the request via `c.Transport.Do`
9. If the response status is 401 and `c.OnUnauthorized` is non-nil, calls `c.OnUnauthorized`. If it returns a non-nil `Credential`, retries the request exactly once with the new credential attached
10. Decodes the response body into a `Representation` (same Content-Type-based codec selection as `Fetch`, see §9.3)
11. Returns a `*Response`

#### 11.4.3 Follow

`Follow` is a convenience for `Fetch` that takes a `Link` instead of a `Target`.

```go
func (c *Client) Follow(ctx context.Context, link Link) (*Response, error)
```

Equivalent to `c.Fetch(ctx, link.Target)`.

### 11.5 Response

The `Response` wraps a decoded `Representation` with HTTP metadata.

```go
type Response struct {
    // Representation is the decoded hypermedia representation.
    Representation Representation

    // StatusCode is the HTTP status code.
    StatusCode int

    // Header contains the response headers.
    Header http.Header

    // Body is the raw response body, available for passthrough
    // (e.g., --json output mode). It is the caller's responsibility
    // to close it. If the Representation was decoded successfully,
    // Body will be a reader over the already-read bytes.
    Body io.ReadCloser
}

// IsSuccess returns true if StatusCode is in the 2xx range.
func (r *Response) IsSuccess() bool

// IsError returns true if StatusCode is 4xx or 5xx.
func (r *Response) IsError() bool
```

### 11.6 Navigator

The `Navigator` provides stateful traversal of a hyper API. It tracks the current `Representation` and provides fluent methods for following links, submitting actions, and inspecting state.

```go
// Navigator provides stateful traversal of a hyper API.
// It tracks the current Representation and provides fluent methods
// for following links, submitting actions, and inspecting state.
type Navigator struct {
    client  *Client
    current *Response
    history []*Response
}
```

Constructor:

```go
// Navigate starts a Navigator at the given target.
// It fetches the target and sets the result as the current position.
func (c *Client) Navigate(ctx context.Context, target Target) (*Navigator, error)
```

#### 11.6.1 Core Methods

```go
// Current returns the current Response (representation + HTTP metadata).
func (n *Navigator) Current() *Response

// Representation returns the current Representation.
// Shorthand for n.Current().Representation.
func (n *Navigator) Representation() Representation

// State returns the current representation's State node.
func (n *Navigator) State() Node

// Kind returns the current representation's Kind.
func (n *Navigator) Kind() string
```

#### 11.6.2 Navigation Methods

```go
// Follow finds a link by rel in the current representation and fetches it.
// The result becomes the new current position. The previous position is
// pushed onto the history stack.
// Returns ErrLinkNotFound if no link with the given rel exists.
func (n *Navigator) Follow(ctx context.Context, rel string) error

// Submit finds an action by rel in the current representation and submits it.
// The result becomes the new current position. The previous position is
// pushed onto the history stack.
// Returns ErrActionNotFound if no action with the given rel exists.
func (n *Navigator) Submit(ctx context.Context, rel string, values map[string]any) error

// FollowLink follows a specific Link (not looked up by rel).
// Useful when the caller has already located the link.
func (n *Navigator) FollowLink(ctx context.Context, link Link) error

// SubmitAction submits a specific Action (not looked up by rel).
// Useful when the caller has already located the action.
func (n *Navigator) SubmitAction(ctx context.Context, action Action, values map[string]any) error

// Back returns to the previous position in the history stack.
// Returns ErrNoHistory if the history stack is empty.
func (n *Navigator) Back() error

// Refresh re-fetches the current position's Self target.
// Returns an error if the current representation has no Self target.
func (n *Navigator) Refresh(ctx context.Context) error
```

#### 11.6.3 Inspection Methods

```go
// Links returns all links from the current representation.
func (n *Navigator) Links() []Link

// Actions returns all actions from the current representation.
func (n *Navigator) Actions() []Action

// FindLink returns the first link with the given rel from the current representation.
func (n *Navigator) FindLink(rel string) (Link, bool)

// FindAction returns the first action with the given rel from the current representation.
func (n *Navigator) FindAction(rel string) (Action, bool)

// Embedded returns embedded representations in the given slot from the current representation.
func (n *Navigator) Embedded(slot string) []Representation

// HasLink returns true if the current representation has a link with the given rel.
func (n *Navigator) HasLink(rel string) bool

// HasAction returns true if the current representation has an action with the given rel.
func (n *Navigator) HasAction(rel string) bool
```

#### 11.6.4 Error Sentinel Values

```go
var (
    ErrLinkNotFound   = errors.New("hyper: link not found")
    ErrActionNotFound = errors.New("hyper: action not found")
    ErrNoHistory      = errors.New("hyper: no history to go back to")
    ErrNoSelf         = errors.New("hyper: representation has no self target")
)
```

### 11.7 Representation Navigation Helpers

These free functions assist in navigating a `Representation`'s hypermedia controls. They are pure functions with no IO.

```go
// FindLink returns the first Link with the given rel, or false if not found.
func FindLink(rep Representation, rel string) (Link, bool)

// FindAction returns the first Action with the given rel, or false if not found.
func FindAction(rep Representation, rel string) (Action, bool)

// FindEmbedded returns the embedded representations in the given slot, or nil.
func FindEmbedded(rep Representation, slot string) []Representation

// ActionValues extracts default values from an Action's Fields as a map.
// This is useful for pre-populating form values before user overrides.
func ActionValues(action Action) map[string]any
```

### 11.8 Client Constructor

```go
// NewClient creates a Client with sensible defaults:
// - Transport: http.DefaultClient
// - Codecs: [JSONRepresentationCodec]
// - SubmissionCodecs: [JSONSubmissionCodec, FormSubmissionCodec]
// - Accept: "application/vnd.api+json"
func NewClient(baseURL string, opts ...ClientOption) (*Client, error)
```

`ClientOption` follows the functional options pattern:

```go
type ClientOption func(*Client)

func WithTransport(t HTTPDoer) ClientOption
func WithCredentials(cs CredentialStore) ClientOption
func WithStaticCredential(cred Credential) ClientOption
func WithCodec(c RepresentationCodec) ClientOption
func WithSubmissionCodec(c SubmissionCodec) ClientOption
func WithAccept(accept string) ClientOption
```

`WithStaticCredential` wraps a single `Credential` in a read-only `CredentialStore` that always returns it, providing a convenient shorthand for API key or fixed-token authentication.

### 11.9 Target Resolution for Client

When resolving a `Target` for an outbound request, the `Client` follows these rules:

1. If `Target.URL` is non-nil and absolute, use it directly
2. If `Target.URL` is non-nil and relative, resolve it against `c.BaseURL`
3. If `Target.Route` is non-nil, the `Client` does NOT support `RouteRef` resolution — `RouteRef` is a server-side concept. On the wire (§14.3), route references are resolved to concrete URLs by the server's `Renderer` before encoding. The `Client` only sees resolved URLs.

This means the `Client` works with any server that produces valid JSON per the wire format (§14.3), regardless of whether that server uses `hyper`, `RouteRef`, or any other URL generation strategy.

### 11.10 Usage Examples

#### CLI Agent: Fetch Root and Build Commands

```go
client, _ := hyper.NewClient("http://localhost:8080",
    hyper.WithCredentials(&hyper.FileCredentialStore{}),
)

// Fetch root representation
resp, err := client.Fetch(ctx, hyper.Path())
if err != nil {
    log.Fatal(err)
}
root := resp.Representation

// Build command tree from links and actions
for _, link := range root.Links {
    fmt.Printf("Link: %s -> %s\n", link.Rel, link.Target.URL)
}
for _, action := range root.Actions {
    fmt.Printf("Action: %s (%s)\n", action.Rel, action.Method)
}
```

#### CLI Agent: Submit an Action

```go
// User wants to create a contact
action, ok := hyper.FindAction(contactList, "create")
if !ok {
    log.Fatal("create action not found")
}

resp, err := client.Submit(ctx, action, map[string]any{
    "name":  "Alan Turing",
    "email": "alan@example.com",
})
if err != nil {
    log.Fatal(err)
}

if resp.IsSuccess() {
    created := resp.Representation
    fmt.Printf("Created: %s\n", created.Kind)
}
```

#### Server-to-Server: Follow Links Across Services

```go
// Service A fetches a representation from Service B
client, _ := hyper.NewClient("http://service-b:8080")

resp, err := client.Fetch(ctx, hyper.Path("/orders/99"))
if err != nil {
    log.Fatal(err)
}

// Follow a link to a related resource
customerLink, ok := hyper.FindLink(resp.Representation, "customer")
if ok {
    custResp, _ := client.Follow(ctx, customerLink)
    fmt.Printf("Customer: %v\n", custResp.Representation.State)
}
```

#### Handling Authentication (Bearer Token)

```go
client, _ := hyper.NewClient("http://localhost:8080",
    hyper.WithCredentials(&hyper.FileCredentialStore{
        Path: "~/.config/myapp/credentials.json",
    }),
)

// Fetch root — may be unauthenticated
resp, _ := client.Fetch(ctx, hyper.Path())

// Check for login action
loginAction, needsAuth := hyper.FindAction(resp.Representation, "login")
if needsAuth {
    // Submit login
    authResp, _ := client.Submit(ctx, loginAction, map[string]any{
        "username": "ada",
        "password": "secret",
    })

    // Extract and store token
    if token, ok := authResp.Representation.State.(hyper.Object)["token"]; ok {
        cred := hyper.BearerToken(token.(hyper.Scalar).V.(string))
        client.Credentials.Store(ctx, client.BaseURL, cred)
    }

    // Re-fetch root with credentials
    resp, _ = client.Fetch(ctx, hyper.Path())
}
```

#### API Key Authentication

```go
// Static API key in a custom header
client, _ := hyper.NewClient("https://api.example.com",
    hyper.WithStaticCredential(
        hyper.APIKeyHeader("X-API-Key", os.Getenv("API_KEY")),
    ),
)

resp, _ := client.Fetch(ctx, hyper.Path("/resources"))
```

```go
// API key as a query parameter
client, _ := hyper.NewClient("https://api.example.com",
    hyper.WithStaticCredential(
        hyper.APIKeyQuery("api_key", os.Getenv("API_KEY")),
    ),
)
```

#### OAuth Token Refresh

```go
store := &hyper.FileCredentialStore{
    Path: "~/.config/myapp/credentials.json",
}

client, _ := hyper.NewClient("https://api.example.com",
    hyper.WithCredentials(store),
)

client.OnUnauthorized = func(ctx context.Context, resp *hyper.Response) (*hyper.Credential, error) {
    // Look for a refresh action in the 401 response
    refreshAction, ok := hyper.FindAction(resp.Representation, "refresh")
    if !ok {
        return nil, nil // no refresh available
    }

    // Submit the refresh action to obtain a new token
    refreshResp, err := client.Submit(ctx, refreshAction, nil)
    if err != nil {
        return nil, err
    }

    // Extract and persist the new token
    token := refreshResp.Representation.State.(hyper.Object)["token"].(hyper.Scalar).V.(string)
    cred := hyper.BearerToken(token)
    store.Store(ctx, client.BaseURL, cred)
    return &cred, nil
}
```

#### Navigator: Stateful Traversal

```go
client, _ := hyper.NewClient("http://localhost:8080")

nav, _ := client.Navigate(ctx, hyper.Path())
nav.Follow(ctx, "contacts")
nav.Submit(ctx, "create", map[string]any{
    "name": "Ada Lovelace",
})
fmt.Println(nav.Kind())
```

#### Navigator: Conditional Navigation

```go
nav, _ := client.Navigate(ctx, hyper.Path())

if nav.HasLink("contacts") {
    nav.Follow(ctx, "contacts")

    // Browse embedded items
    for _, contact := range nav.Embedded("items") {
        fmt.Printf("%s: %v\n", contact.Kind, contact.State)
    }

    // Go back to root
    nav.Back()
}

if nav.HasAction("login") {
    nav.Submit(ctx, "login", map[string]any{
        "username": "ada",
        "password": "secret",
    })
}
```

#### Navigator: Server-to-Server Traversal

```go
nav, _ := client.Navigate(ctx, hyper.Path("/orders/99"))
// Follow linked resources across the API
if err := nav.Follow(ctx, "customer"); err == nil {
    customerState := nav.State()
    nav.Back() // return to order
}
if err := nav.Follow(ctx, "line-items"); err == nil {
    items := nav.Embedded("items")
    // process items...
}
```

### 11.11 Design Rationale

1. **`HTTPDoer` over `http.RoundTripper`**: `HTTPDoer` wraps `*http.Client` rather than `http.RoundTripper` because the Client needs cookie jar support, redirect policy, and timeout configuration that `*http.Client` provides. Wrapping at the `Do` level lets callers substitute the entire client (for testing, logging, etc.) without reconstructing these behaviors.

2. **`CredentialStore` as a separate interface**: Authentication is an IO dependency that varies between CLI (file-based), server-to-server (environment variables, vault), and testing (hardcoded tokens). Expressing it as an interface lets each context provide its own implementation without subclassing or configuration flags.

3. **No `Resolver` on Client**: The server-side `Resolver` (§8.2) converts abstract `Target` values (including `RouteRef`) to URLs at render time. The Client operates on the wire format where all targets are already resolved to URLs. Including a `Resolver` on the Client would conflate server and client concerns.

4. **`Response` wraps `Representation`**: Returning a `Response` rather than just a `Representation` gives callers access to HTTP metadata (status codes, headers) needed for error handling, caching, and content negotiation. The raw `Body` is available for passthrough (e.g., `--json` mode in the CLI).

5. **Navigation helpers are free functions**: `FindLink`, `FindAction`, `FindEmbedded`, and `ActionValues` are pure functions, not methods on `Client`. This keeps them testable without HTTP setup and usable in contexts beyond the client (e.g., server-side representation construction).

6. **`Credential` struct over bare token string**: The previous `Token() string` approach hardcoded the `Authorization: Bearer` pattern. Real-world APIs use diverse authentication strategies — API keys in custom headers, query parameter keys, and various `Authorization` schemes. The `Credential` struct carries both the value and its placement strategy, making these patterns expressible without custom `HTTPDoer` wrappers. `Scheme` is a string rather than an enum for extensibility.

7. **`OnUnauthorized` as a function, not an interface**: OAuth token refresh is the primary use case for automatic retry on 401. A single callback is simpler than a multi-method interface for this single-purpose hook. The one-retry limit prevents infinite loops when credentials are genuinely invalid. Callers who do not need refresh simply leave the field nil.

8. **Cookie auth remains implicit**: The `HTTPDoer` wraps `*http.Client`, which already provides cookie jar support. Servers that authenticate via `Set-Cookie` on login responses get automatic cookie handling on subsequent requests without any `Credential` involvement. This is by design — cookie management is a transport concern, not a credential concern.

9. **Navigator is opt-in**: `Navigator` wraps `Client`, it does not replace it. The stateless `Fetch`/`Follow`/`Submit` methods remain for callers who want explicit control. Navigator is sugar for the common "walk the API" pattern.

10. **History is bounded**: Implementations SHOULD cap the history stack (e.g., 50 entries) to prevent unbounded memory growth in long traversal sessions. Entries beyond the cap are silently dropped from the bottom of the stack.

11. **Navigator is not concurrency-safe**: A `Navigator` represents a single traversal session. Concurrent use requires separate `Navigator` instances (which can share the same `Client`).

12. **Errors on Follow/Submit do not update position**: When `Follow` or `Submit` returns a non-nil error, the current position is NOT changed. The navigator stays where it was, and the caller can inspect the error and decide how to proceed.

13. **Free functions remain alongside Navigator**: `FindLink`, `FindAction`, etc. remain as free functions. The Navigator's convenience methods (`HasLink`, `FindLink`) delegate to them. This keeps the free functions testable independently and useful outside the Navigator context.

### 11.12 Streaming Fetch

```go
// FetchStream sends a GET with Accept: text/event-stream and returns
// a channel of Response values, one per SSE event.
func (c *Client) FetchStream(ctx context.Context, target Target) (<-chan *Response, error)
```

The client SHOULD decode each `data:` payload using the registered JSON codec
and yield a `Response` with the `event:` field mapped to
`Representation.Kind`. The returned channel is closed when the stream ends or
the context is cancelled.

## 12. HTML Codec

### 12.1 HTML Role

HTML SHALL be treated as a first-class target format.

### 12.2 Action Rendering

An HTML codec SHOULD:

- render `Link` as `<a>`
- render a fieldless `GET` action as a link or button
- render an action with fields as a `<form>`
- render non-`GET` actions using a form submission mechanism appropriate for
  the host application

### 12.3 Embedded Representations

An HTML codec SHOULD support:

- full-document rendering
- fragment rendering
- nested rendering of embedded representations

### 12.4 Codec-Specific Hints

`Action.Hints` MAY carry codec-specific or framework-specific hint keys.

The core model does not define or require any specific keys. Codecs MAY
interpret hint keys that are relevant to their output format. See the
Interaction Points section for concrete examples of how different front-end
frameworks use `Hints`.

## 13. Markdown Codec

### 13.1 Markdown Role

Markdown SHOULD be treated as a read-oriented alternate representation.

### 13.2 Limitations

A Markdown codec MAY degrade actions, because Markdown does not natively model
interactive form controls.

A Markdown codec MAY:

- render links normally
- render actions as prose, lists, or reference blocks
- omit UI-specific hints

## 14. JSON Codec

### 14.1 JSON Role

JSON support SHOULD preserve hypermedia semantics rather than reducing a
representation to plain state only.

### 14.2 Minimum Behavior

A JSON codec SHOULD preserve:

- state
- links
- actions
- embedded representations

### 14.3 JSON Wire Format

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

#### 14.3.1 State Encoding

State values SHALL be encoded according to their type:

- `Scalar` — encoded as the underlying JSON value (string, number, boolean,
  or null).
- `RichText` — encoded as an object with a type discriminator:
  `{"_type": "richtext", "mediaType": "text/markdown", "source": "..."}`.
- `Object` — encoded as a JSON object whose keys map to encoded values.
- `Collection` — encoded as a JSON array of encoded values.

#### 14.3.2 Link Encoding

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

#### 14.3.3 Action Encoding

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

#### 14.3.4 Field Encoding

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

#### 14.3.5 Embedded Representation Encoding

The `embedded` key SHALL be a JSON object whose keys are slot names and
whose values are arrays of encoded representations (following the same
top-level structure recursively).

#### 14.3.6 Target Encoding

In JSON output, targets SHALL be represented as resolved URL strings under
the `href` key. The `self` field SHALL be encoded as
`{"href": "<resolved-url>"}` or omitted when absent. The `Target.URL` field
(a `*url.URL`) SHALL be serialized to its string form via `URL.String()`.

#### 14.3.7 Divergence from Existing Formats

The `hyper` JSON wire format is intentionally self-contained and does not
conform to HAL, Siren, or JSON:API. Key differences:

- HAL uses `_links` and `_embedded`; `hyper` uses `links` and `embedded`.
- Siren separates `properties` from `entities`; `hyper` uses `state` for all
  application data and `embedded` for sub-representations.
- JSON:API mandates `data`, `attributes`, and `relationships`; `hyper` uses a
  flat structure with `kind` and `state`.

Implementations that need interoperability with these formats SHOULD provide
separate codec implementations.

For interoperability with JSON:API clients, the separate `jsonapi` codec
package (module `github.com/dhamidi/hyper/jsonapi`) provides a
`RepresentationCodec` and `SubmissionCodec` that map between `hyper`
representations and the JSON:API wire format. See that package's documentation
for mapping details and known limitations (e.g., `Action` encoding).

### 14.4 Codec Selection Guidance

Implementations typically register one or more codecs depending on the
audience and use case. The following guidelines help choose the right codec:

| Scenario | Recommended Codec | Rationale |
|----------|------------------|-----------|
| Public REST APIs, browser clients, mobile apps | `jsonapi` (`application/vnd.api+json`) | Broad ecosystem tooling (Ember Data, Axios serializers, JSONAPI::Resources). |
| Third-party integrations | `jsonapi` | External consumers expect a well-known wire format. |
| hyper-to-hyper communication | Native JSON (§14.3) | Full fidelity: hints, ordered links, actions, and rich-text all round-trip. |
| CLI `--json` output | Native JSON (§14.3) | Scripts and tooling benefit from the 1:1 mapping of `Representation`. |
| Debugging / introspection | Native JSON (§14.3) | No information is dropped or restructured. |

When building a server that serves both audiences, register both codecs and
let content negotiation (`Accept` header) select the appropriate one at
request time. The `jsonapi` codec SHOULD be treated as the default for
public-facing REST endpoints.

## 15. Examples

### 15.1 Representation with an Update Action

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

### 15.2 Embedded Inline Field Editor

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

### 15.3 Handler with Negotiated HTML or Markdown and Form Decoding

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

### 15.4 Explicit Response Format

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

### 15.5 Root Representation (API Entry Point)

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

## 16. Interaction Points

This section shows how `hyper` integrates with the broader Go ecosystem
through short, focused examples. Each subsection poses a concrete question
and answers it with minimal code.

### 16.1 Routing with `github.com/dhamidi/dispatch`

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

### 16.2 Routing with `net/http`'s `http.ServeMux`

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

### 16.3 HTMX with `html/template`

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

### 16.4 Hotwire (Turbo + Stimulus)

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

### 16.5 Component Abstraction with `github.com/dhamidi/htmlc`

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

    // Surface representation-level hints.
    if len(r.Hints) > 0 {
        scope["hints"] = r.Hints
    }

    // Surface actions as structured data keyed by rel.
    // Also build an actionList array for enumeration components.
    if len(r.Actions) > 0 {
        actions := make(map[string]map[string]any, len(r.Actions))
        actionList := make([]map[string]any, 0, len(r.Actions))
        for _, a := range r.Actions {
            actionScope := map[string]any{
                "name":   a.Name,
                "rel":    a.Rel,
                "method": a.Method,
            }
            if len(a.Hints) > 0 {
                actionScope["hints"] = a.Hints
                // Flatten hx-* hints for direct attribute spreading.
                hxAttrs := make(map[string]any)
                for k, v := range a.Hints {
                    if strings.HasPrefix(k, "hx-") {
                        hxAttrs[k] = v
                    }
                }
                if len(hxAttrs) > 0 {
                    actionScope["hxAttrs"] = hxAttrs
                }
            }
            if len(a.Fields) > 0 {
                actionScope["fields"] = fieldsToScope(a.Fields)
            }
            actions[a.Rel] = actionScope
            actionList = append(actionList, actionScope)
        }
        scope["actions"] = actions
        scope["actionList"] = actionList
    }

    // Surface links as structured data keyed by rel.
    if len(r.Links) > 0 {
        links := make(map[string]map[string]any, len(r.Links))
        for _, l := range r.Links {
            links[l.Rel] = map[string]any{
                "rel":   l.Rel,
                "title": l.Title,
            }
        }
        scope["links"] = links
    }

    return scope
}

// fieldsToScope converts action fields into template-friendly maps.
func fieldsToScope(fields []hyper.Field) []map[string]any {
    result := make([]map[string]any, len(fields))
    for i, f := range fields {
        m := map[string]any{
            "name":     f.Name,
            "type":     f.Type,
            "required": f.Required,
            "readOnly": f.ReadOnly,
        }
        if f.Value != nil {
            m["value"] = f.Value
        }
        if f.Label != "" {
            m["label"] = f.Label
        }
        if f.Help != "" {
            m["help"] = f.Help
        }
        if f.Error != "" {
            m["error"] = f.Error
        }
        if len(f.Options) > 0 {
            opts := make([]map[string]any, len(f.Options))
            for j, o := range f.Options {
                opts[j] = map[string]any{
                    "value":    o.Value,
                    "label":    o.Label,
                    "selected": o.Selected,
                }
            }
            m["options"] = opts
        }
        result[i] = m
    }
    return result
}
```

The `hxAttrs` map extracts `hx-*` keys from `Action.Hints` so that
templates can spread them onto elements using `v-bind` (see below). The
codec is responsible for injecting the resolved URL as `hx-{method}` into
`hxAttrs` and as `href` into the action scope before rendering —
`representationToScope` does not resolve targets. For example, the htmlc
codec would add `hxAttrs["hx-delete"] = "/contacts/42"` and
`actionScope["href"] = "/contacts/42"` after resolving `Action.Target`
through the `Resolver`.

The `actionList` array contains the same action scope maps as the `actions`
map, but in their original declaration order. This supports enumeration
components (see §16.7) that render all available actions without knowing
their rels in advance.

The `fieldsToScope` helper now includes `readOnly`, `help`, and `options`
on each field map, giving `<field>` components (§16.7) enough information
to render any field type — including `select`, `checkbox-group`, and
`textarea` — without additional processing.

Note that `links` entries do not include `href` — URLs are resolved by the
codec through `Resolver` and injected into the scope separately. The same
applies to `actions` entries; the codec adds resolved URLs after calling
`representationToScope`.

#### Attribute Spreading with `v-bind`

`htmlc` supports spreading a map of attributes onto an element using
`v-bind` with a map value. When `v-bind` receives a `map[string]any`, each
key-value pair becomes an HTML attribute on the element:

```vue
<!-- Spread hx-* attributes from action hints -->
<button v-bind="actions.delete.hxAttrs">
  {{ actions.delete.name }}
</button>

<!-- Produces (after codec resolves target): -->
<!-- <button hx-delete="/contacts/42" hx-confirm="..." hx-target="closest tr" hx-swap="outerHTML swap:1s">Delete</button> -->
```

Templates may mix data-driven and hard-coded attributes. For example,
`hx-target` might be hard-coded (it is layout-specific) while `hx-confirm`
comes from hints (it is data-specific):

```vue
<button v-bind="actions.delete.hxAttrs" hx-target="closest tr">
  Delete
</button>
```

Individual `:attr` bindings (e.g., `:href="editHref"`) continue to work as
before. The map form of `v-bind` is additive — it does not replace
individually bound attributes.

#### Convention: When to Use Data-Driven Hints vs. Hard-Coded Attributes

Both patterns are valid and may coexist in the same template:

- **Data-driven** (`v-bind="actions.delete.hxAttrs"`): Best when the same
  representation serves multiple codecs, when a generic codec iterates over
  hints, or when attributes are determined by the server (e.g., confirmation
  messages that vary per record).
- **Hard-coded** (`hx-target="closest tr"`): Best when the attribute is
  intrinsic to the template layout and will not vary across representations.

App-specific templates MAY hard-code all htmx attributes for clarity.
Data-driven hints are opt-in per template, not a requirement.

Use the engine in a handler:

```go
func contactHandler(eng *htmlc.Engine, w http.ResponseWriter, r *http.Request) {
    rep := buildContactRepresentation()
    scope := representationToScope(rep)
    _ = eng.RenderPage(w, "contact", scope)
}
```

### 16.6 Reusable Component Abstractions

**How do I avoid repeating boilerplate when rendering actions, forms, and
fields across multiple templates?**

The `representationToScope` function (§16.5) surfaces actions, links, and
fields in a structured, template-friendly format. Any component-capable
template engine can consume this structure to build reusable UI components
that eliminate manual construction of buttons, forms, and inputs in every
template. `hyper` itself does not ship or require any particular template
engine or component library.

This section illustrates the pattern with three example components written
in `htmlc` / Vue SFC syntax: `<action>` renders a single action as the
appropriate HTML element, `<field>` renders a single field as the
appropriate input, and `<actions>` enumerates all non-hidden actions for a
representation. The same pattern applies to any template engine that
supports composition — Go `html/template` with partials, `templ`, or
custom rendering code.

#### 16.6.1 The `<action>` Component

The `<action>` component receives an action scope (as produced by
`representationToScope`) via `v-bind` and renders the appropriate HTML
element based on the action's method and fields:

| Condition | Rendered Element |
|---|---|
| `method == "GET"` and no fields | `<a>` with `href` |
| `method != "GET"` and no fields | `<button>` with `hx-*` attributes |
| Has fields | `<form>` with nested `<field>` components |

```vue
<!-- components/action.vue -->
<template>
  <!-- GET actions with no fields: render as link -->
  <a
    v-if="!hasFields && isGet"
    v-bind="hxAttrs"
    :href="href">
    <slot>{{ name }}</slot>
  </a>

  <!-- Non-GET actions with no fields: render as button -->
  <button
    v-if="!hasFields && !isGet"
    v-bind="hxAttrs"
    :class="{ destructive: hints && hints.destructive }"
    type="button">
    <slot>{{ name }}</slot>
  </button>

  <!-- Actions with fields: render as form -->
  <form
    v-if="hasFields"
    :method="formMethod"
    :action="href"
    v-bind="formHxAttrs">
    <slot name="fields">
      <field v-for="f in fields" :key="f.name" v-bind="f" />
    </slot>
    <button type="submit">
      <slot name="submit">{{ name }}</slot>
    </button>
  </form>
</template>
```

**Computed properties:**

- `isGet` — `method === "GET"`
- `hasFields` — `fields && fields.length > 0`
- `formMethod` — maps HTTP method to HTML form method: `"GET"` stays
  `"GET"`, everything else becomes `"POST"` (with `hx-*` carrying the
  actual method)
- `hxAttrs` — the `hx-*` attribute map from `Action.Hints`, as extracted
  by `representationToScope`
- `formHxAttrs` — for forms, the `hx-*` attributes go on the `<form>`
  element rather than the submit button

**Hint-driven behavior:**

| Hint Key | Effect |
|---|---|
| `hx-*` | Spread as HTML attributes on the element |
| `destructive` | Adds `class="destructive"` for styling |
| `confirm` | Translated to `hx-confirm` by the codec |
| `hidden` | The `<actions>` wrapper skips rendering entirely |

**Slot-based customization:**

- **Default slot** — custom button or link content (overrides `action.name`)
- **`fields` slot** — custom field layout (overrides auto-generated fields)
- **`submit` slot** — custom submit button content

Usage:

```vue
<!-- Simple: render delete action as button with hx-* attributes -->
<action v-bind="actions.delete" />

<!-- Custom content via default slot -->
<action v-bind="actions.delete">Remove this contact</action>

<!-- Form action with custom field layout via fields slot -->
<action v-bind="actions.create || actions.update">
  <template #fields>
    <fieldset>
      <legend>Contact Values</legend>
      <field v-for="f in (actions.create || actions.update).fields"
        :key="f.name" v-bind="f" />
    </fieldset>
  </template>
</action>
```

#### 16.6.2 The `<field>` Component

The `<field>` component renders a single field scope (from
`fieldsToScope`) as the appropriate HTML input element:

```vue
<!-- components/field.vue -->
<template>
  <!-- text, email, tel, url, number, date, password, hidden -->
  <p v-if="isInput">
    <label :for="name">{{ label || name }}</label>
    <input
      :id="name"
      :type="type"
      :name="name"
      :value="value"
      :required="required"
      :readonly="readOnly" />
    <span class="error" v-if="error">{{ error }}</span>
    <span class="help" v-if="help">{{ help }}</span>
  </p>

  <!-- select -->
  <p v-if="type === 'select'">
    <label :for="name">{{ label || name }}</label>
    <select :id="name" :name="name" :required="required">
      <option v-for="opt in options" :key="opt.value"
        :value="opt.value" :selected="opt.selected">
        {{ opt.label }}
      </option>
    </select>
    <span class="error" v-if="error">{{ error }}</span>
  </p>

  <!-- checkbox -->
  <p v-if="type === 'checkbox'">
    <label>
      <input type="checkbox" :name="name" :value="value" :checked="value" />
      {{ label || name }}
    </label>
  </p>

  <!-- checkbox-group (bulk operations) -->
  <div v-if="type === 'checkbox-group'">
    <label v-for="opt in options" :key="opt.value">
      <input type="checkbox" :name="name" :value="opt.value"
        :checked="opt.selected" />
      {{ opt.label }}
    </label>
  </div>

  <!-- textarea -->
  <p v-if="type === 'textarea'">
    <label :for="name">{{ label || name }}</label>
    <textarea :id="name" :name="name" :required="required">{{ value }}</textarea>
    <span class="error" v-if="error">{{ error }}</span>
    <span class="help" v-if="help">{{ help }}</span>
  </p>
</template>
```

**Computed properties:**

- `isInput` — true when `type` is one of `text`, `email`, `tel`, `url`,
  `number`, `date`, `password`, `hidden` (single-line input types)

The `<field>` component relies entirely on the field scope produced by
`fieldsToScope` (§16.5). Each key in the scope map (`name`, `type`,
`value`, `required`, `readOnly`, `label`, `help`, `error`, `options`)
maps directly to a template binding.

#### 16.6.3 The `<actions>` Enumeration Component

The `<actions>` component renders all non-hidden actions for a
representation. It consumes the `actionList` array from
`representationToScope`:

```vue
<!-- components/actions.vue -->
<template>
  <div class="actions" v-if="actionList.length > 0">
    <template v-for="a in actionList" :key="a.rel">
      <action v-bind="a" v-if="!a.hints || !a.hints.hidden" />
    </template>
  </div>
</template>
```

Usage:

```vue
<!-- Render all non-hidden actions for a representation -->
<actions :action-list="actionList" />
```

This is especially powerful for:

- **Admin interfaces** that auto-generate UI from the data model
- **Generic resource browsers** that render any representation
- **Prototyping** — get a working UI by defining only the representation,
  before writing custom templates

#### 16.6.4 When to Use Components vs. Custom Templates

The `<action>` and `<field>` components eliminate boilerplate for common
patterns but are not always the right choice:

| Use Case | Recommendation |
|---|---|
| Standard CRUD buttons and forms | Use `<action>` — reduces repetition |
| Actions with app-specific layout (e.g., inline validation, conditional fields) | Use `<action>` with slots for custom field layout |
| Highly custom UI (e.g., drag-and-drop, multi-step wizards) | Write custom template markup |
| Admin/generic views | Use `<actions>` for full enumeration |
| Prototyping | Use `<actions>` to get working UI quickly |

Components and manual markup may coexist in the same template. For
example, a template might use `<action>` for a delete button while
rendering a form manually for finer control:

```vue
<template>
  <!-- Custom form with full control -->
  <form method="POST" :action="href">
    <!-- custom field layout here -->
    <button type="submit">Save</button>
  </form>

  <!-- Component-based delete button -->
  <action v-bind="actions.delete" />
</template>
```

#### 16.6.5 Relationship to htmx

The `<action>` component is htmx-aware but not htmx-dependent. Without
`hx-*` hints, it renders standard HTML forms and links that work with
normal browser navigation. With `hx-*` hints, it progressively enhances
elements with htmx attributes. This aligns with htmx's philosophy of
progressive enhancement.

The component does not generate `hx-*` attributes itself — it only
spreads attributes that the server has placed in `Action.Hints`. This
keeps the component generic and ensures that the server remains the
authority on interaction behavior.

#### 16.6.6 Shipping Components

The `<action>`, `<field>`, and `<actions>` components shown above are
illustrative — they are not part of `hyper` or `htmlc`. Applications
that use a component-capable template engine can define these components
at the project level (e.g., as `.vue` files in the project's component
directory) or package them as a shared component library.

Many applications will not need generic components at all. Custom
templates that hard-code the markup for each representation kind are
simpler and provide full control over layout and behavior. The generic
components are most valuable for admin interfaces, prototyping, and
applications that render many similar CRUD views.

### 16.7 CLI Client Hints

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

## 17. Compliance Requirements

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
6. define the extension interfaces specified in section 18

## 18. Extensibility (Draft)

This section is a **draft** and subject to change.

Existing Go types SHOULD be able to participate in the hypermedia system by
implementing narrow, opt-in interfaces. Each interface maps to a single concern
so that a type can adopt only the extension points that are relevant to it.

### 18.1 Extension Interfaces

#### 18.1.1 RepresentationProvider

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

#### 18.1.2 NodeProvider

A type that can express its state as a `Node`.

```go
type NodeProvider interface {
    HyperNode() Node
}
```

This allows domain types to supply structured state for the `State` field of a
`Representation` without being modified to implement `Node` directly.

#### 18.1.3 ValueProvider

A type that can express itself as a `Value`.

```go
type ValueProvider interface {
    HyperValue() Value
}
```

This allows leaf domain values (e.g. custom identifiers, enumerations, money
types) to participate in `Object` or `Collection` containers without
wrapping in `Scalar`.

#### 18.1.4 LinkProvider

A type that can contribute navigational links.

```go
type LinkProvider interface {
    HyperLinks() []Link
}
```

When constructing a `Representation` from an existing domain type, the builder
or codec SHOULD merge links returned by `HyperLinks()` into the
representation's `Links` slice.

#### 18.1.5 ActionProvider

A type that can contribute available actions.

```go
type ActionProvider interface {
    HyperActions() []Action
}
```

When constructing a `Representation` from an existing domain type, the builder
or codec SHOULD merge actions returned by `HyperActions()` into the
representation's `Actions` slice.

#### 18.1.6 EmbeddedProvider

A type that can contribute embedded sub-representations.

```go
type EmbeddedProvider interface {
    HyperEmbedded() map[string][]Representation
}
```

When constructing a `Representation` from an existing domain type, the builder
or codec SHOULD merge embedded representations returned by `HyperEmbedded()`
into the representation's `Embedded` map. If a slot key already exists in the
target representation, the provider's representations SHALL be appended to the
existing slice for that slot.

### 18.2 Composition Rules

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
4. `EmbeddedProvider` SHOULD be consulted to populate `Embedded` when building
   a representation from parts, following the same pattern as `LinkProvider`
   and `ActionProvider`.

### 18.3 Discovery by Codecs and Renderers

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
    if ep, ok := v.(EmbeddedProvider); ok {
        embedded := ep.HyperEmbedded()
        if len(embedded) > 0 {
            rep.Embedded = make(map[string][]Representation, len(embedded))
            for slot, reps := range embedded {
                rep.Embedded[slot] = append(rep.Embedded[slot], reps...)
            }
        }
    }

    return rep
}
```

### 18.4 Requirements

- Extension interfaces MUST be optional; types that do not implement them
  SHALL continue to work through manual `Representation` construction.
- Provider methods MUST be safe to call concurrently and MUST NOT cause side
  effects.
- Provider methods SHOULD return new slices and structs rather than shared
  mutable state.
- Implementations MUST NOT use reflection to discover these interfaces;
  standard Go type assertions SHALL be used.

### 18.5 Compliance

A conforming implementation of `hyper` SHOULD:

1. define the extension interfaces listed above
2. check for provider interfaces via type assertion in codecs and renderers
3. document which interfaces are consulted at each stage of encoding

A conforming implementation MAY:

1. define additional provider interfaces beyond those listed here
2. accept provider interfaces on values nested inside `Object` and
   `Collection` containers

## 19. Open Questions

The following questions remain open for later revisions:

1. ~~whether `Hints` should remain a plain map or become typed codec extensions~~
   Resolved: see §19.1 item 4.
2. ~~whether `Object` and `Collection` should remain minimal or grow helper APIs~~
   Resolved: `StateFrom` (§6.4) provides a convenience constructor for `Object`.
   `WithValues` and `WithErrors` (§7.4) provide field derivation helpers.
3. ~~whether JSON support should target a specific hypermedia media type first~~
   Resolved: see §19.1 item 5.
4. ~~whether additional provider interfaces (e.g. `EmbeddedProvider`,
   `MetaProvider`) should be added to the extensibility surface~~
   Resolved: see §19.1 item 8.
5. ~~whether provider interfaces should accept a `context.Context` parameter~~
   Resolved: see §19.1 item 6.
6. ~~whether `Hints` keys should follow a namespacing convention (e.g.
   `htmx:target` vs `hx-target`) to avoid collisions across frameworks~~
   Resolved: see §19.1 item 7.

### 19.1 Resolved

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
4. **`Hints` type.** Resolved: `Hints` remains `map[string]any`. The plain map
   provides maximum flexibility for heterogeneous codec-specific and UI-specific
   directives. Codecs consume only the keys they recognize and ignore the rest.
5. **JSON hypermedia media type.** Resolved: JSON:API (https://jsonapi.org/) is
   the RECOMMENDED interoperability format for machine clients that require a
   standardized hypermedia JSON dialect. This support SHALL be delivered as a
   separate codec package, not as part of the core `hyper` module. The native
   `hyper` JSON wire format (§14.3) remains the default for `hyper`-to-`hyper`
   communication.
6. **`context.Context` in provider interfaces.** Resolved: provider interface
   methods do not accept `context.Context`. Provider methods are pure data
   transformations that MUST NOT perform I/O. Callers that need context-aware
   construction should resolve context-dependent data before invoking providers.
7. **`Hints` key namespacing.** Resolved: no namespace convention. Hint keys
   SHOULD use the conventions of their target framework (e.g., `hx-target` for
   htmx). Codecs consume only the keys they recognize and MUST ignore unknown
   keys, which prevents collisions in practice.
8. **Additional provider interfaces.** Resolved: `EmbeddedProvider` added to
   §18.1. `MetaProvider` and `HintsProvider` were evaluated but not added —
   metadata and hints are handler-level concerns, not domain-type concerns, and
   are better set directly on the `Representation`.
