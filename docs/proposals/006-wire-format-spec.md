# Proposal 006: JSON Wire Format Specification

- **Status:** Draft
- **Date:** 2026-03-17
- **Author:** hyper contributors

## 1. Problem Statement

The hyper JSON wire format is currently defined implicitly by the Go
`JSONCodec()` implementation in `json_codec.go`. There is no standalone
specification that a developer in another language can read and implement
against. This creates several concrete problems:

- **Cross-language implementation requires reading Go source.** The TypeScript
  server example in `use-cases/cli-server.md` §5 demonstrates that the wire
  format is implementable in other languages, but the author had to reverse-
  engineer the format from Go code and driver program output.

- **Go-internal types leak into the mental model.** `RouteRef` is a Go-side
  routing convenience that must never appear on the wire, but the spec does not
  state this explicitly. Third-party implementors need to know which types are
  interoperable and which are Go-internal (`cli-server.md` §13).

- **Media type is unspecified.** The JSON codec registers `application/json` as
  its media type, but the wire format carries hypermedia controls (links,
  actions, embedded representations) that distinguish it from plain JSON data.
  The spec does not recommend a media type identifier for content negotiation
  (`rest-cli.md` §17).

- **Default `Consumes` is undefined.** When `Action.Consumes` is empty, clients
  do not know what content type to use for submissions (`rest-cli.md` §17).

- **Value type coercion is unspecified.** The JSON wire format does not specify
  how to distinguish `"42"` (string) from `42` (number) in `Field.Value`. Clients
  need `Field.Type`-based coercion rules (`rest-cli.md` §17).

- **No compliance testing path.** Without a formal specification, there is no way
  to build a portable compliance test suite that validates wire-format documents
  across implementations (`cli-server.md` §13).

## 2. Background

### 2.1 Current State

The reference implementation is `JSONCodec()` in `json_codec.go`. It encodes
`Representation` values as JSON objects using manual map construction (not struct
tags). The codec is symmetric: `Encode` produces JSON and `DecodeRepresentation`
parses it back. The client-side decoder in `client.go` mirrors the encoding
rules.

Key characteristics of the current implementation:

- **Omit-when-empty:** All optional fields are omitted from the JSON output when
  they have zero values. This applies to top-level keys (`kind`, `self`, `state`,
  `links`, `actions`, `embedded`, `meta`, `hints`) and to nested fields within
  links, actions, and fields.
- **Target resolution:** `Target` values are resolved to URL strings at encoding
  time via the `Resolver` interface. The wire format contains only the resolved
  URL string, never a `RouteRef`.
- **RichText tagging:** `RichText` values are encoded with a `_type` discriminator
  to distinguish them from plain scalars.

### 2.2 Existing Wire Format Evidence

The wire format is documented implicitly in several places:

- `json_codec.go` — the reference encoder/decoder
- `client.go` — client-side decoder (`decodeRepresentation`, `decodeScalarValue`)
- `json_codec_test.go` — encoding/decoding test cases with expected JSON
- `use-cases/rest-cli.md` §5.3 — annotated JSON example of a contact list
- `use-cases/cli-server.md` §5 — TypeScript server producing the same format
- `internal/tryme/main.go` — driver program exercising the full encoding pipeline

### 2.3 JSON:API Contrast

The `jsonapi/` subpackage provides an alternative codec that maps `Representation`
to the JSON:API wire format (`application/vnd.api+json`). This demonstrates that
the hyper data model is codec-agnostic — the same `Representation` can be
serialized to different wire formats. The native hyper JSON format described in
this proposal is one such format; JSON:API is another.

## 3. Proposal

### 3.1 Media Type

The hyper JSON wire format SHOULD use the media type:

```
application/vnd.hyper+json
```

This follows the `vnd.` vendor tree convention for application-specific media
types. The `+json` structured syntax suffix indicates that the format is JSON
and can be parsed by any JSON parser.

Servers MAY also serve the format as `application/json` for clients that do not
perform content negotiation. When both are available, `application/vnd.hyper+json`
is preferred for clients that understand hyper's hypermedia controls.

The Go `JSONCodec()` implementation SHOULD register both `application/json` and
`application/vnd.hyper+json` as supported media types. The existing
`application/json` registration ensures backwards compatibility.

### 3.2 Representation

A Representation is a JSON object. All keys are OPTIONAL and MUST be omitted
when the value is empty (empty string, null, empty array, or empty object).

