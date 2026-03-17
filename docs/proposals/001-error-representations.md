# Proposal 001: Standard Error Representations

- **Status:** Draft
- **Date:** 2026-03-17
- **Author:** hyper contributors

## 1. Problem Statement

Every use-case exploration built with hyper has independently invented its own
error representation format. The CLI server use case created an ad hoc
`kind: "error"` convention with `State` keys `"status"` and `"message"`. The
REST CLI use case assumed `"error"` kind with `"message"` but noted the spec
defines nothing. The agent streaming use case introduced `"stream-error"` with
`"code"` and `"message"`. The blog platform added `"title"` to the mix.

Without a standard convention:

- Generic clients (CLIs, navigators, AI agents) cannot present errors
  consistently because they must guess at the shape of error responses.
- Each server author re-invents field names, leading to subtle
  incompatibilities (`"status"` vs `"code"`, `"message"` vs `"detail"`).
- Validation errors (422) use a separate mechanism (`Field.Error`) that is
  implicitly supported by the `Field` type but not explicitly documented as a
  pattern.

A standard error convention is the most frequently identified gap across all
use-case documents.

## 2. Background

### 2.1 Error Patterns Found in Use Cases

The following table summarizes error representation patterns discovered across
use-case explorations:

| Use Case | Status | Kind | State Keys | Field.Error | Links/Actions |
|---|---|---|---|---|---|
| cli-server (404) | 404 | `"error"` | `status`, `message` | No | `rel: "root"` |
| cli-server (422) | 422 | Same as success | Values preserved | Yes (per-field) | — |
| rest-cli (401) | 401 | `"error"` | `message` | No | `rel: "login"` |
| blog-platform (403) | 403 | `"error"` | `status`, `title`, `message` | No | `rel: "list"` |
| blog-platform (422) | 422 | `"post-form"` | Values preserved | Yes (per-field) | — |
| agent-streaming | SSE | `"stream-error"` | `code`, `message` | No | retry `Action` |

### 2.2 Existing `Field.Error` Mechanism

The library already provides a validation error mechanism through `Field.Error`
and the `WithErrors` convenience function:

```go
type Field struct {
    Name     string
    Type     string
    Required bool
    Value    any
    Error    string   // ← per-field validation error
    Options  []Option
    Hints    map[string]any
}

func WithErrors(fields []Field, values map[string]any, errors map[string]string) []Field
```

`WithErrors` populates both submitted values and per-field error messages,
enabling form re-presentation with inline errors. This pattern is used
consistently for 422 responses across all use cases.

### 2.3 External Standards

**RFC 9457 — Problem Details for HTTP APIs** defines a JSON format for HTTP
error responses with fields `type` (URI), `title`, `status`, `detail`, and
`instance`. It is widely adopted in REST APIs and is the IETF standard for
this problem.

**JSON:API errors** define an `errors` array where each entry may carry `id`,
`status`, `code`, `title`, `detail`, `source` (pointer/parameter/header), and
`meta`. This is oriented toward document-centric APIs with multiple errors per
response.

**HAL-FORMS** does not define an error convention; errors are left to the
application.

## 3. Proposal

### 3.1 Error Representation Kind: `"error"`

Non-validation error responses SHOULD use `Representation.Kind` set to
`"error"`. This signals to generic clients that the response describes an
error condition rather than a domain resource.

```go
rep := hyper.Representation{
    Kind: "error",
    State: hyper.Object{ ... },
}
```

### 3.2 Standard State Keys

Error representations SHOULD include the following `State` keys:

| Key | Type | Required | Description |
|---|---|---|---|
| `message` | string | Yes | Human-readable description of the error. |
| `code` | string | No | Machine-readable error code (e.g., `"not_found"`, `"token_expired"`). Stable across locales and message changes. |
| `status` | integer | No | HTTP status code. Useful when the representation is consumed outside the original HTTP context (e.g., SSE, message queues). |
| `details` | object/array | No | Additional structured data about the error. Format is application-specific. |

The `message` key is the only required field. A minimal error representation
is:

```go
hyper.Representation{
    Kind: "error",
    State: hyper.Object{
        "message": hyper.Scalar{V: "Contact not found"},
    },
}
```

Servers MAY include additional application-specific keys in `State` alongside
the standard keys.

### 3.3 Validation Errors (422 Unprocessable Entity)

Validation errors SHOULD NOT use `kind: "error"`. Instead, they SHOULD
re-present the action's original representation with `Field.Error` values
populated, preserving submitted values. The HTTP status code 422 distinguishes
this from a successful response.

This is the existing convention and it is affirmed as the standard:

```go
fields := hyper.WithErrors(action.Fields, submittedValues, validationErrors)
rep := hyper.Representation{
    Kind: "contact-form",  // same kind as the original form
    Actions: []hyper.Action{
        {
            Name:   "create-contact",
            Fields: fields,  // Field.Error populated per-field
            // ... other action properties preserved
        },
    },
}
renderer.Respond(w, r, http.StatusUnprocessableEntity, rep)
```