```json
{
  "kind":     "<string>",
  "self":     { "href": "<url-string>" },
  "state":    <object-or-array>,
  "links":    [ <link>, ... ],
  "actions":  [ <action>, ... ],
  "embedded": { "<slot>": [ <representation>, ... ], ... },
  "meta":     { "<key>": <any>, ... },
  "hints":    { "<key>": <any>, ... }
}
```

| Key        | Type                                     | Description |
|------------|------------------------------------------|-------------|
| `kind`     | string                                   | Application-defined semantic label for this representation. |
| `self`     | object `{"href": "<url>"}`               | Canonical URL of this resource. See §3.7 Target. |
| `state`    | object or array                          | Primary application state. See §3.5 Node. |
| `links`    | array of Link objects                    | Navigational controls. See §3.3 Link. |
| `actions`  | array of Action objects                  | Available state transitions. See §3.4 Action. |
| `embedded` | object mapping slot names to arrays      | Named groups of nested Representations. |
| `meta`     | object                                   | Application metadata (opaque to the format). |
| `hints`    | object                                   | Codec and UI rendering directives. |

### 3.3 Link

A Link is a JSON object representing a navigational control.

```json
{
  "rel":   "<string>",
  "href":  "<url-string>",
  "title": "<string>",
  "type":  "<media-type-string>"
}
```

| Key     | Required | Type   | Description |
|---------|----------|--------|-------------|
| `rel`   | Yes      | string | Link relation type (e.g., `"self"`, `"next"`, `"root"`). |
| `href`  | Yes      | string | Resolved URL of the link target. |
| `title` | No       | string | Human-readable label. Omit when empty. |
| `type`  | No       | string | Expected media type of the target. Omit when empty. |

### 3.4 Action

An Action is a JSON object representing an available state transition.

```json
{
  "name":     "<string>",
  "rel":      "<string>",
  "method":   "<string>",
  "href":     "<url-string>",
  "consumes": [ "<media-type>", ... ],
  "produces": [ "<media-type>", ... ],
  "fields":   [ <field>, ... ],
  "hints":    { "<key>": <any>, ... }
}
```

| Key        | Required | Type             | Description |
|------------|----------|------------------|-------------|
| `name`     | Yes      | string           | Identifier for the action. |
| `rel`      | No       | string           | Semantic relation (e.g., `"create"`, `"update"`, `"delete"`). |
| `method`   | No       | string           | HTTP method (`"GET"`, `"POST"`, `"PUT"`, `"DELETE"`, etc.). |
| `href`     | No       | string           | Resolved URL of the action target. |
| `consumes` | No       | array of strings | Accepted submission media types. |
| `produces` | No       | array of strings | Likely response media types. |
| `fields`   | No       | array of Fields  | Input metadata. See §3.4.1 Field. |
| `hints`    | No       | object           | Codec-specific metadata. |

#### 3.4.1 Field

A Field describes a single input parameter for an Action.

```json
{
  "name":     "<string>",
  "type":     "<string>",
  "value":    <any>,
  "required": true,
  "readOnly": true,
  "label":    "<string>",
  "help":     "<string>",
  "options":  [ <option>, ... ],
  "error":    "<string>",
  "accept":   "<string>",
  "maxSize":  <integer>,
  "multiple": true
}
```

| Key        | Required | Type             | Description |
|------------|----------|------------------|-------------|
| `name`     | Yes      | string           | Field name (used as the key in submission payloads). |
| `type`     | No       | string           | Input type (see §3.4.3 Field Types). |
| `value`    | No       | any              | Current/default value. |
| `required` | No       | boolean          | Whether the field is mandatory. Omit when `false`. |
| `readOnly` | No       | boolean          | Whether the field is read-only. Omit when `false`. |
| `label`    | No       | string           | Human-readable label for display. |
| `help`     | No       | string           | Help text describing the field. |
| `options`  | No       | array of Options | Enumerated choices. See §3.4.2 Option. |
| `error`    | No       | string           | Validation error message for this field. |
| `accept`   | No       | string           | Accepted MIME types (for file fields). |
| `maxSize`  | No       | integer          | Maximum file size in bytes. Omit when `0`. |
| `multiple` | No       | boolean          | Whether the field accepts multiple files. Omit when `false`. |

#### 3.4.2 Option

An Option represents one choice in an enumerated field.

```json
{
  "value":    "<string>",
  "label":    "<string>",
  "selected": true
}
```

| Key        | Required | Type    | Description |
|------------|----------|---------|-------------|
| `value`    | Yes      | string  | The option value submitted to the server. |
| `label`    | No       | string  | Human-readable label. Omit when empty. |
| `selected` | No       | boolean | Whether this option is pre-selected. Omit when `false`. |

#### 3.4.3 Field Types

The `type` key on a Field uses a vocabulary aligned with HTML input types.
Recommended values:

| Type       | JSON value type | Description |
|------------|-----------------|-------------|
| `text`     | string          | Free-form text. |
| `email`    | string          | Email address. |
| `tel`      | string          | Telephone number. |
| `url`      | string          | URL. |
| `password` | string          | Password (clients should mask input). |
| `number`   | number          | Numeric value. |
| `hidden`   | any             | Hidden field (not displayed to users). |
| `date`     | string          | Date in ISO 8601 format (`YYYY-MM-DD`). |
| `datetime` | string          | Date-time in ISO 8601 format. |
| `select`   | string          | Single selection from `options`. |
| `textarea` | string          | Multi-line text. |
| `file`     | —               | File upload (see `accept`, `maxSize`, `multiple`). |

Implementations SHOULD treat unknown type values as `text`.

### 3.5 Node (State)

The `state` key holds the primary application data. It MUST be one of:

- **Object** — a JSON object mapping string keys to Value entries.
- **Collection** — a JSON array of Value entries.

### 3.6 Value

A Value is either a **Scalar** or a **RichText**.

#### 3.6.1 Scalar

A Scalar is a bare JSON value: string, number, boolean, or null. It appears
directly in the state object or collection without any wrapper.

```json
{
  "state": {
    "name": "Ada Lovelace",
    "age": 36,
    "active": true,
    "deleted_at": null
  }
}
```

#### 3.6.2 RichText

A RichText value carries formatted text with a media type. It is encoded as a
JSON object with a `_type` discriminator:

```json
{
  "_type":     "richtext",
  "mediaType": "<media-type-string>",
  "source":    "<string>"
}
```

| Key         | Required | Type   | Description |
|-------------|----------|--------|-------------|
| `_type`     | Yes      | string | MUST be `"richtext"`. Discriminator for decoders. |
| `mediaType` | Yes      | string | Media type of the source content (e.g., `"text/markdown"`, `"text/plain"`). |
| `source`    | Yes      | string | The formatted text content. |

Example within a state object:

```json
{
  "state": {
    "name": "Ada Lovelace",
    "bio": {
      "_type": "richtext",
      "mediaType": "text/markdown",
      "source": "Wrote the **first algorithm**."
    }
  }
}
```

Decoders MUST check for the presence of `_type` equal to `"richtext"` to
distinguish RichText values from plain JSON objects that happen to appear as
state values.

### 3.7 Target (Wire Format)

On the wire, targets are always resolved URLs. The `self` key on a
Representation and the `href` key on Links and Actions are plain URL strings.

The `self` target uses a wrapper object:

```json
"self": { "href": "/contacts/42" }
```

Links and Actions flatten the target into an `href` key directly:

```json
{ "rel": "next", "href": "/contacts?page=2" }
```

#### 3.7.1 RouteRef is Go-Internal

`RouteRef` is a Go-side convenience for server-side route resolution. It MUST
NOT appear in the wire format. All targets MUST be resolved to URL strings
before encoding.

An encoder that encounters an unresolvable `RouteRef` (i.e., no `Resolver` is
configured) MUST return an error rather than serializing the `RouteRef` fields
to JSON. This ensures that route references never leak into the wire format
accidentally.

Implementations in other languages do not need a `RouteRef` type — they should
produce URL strings directly.

### 3.8 Default `Consumes` Behavior

When `Action.Consumes` is absent or empty:

- For actions **with fields**: clients SHOULD submit the request body as
  `application/json`.
- For actions **without fields**: clients SHOULD send no request body.

Servers SHOULD explicitly populate `Action.Consumes` when the submission
format matters. The default of `application/json` aligns with the most common
case and the built-in `JSONSubmissionCodec`.

### 3.9 Content Type and Codec Selection

`Action.Consumes` and `Action.Produces` specify media types that map to the
codec system:

- **`Consumes`** tells clients which `SubmissionCodec` to use for encoding the
  request body. A client seeing `"consumes": ["application/json"]` should use the
  JSON submission codec.
- **`Produces`** tells clients which `RepresentationCodec` to expect in the
  response. A client seeing `"produces": ["application/vnd.hyper+json"]` should
  use the hyper JSON codec for decoding.

When multiple media types are listed, the client SHOULD use the first one it
supports (preference order is server-defined).

### 3.10 Field Value Type Coercion