**Rationale:** Returning the same representation kind allows clients that
rendered the original form to re-render it in place with error annotations.
This works naturally for HTML (inline error messages), CLI (field-level error
display), and programmatic clients (retry with corrections).

### 3.4 Non-Validation HTTP Errors

For errors that are not field-level validation failures, use `kind: "error"`
with appropriate State keys:

**404 Not Found:**
```go
hyper.Representation{
    Kind: "error",
    State: hyper.Object{
        "message": hyper.Scalar{V: "Contact not found"},
        "code":    hyper.Scalar{V: "not_found"},
        "status":  hyper.Scalar{V: 404},
    },
}
```

**401 Unauthorized:**
```go
hyper.Representation{
    Kind: "error",
    State: hyper.Object{
        "message": hyper.Scalar{V: "Token expired"},
        "code":    hyper.Scalar{V: "token_expired"},
        "status":  hyper.Scalar{V: 401},
    },
}
```

**403 Forbidden:**
```go
hyper.Representation{
    Kind: "error",
    State: hyper.Object{
        "message": hyper.Scalar{V: "You do not have permission to publish posts"},
        "code":    hyper.Scalar{V: "forbidden"},
        "status":  hyper.Scalar{V: 403},
    },
}
```

**500 Internal Server Error:**
```go
hyper.Representation{
    Kind: "error",
    State: hyper.Object{
        "message": hyper.Scalar{V: "An unexpected error occurred"},
        "code":    hyper.Scalar{V: "internal_error"},
        "status":  hyper.Scalar{V: 500},
    },
}
```

### 3.5 Links and Actions on Error Representations

Error representations MAY include `Links` to provide recovery navigation:

```go
hyper.Representation{
    Kind: "error",
    State: hyper.Object{
        "message": hyper.Scalar{V: "Contact not found"},
        "code":    hyper.Scalar{V: "not_found"},
        "status":  hyper.Scalar{V: 404},
    },
    Links: []hyper.Link{
        {Rel: "collection", Target: hyper.Route("contacts"), Title: "All Contacts"},
        {Rel: "root", Target: hyper.Path(), Title: "Home"},
    },
}
```

Error representations MAY include `Actions` to provide retry or recovery
operations:

```go
hyper.Representation{
    Kind: "error",
    State: hyper.Object{
        "message": hyper.Scalar{V: "Service temporarily unavailable"},
        "code":    hyper.Scalar{V: "service_unavailable"},
        "status":  hyper.Scalar{V: 503},
    },
    Actions: []hyper.Action{
        {
            Name:   "retry",
            Rel:    "retry",
            Method: "POST",
            Target: hyper.Path("/messages"),
            Fields: originalFields,
        },
    },
}
```

### 3.6 Streaming Errors

Errors within SSE streams SHOULD use `kind: "stream-error"` to distinguish
them from request-level errors. The same `State` keys apply:

```go
hyper.Representation{
    Kind: "stream-error",
    State: hyper.Object{
        "message": hyper.Scalar{V: "The model backend is temporarily unavailable"},
        "code":    hyper.Scalar{V: "model_unavailable"},
    },
    Actions: []hyper.Action{
        {Name: "retry", Rel: "retry", Method: "POST", Target: target, Fields: fields},
    },
    Meta: map[string]any{"eventID": "error-1"},
}
```

The `"stream-error"` kind signals that the error occurred mid-stream rather
than at the HTTP request level. Clients receiving a `stream-error` event
SHOULD treat it as a terminal event for the current stream unless a retry
action is provided.

### 3.7 Client Handling

Generic clients SHOULD implement error detection as follows:

1. Check the HTTP status code. Any 4xx or 5xx status indicates an error.
2. If the status is 422, check `Field.Error` values on action fields for
   per-field validation messages.
3. For all other error statuses, check if `Representation.Kind` is `"error"`
   and read `State["message"]` for a human-readable description.
4. If `State["code"]` is present, use it for programmatic error handling
   (e.g., mapping `"token_expired"` to a re-authentication flow).
5. If `Links` are present, offer them as recovery options.
6. If `Actions` are present (e.g., `rel: "retry"`), offer them as recovery
   operations.

## 4. Examples

### 4.1 Validation Error (422)

HTTP response:
```
HTTP/1.1 422 Unprocessable Entity
Content-Type: application/json
```

```json
{
  "kind": "contact-form",
  "actions": [
    {
      "name": "create-contact",
      "method": "POST",
      "href": "/contacts",
      "fields": [
        {
          "name": "name",
          "type": "text",
          "required": true,
          "value": "",
          "error": "Name is required"
        },
        {
          "name": "email",
          "type": "email",
          "required": true,
          "value": "not-an-email",
          "error": "Invalid email address"
        },
        {
          "name": "phone",
          "type": "tel",
          "value": "+1234567890"
        }
      ]
    }
  ]
}
```

### 4.2 Not Found (404)