`Field.Value` is typed as `any` in the wire format, but JSON has limited type
discrimination. Clients SHOULD use `Field.Type` to coerce values:

| Field Type  | Coercion Rule |
|-------------|---------------|
| `number`    | Parse as JSON number. If the wire value is a string (e.g., `"42"`), parse it as a number. |
| `text`, `email`, `tel`, `url`, `password`, `textarea` | Treat as string. If the wire value is a number, convert to its string representation. |
| `select`    | Match against `Option.Value` strings. |
| `hidden`    | Preserve the JSON type as-is (string, number, boolean, or null). |
| `date`, `datetime` | Treat as string in ISO 8601 format. |

When `Field.Type` is absent, clients SHOULD preserve the JSON type as received.

### 3.11 Omit-When-Empty Rules

The wire format follows a consistent omit-when-empty policy to keep payloads
compact. A field MUST be omitted from the JSON output when:

| Value type       | Omit condition |
|------------------|----------------|
| string           | Empty string `""` |
| boolean          | `false` |
| integer          | `0` |
| array            | Empty or null |
| object (map)     | Empty or null |
| any (`value`)    | `null` |

This applies recursively to all levels: top-level Representation keys, Link
fields, Action fields, Field fields, and Option fields.

The only exceptions are the `name` key on Actions and Fields, and the `rel` and
`href` keys on Links, which are always present as they serve as identifiers.

## 4. Examples

### 4.1 Complete Annotated Example

The following JSON document demonstrates every wire format feature — a contact
list with embedded items, links, actions, fields with options, rich text state,
and metadata:

```json
{
  "kind": "contact-list",
  "self": {
    "href": "/contacts"
  },
  "state": {
    "title": "All Contacts"
  },
  "links": [
    {
      "rel": "root",
      "href": "/",
      "title": "Home"
    },
    {
      "rel": "next",
      "href": "/contacts?page=2"
    }
  ],
  "actions": [
    {
      "name": "Create Contact",
      "rel": "create",
      "method": "POST",
      "href": "/contacts",
      "consumes": ["application/json"],
      "fields": [
        {
          "name": "name",
          "type": "text",
          "label": "Full Name",
          "required": true
        },
        {
          "name": "email",
          "type": "email",
          "label": "Email Address",
          "required": true
        },
        {
          "name": "category",
          "type": "select",
          "label": "Category",
          "options": [
            { "value": "personal", "label": "Personal" },
            { "value": "work", "label": "Work", "selected": true }
          ]
        }
      ]
    },
    {
      "name": "Search",
      "method": "GET",
      "href": "/contacts",
      "fields": [
        {
          "name": "q",
          "type": "text",
          "label": "Search"
        }
      ]
    }
  ],
  "embedded": {
    "items": [
      {
        "kind": "contact",
        "self": { "href": "/contacts/1" },
        "state": {
          "id": 1,
          "name": "Ada Lovelace",
          "email": "ada@example.com",
          "bio": {
            "_type": "richtext",
            "mediaType": "text/markdown",
            "source": "Wrote the **first algorithm**."
          }
        },
        "links": [
          { "rel": "self", "href": "/contacts/1", "title": "Ada Lovelace" }
        ],
        "actions": [
          {
            "name": "Update",
            "rel": "update",
            "method": "PUT",
            "href": "/contacts/1",
            "fields": [
              { "name": "name", "type": "text", "value": "Ada Lovelace", "required": true },
              { "name": "email", "type": "email", "value": "ada@example.com", "required": true }
            ]
          },
          {
            "name": "Delete",
            "rel": "delete",
            "method": "DELETE",
            "href": "/contacts/1"
          }
        ]
      },
      {
        "kind": "contact",
        "self": { "href": "/contacts/2" },
        "state": {
          "id": 2,
          "name": "Grace Hopper",
          "email": "grace@example.com"
        },
        "links": [
          { "rel": "self", "href": "/contacts/2", "title": "Grace Hopper" }
        ]
      }
    ]
  },
  "meta": {
    "total_count": 42,
    "page": 1,
    "page_size": 20
  },
  "hints": {
    "layout": "table"
  }
}
```

### 4.2 Minimal Representation

A valid hyper JSON representation can be as small as an empty object. The
simplest useful representation has just `kind` and `state`:

```json
{
  "kind": "greeting",
  "state": {
    "message": "Hello, world!"
  }
}
```

### 4.3 Collection State

When `state` is a collection (array) instead of an object:

```json
{
  "kind": "tag-list",
  "state": ["go", "hypermedia", "api"]
}
```

### 4.4 Validation Error Response

A server responding to invalid input with `Field.Error` values (HTTP 422):

```json
{
  "kind": "contact",
  "actions": [
    {
      "name": "Create Contact",
      "rel": "create",
      "method": "POST",
      "href": "/contacts",
      "fields": [
        {
          "name": "name",
          "type": "text",
          "value": "",
          "required": true,
          "error": "Name is required."
        },
        {
          "name": "email",
          "type": "email",
          "value": "not-an-email",
          "required": true,
          "error": "Invalid email address."
        }
      ]
    }
  ]
}
```

### 4.5 File Upload Field

An action with a file upload field:

```json
{
  "name": "Upload Photo",
  "method": "POST",
  "href": "/contacts/1/photo",
  "fields": [
    {
      "name": "photo",
      "type": "file",
      "accept": "image/png, image/jpeg",
      "maxSize": 5242880,
      "required": true
    }
  ]
}
```

## 5. Alternatives Considered

### 5.1 Custom Media Type vs `application/json`

**Option A: `application/vnd.hyper+json`** (proposed)

Pros:
- Clear signal that the response contains hypermedia controls.
- Enables content negotiation: a server can serve plain JSON data at
  `application/json` and the hypermedia representation at
  `application/vnd.hyper+json`.
- Follows established patterns (JSON:API uses `application/vnd.api+json`,
  HAL uses `application/hal+json`).

Cons:
- Adds a media type that clients must know about.
- Requires explicit `Accept` header for content negotiation.

**Option B: `application/json` with a profile link**

Using `application/json; profile="https://hyper.example/v1"` would keep the
base media type as `application/json` while signaling the profile.

Pros:
- Works with existing `application/json` tooling.
- Profile is a standard mechanism (RFC 6906).

Cons:
- Profile parameter is not widely supported by HTTP intermediaries.
- Less discoverable than a dedicated media type.
- Harder to use in content negotiation.

**Decision:** Recommend `application/vnd.hyper+json` as the primary media type,
with `application/json` as a fallback. The dedicated media type is clearer and
aligns with established hypermedia format conventions.

### 5.2 JSON Schema vs Prose Specification

**Option A: Prose specification** (proposed)

Define the wire format in prose with tables and examples, as in this document.

Pros:
- Readable by humans; easy to understand without tooling.
- Can express semantic constraints (e.g., "MUST NOT contain RouteRef") that
  JSON Schema cannot.
- Matches the style of the existing hyper documentation.

Cons:
- Not machine-readable; cannot be used for automated validation.

**Option B: JSON Schema**

Publish a formal JSON Schema alongside the prose.

Pros:
- Machine-readable; enables automated validation and code generation.
- Can serve as the basis for a compliance test suite.

Cons:
- Adds maintenance burden (schema must stay in sync with prose).
- JSON Schema cannot express all constraints (e.g., the RichText `_type`
  discriminator within dynamically-typed state values).

**Decision:** Start with prose specification. A JSON Schema can be added as a
companion artifact in a follow-up proposal, once the prose specification is
stable.

## 6. Open Questions

1. **Should a JSON Schema be published alongside this specification?** A JSON
   Schema would enable automated validation and serve as the foundation for a
   compliance test suite. However, it adds maintenance burden and cannot express
   all constraints (e.g., the `_type` discriminator in dynamically-typed state).
   This could be addressed in a separate proposal.

2. **Should the compliance test suite be part of this proposal or separate?** A
   portable test suite (e.g., a set of JSON documents with expected parse
   results) would let server and client authors in any language verify their
   implementations. This is a significant effort and may warrant its own
   proposal with its own scope and deliverables.

3. **Should `Embedded` allow non-array values?** The current wire format always
   wraps embedded representations in arrays, even for single items. An
   alternative would allow `"embedded": {"item": {...}}` for singletons. The
   array-always approach is simpler and avoids type ambiguity for decoders.

4. **Should `meta` and `hints` have any reserved keys?** Currently both are
   opaque objects. Proposal 004 defines a hint vocabulary, and various use cases
   use `meta` keys like `total_count` and `poll-interval`. Should this spec
   reserve any keys, or leave that to separate proposals?

5. **Should the `_type` discriminator use a different prefix?** The `_type` key
   for RichText uses an underscore prefix to avoid collision with application
   state keys. Alternative prefixes (`@type`, `$type`) or a different mechanism
   (a wrapper object like `{"richtext": {...}}`) could be considered. The
   underscore prefix is established in the current implementation and changing it
   would be a breaking change.