```
HTTP/1.1 404 Not Found
Content-Type: application/json
```

```json
{
  "kind": "error",
  "state": {
    "message": "Contact not found",
    "code": "not_found",
    "status": 404
  },
  "links": [
    {"rel": "collection", "href": "/contacts", "title": "All Contacts"},
    {"rel": "root", "href": "/", "title": "Home"}
  ]
}
```

### 4.3 Unauthorized (401)

```
HTTP/1.1 401 Unauthorized
Content-Type: application/json
```

```json
{
  "kind": "error",
  "state": {
    "message": "Token expired",
    "code": "token_expired",
    "status": 401
  },
  "links": [
    {"rel": "login", "href": "/login"}
  ]
}
```

### 4.4 Server Error (500)

```
HTTP/1.1 500 Internal Server Error
Content-Type: application/json
```

```json
{
  "kind": "error",
  "state": {
    "message": "An unexpected error occurred",
    "code": "internal_error",
    "status": 500
  },
  "links": [
    {"rel": "root", "href": "/", "title": "Home"}
  ]
}
```

### 4.5 Stream Error (SSE)

```
event: stream-error
id: error-1
data: {"kind":"stream-error","state":{"code":"model_unavailable","message":"The model backend is temporarily unavailable"},"actions":[{"name":"retry","rel":"retry","method":"POST","href":"/agents/a1/conversations/c42/messages","fields":[{"name":"content","type":"textarea","required":true,"value":"Help with Go generics"}]}]}
```

## 5. Alternatives Considered

### 5.1 Adopt RFC 9457 (Problem Details) Directly

RFC 9457 defines `application/problem+json` with fields `type`, `title`,
`status`, `detail`, and `instance`. We could adopt this as-is.

**Pros:**
- Industry standard with broad tooling support.
- Well-specified extension mechanism via additional members.

**Cons:**
- Requires a separate content type (`application/problem+json`), breaking the
  unified content negotiation model where all responses are representations.
- The `type` field (a URI identifying the error type) adds complexity that
  most hyper APIs do not need.
- Does not integrate with hyper's `Links` and `Actions` model — RFC 9457
  errors are terminal documents, not navigable representations.
- Validation errors would need a separate mechanism anyway, since RFC 9457
  has no equivalent of `Field.Error`.

**Decision:** Use RFC 9457 as inspiration for key naming (we adopt `message`
for clarity over `detail`) but keep errors as standard hyper representations.
This preserves the hypermedia model: errors can carry links for recovery and
actions for retry.

### 5.2 Separate Error Type (Not a Representation)

Define a distinct `Error` type outside the `Representation` model.

**Pros:**
- Type-safe distinction between errors and resources at compile time.

**Cons:**
- Doubles the API surface for rendering, content negotiation, and codec
  support.
- Clients need two parsing paths instead of one.
- Loses the ability to embed links and actions in error responses.
- The existing `Representation` type already handles errors well — every
  use case successfully used it for errors.

**Decision:** Errors are representations. The `Kind` field distinguishes them,
not the Go type.

### 5.3 Use `kind: "error"` for Validation Errors Too

Unify all errors under `kind: "error"` and encode field errors in `State`
rather than `Field.Error`.

**Pros:**
- Single error kind to check for.

**Cons:**
- Breaks the re-presentation pattern where clients re-render the original
  form with inline errors.
- Loses the natural mapping between field names and their errors.
- Every use case already uses the `Field.Error` pattern for 422; changing
  this would contradict established practice.

**Decision:** Validation errors (422) continue to use the form's original
`Kind` with `Field.Error`. Non-validation errors use `kind: "error"`.

## 6. Open Questions

1. **Should error representations carry `Links` by default?** The examples
   above include navigation links (e.g., back to collection, home). Should
   the convention mandate at least a `rel: "root"` link, or leave link
   inclusion entirely to the server?

2. **Should retry `Actions` be encouraged?** For transient errors (503, 429),
   including a retry action with the original request fields lets clients
   retry without reconstructing the request. Should this be a SHOULD-level
   recommendation for 5xx and 429 responses?

3. **How do streaming errors interact with connection-level errors?** A
   `stream-error` event is an application-level error within an open SSE
   connection. If the connection itself drops, the client sees a transport
   error, not a `stream-error`. Should the convention define how clients
   distinguish and handle both cases?

4. **Should `code` values be namespaced?** To avoid collisions between
   libraries and applications, should error codes follow a namespacing
   convention (e.g., `"hyper:not_found"` vs `"myapp:quota_exceeded"`)?

5. **Should there be a `kind: "validation-error"` for non-form validation?**
   Some validation errors may not map to form fields (e.g., cross-field
   constraints, business rule violations). Should there be a dedicated kind
   for structured validation errors that are not tied to a specific action's
   fields?

6. **Integration with `jsonapi/` codec.** JSON:API has its own error format
   with an `errors` array. Should the JSON:API codec automatically map
   `kind: "error"` representations to JSON:API error objects, or should this
   be a separate concern?
