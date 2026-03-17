# Proposal 004: Standard Hint Vocabulary

- **Status:** Draft
- **Date:** 2026-03-17
- **Author:** hyper contributors

## 1. Problem Statement

The `Hints` field on `Representation` and `Action` (as well as on `Field`) is
hyper's primary extension point for passing rendering directives and behavioral
metadata to codecs and clients. However, hint keys are currently invented ad hoc
by each application. A CLI tool uses `"confirm"` and `"destructive"`; a blog
platform uses `"sortable"`, `"ui_component"`, and `"variant"`; an htmx-driven
app uses `"hx-target"` and `"hx-swap"`. There is no shared vocabulary.

Without conventions:

- **Generic clients cannot leverage hints.** A CLI navigator, an AI agent, and a
  web codec all encounter `"destructive": true` but have no guarantee it means
  the same thing across applications.
- **Codec authors duplicate effort.** Each codec must document which hint keys it
  recognizes, and application authors must read each codec's documentation to
  know which keys to set.
- **Applications are not portable.** Switching from the HTML codec to a
  hypothetical terminal-UI codec means rewriting all hints, because there is no
  shared semantic layer.

A standard vocabulary—organized into tiers by expected support level—gives hint
keys stable semantics while preserving the open `map[string]any` extensibility
that makes hints useful.

## 2. Background

### 2.1 Library Definitions

`Representation.Hints` and `Action.Hints` are both `map[string]any` (defined in
`hyper.go`). The library itself does not interpret hints; interpretation is
delegated to codecs and clients.

The HTML codec (`html_codec.go`) currently interprets three patterns:

| Hint | Type | Behavior |
|---|---|---|
| `"hidden"` | `bool` | When `true`, the action is not rendered at all |
| `"destructive"` | `bool` | Emits `class="destructive"` on the element |
| Any other key | `string` | Emitted as an HTML attribute (`key="escaped-value"`) |

Boolean hints other than `"destructive"` and `"hidden"` are silently skipped.
Non-string, non-bool values (e.g., `int`, nested maps) are also skipped.

### 2.2 Hint Keys Found Across Use Cases

The following table catalogs every hint key found in use-case exploration
documents, organized by scope and source.

#### Action-Level Hints

| Key | Type | Source | Purpose |
|---|---|---|---|
| `"destructive"` | `bool` | rest-cli, cli-server, contacts-hypermedia, blog-platform | Flags irreversible/dangerous action |
| `"hidden"` | `bool` | rest-cli, blog-platform | Suppresses action from default listings |
| `"confirm"` | `string` | rest-cli, cli-server, contacts-hypermedia | Confirmation message before execution |
| `"async"` | `bool` | rest-cli, agent-streaming | Action returns an async job resource |
| `"stream"` | `bool` | agent-streaming | Action returns a streaming (SSE) response |
| `"auth-required"` | `bool` | rest-cli | Action will fail without authentication |
| `"group"` | `string` | rest-cli | Logical grouping for display |
| `"inline"` | `bool` | blog-platform | Render form inline rather than navigate |
| `"variant"` | `string` | blog-platform | Style variant (e.g., `"danger"`) |
| `"dialog"` | `string` | blog-platform | Render action in a named dialog |
| `"clipboard"` | `bool` | blog-platform | Action copies a value to clipboard |
| `"copy_value"` | `string` | blog-platform | Value to copy when `clipboard` is true |

#### Representation-Level Hints

| Key | Type | Source | Purpose |
|---|---|---|---|
| `"immutable"` | `bool` | rest-cli | Representation is read-only |
| `"sortable"` | `bool` | blog-platform | Collection supports drag-and-drop reorder |
| `"sortable_handle"` | `string` | blog-platform | CSS selector for drag handle |
| `"sortable_group"` | `string` | blog-platform | Group name for multi-list drag |
| `"page_title"` | `string` | blog-platform | Page title for rendered output |

#### Field-Level Hints

| Key | Type | Source | Purpose |
|---|---|---|---|
| `"ui_component"` | `string` | blog-platform | Named UI component for rendering |
| `"accept"` | `string` | blog-platform | File type filter (e.g., `"image/*"`) |
| `"preview"` | `bool` | blog-platform | Show preview of selected value |
| `"hidden"` | `bool` | blog-platform | Suppress field from rendered form |

#### Codec-Specific Hints (htmx)

| Key | Type | Source | Purpose |
|---|---|---|---|
| `"hx-get"` | `string` | contacts-hypermedia, htmlc-codec | GET request target |
| `"hx-post"` | `string` | contacts-hypermedia, blog-platform | POST request target |
| `"hx-put"` | `string` | blog-platform | PUT request target |
| `"hx-delete"` | `string` | blog-platform | DELETE request target |
| `"hx-target"` | `string` | contacts-hypermedia, blog-platform | Element to update |
| `"hx-swap"` | `string` | contacts-hypermedia, blog-platform | Swap strategy |
| `"hx-trigger"` | `string` | contacts-hypermedia | Event trigger expression |
| `"hx-confirm"` | `string` | htmlc-codec | Browser confirm() message |
| `"hx-push-url"` | `string` | contacts-hypermedia | Push URL to browser history |
| `"hx-select"` | `string` | contacts-hypermedia | Select subset of response |
| `"hx-indicator"` | `string` | contacts-hypermedia | Loading indicator selector |
| `"hx-encoding"` | `string` | blog-platform | Encoding for file uploads |

#### Meta Keys (Not Hints, but Related)

| Key | Type | Source | Purpose |
|---|---|---|---|
| `Meta["element-id"]` | `string` | contacts-hypermedia | Stable HTML element ID |
| `Meta["sse-event"]` | `string` | agent-streaming | Override SSE event type |
| `Meta["retry"]` | `int` | agent-streaming | SSE reconnection interval (ms) |
| `Meta["poll-interval"]` | `int` | rest-cli | Polling interval for async jobs |

### 2.3 Observations

1. **Dual-hinting pattern.** Some concepts appear in both generic form
   (`"confirm"`) and codec-specific form (`"hx-confirm"`). The generic form
   carries semantics; the codec-specific form carries rendering instructions.
2. **Meta vs Hints confusion.** Keys like `"element-id"` and `"sse-event"` live
   in `Meta` in some use cases but arguably belong in `Hints` since they direct
   rendering behavior, not application state.
3. **Nested structures.** Some use cases embed nested maps in hints (e.g., lazy
   loading directives). The current HTML codec silently drops these.
4. **No field-level `Hints` in the type system.** `Field` does not have a
   `Hints` field in the library source; field-level hints appear only in
   use-case explorations. This proposal documents the pattern but does not
   propose adding the field (that would be a separate proposal).

## 3. Proposal

Define a three-tier vocabulary for hint keys. Tiers indicate expected support
level, not enforcement—hints remain a `map[string]any` with no compile-time
validation.

### 3.1 Tier 1: Standard Hints

All hyper-aware clients and codecs SHOULD understand these keys. They carry
semantic meaning independent of any specific rendering technology.

#### `"destructive"` (bool) — Action-level

When `true`, the action has irreversible or significant consequences (e.g.,
deleting a resource, canceling a subscription). Clients SHOULD present the
action with appropriate visual warning (e.g., red styling) and MAY require
explicit confirmation even if `"confirm"` is not set.

```go
hyper.Action{
    Name:   "delete",
    Rel:    "delete",
    Method: "DELETE",
    Target: hyper.TargetURI("/contacts/42"),
    Hints:  map[string]any{"destructive": true},
}
```

#### `"hidden"` (bool) — Action-level, Field-level

When `true`, the action or field SHOULD NOT be rendered in default listings or
forms. The action remains available for programmatic invocation (e.g., by an AI
agent that knows to look for it). Useful for administrative actions or
implementation details exposed for automation.

#### `"async"` (bool) — Action-level

When `true`, submitting the action returns a job or task representation rather
than the final result. Clients SHOULD expect to poll or follow a link to get
the completed result.

#### `"auth-required"` (bool) — Action-level

When `true`, the action will fail without prior authentication. Clients MAY
pre-check authentication state and prompt for credentials before submission.

#### `"immutable"` (bool) — Representation-level

When `true`, the representation is read-only—no mutation actions are expected
or meaningful. Clients MAY optimize rendering by omitting edit affordances.

#### `"group"` (string) — Action-level

A logical group name for organizing actions in display. Clients SHOULD group
actions with the same `"group"` value together. The value is a free-form string
(e.g., `"navigation"`, `"admin"`, `"bulk-operations"`).

#### `"confirm"` (string) — Action-level

A human-readable confirmation message to present before executing the action.
Clients SHOULD display this message and require explicit user approval. The
string value is the message text (e.g., `"Are you sure you want to delete this
contact?"`).

```go
hyper.Action{
    Name:   "delete",
    Rel:    "delete",
    Method: "DELETE",
    Target: hyper.TargetURI("/contacts/42"),
    Hints:  map[string]any{
        "destructive": true,
        "confirm":     "Are you sure you want to delete Ada Lovelace?",
    },
}
```

### 3.2 Tier 2: Recommended Hints

Codecs and specialized clients SHOULD support these where applicable. They are
meaningful across multiple rendering contexts but may not apply to all clients.

#### `"element-id"` (string) — Representation-level, Action-level

A stable identifier for the HTML element (or equivalent UI element) representing
this item. Useful for partial page updates, scroll anchoring, and test
automation.

> **Note:** This key currently appears as `Meta["element-id"]` in some use
> cases. This proposal recommends migrating it to `Hints` since it directs
> rendering, not application logic.

#### `"sortable"` (bool) — Representation-level

When `true` on a collection representation, items in the collection support
drag-and-drop reordering. Codecs SHOULD enable reorder affordances.

#### `"sse-event"` (string) — Representation-level

Overrides the SSE event type when this representation is sent as a
server-sent event. By default, the `Kind` field is used as the event type;
this hint allows a different value.

> **Note:** This key currently appears as `Meta["sse-event"]` in the
> agent-streaming use case. This proposal recommends migrating it to `Hints`.

#### `"retry"` (int) — Representation-level

SSE reconnection interval in milliseconds. Clients receiving this hint in an
SSE stream SHOULD use it as the reconnection delay.

#### `"stream"` (bool) — Action-level

When `true`, the action returns a streaming response (e.g., SSE). Often used
together with `"async": true`. Clients SHOULD prepare for an open-ended
response rather than a single representation.

### 3.3 Tier 3: Codec-Specific Hints

Codec-specific hints use a prefix derived from the codec or rendering
technology. They are opaque to clients that do not understand the prefix.

#### Naming Convention

Codec-specific hints SHOULD use `{prefix}-{key}` naming, where `{prefix}`
identifies the codec or technology:

- **`hx-*`** — htmx attributes (e.g., `"hx-target"`, `"hx-swap"`,
  `"hx-trigger"`)
- **`aria-*`** — ARIA accessibility attributes
- **`data-*`** — Custom data attributes for HTML rendering

The HTML codec already supports this pattern: any string-valued hint key is
emitted as an HTML attribute. This means `"hx-target": "#main"` in hints
becomes `hx-target="#main"` in the rendered HTML.

#### Documented Codec-Specific Keys

The following htmx-prefixed keys are in active use across use cases and are
recognized by the HTML codec:

| Key | Purpose |
|---|---|
| `"hx-get"`, `"hx-post"`, `"hx-put"`, `"hx-delete"` | HTTP method + target |
| `"hx-target"` | Element to update with response |
| `"hx-swap"` | How to swap the response into the DOM |
| `"hx-trigger"` | Event that triggers the request |
| `"hx-confirm"` | Browser-native confirmation dialog |
| `"hx-push-url"` | Push URL to browser history |
| `"hx-select"` | Select a subset of the response |
| `"hx-indicator"` | Loading indicator element |
| `"hx-encoding"` | Encoding type for file uploads |

#### Application-Specific Keys

Applications MAY define their own hint keys for domain-specific rendering
directives. These keys SHOULD use a descriptive underscore-separated name to
avoid collision with standard or codec-specific keys:

| Key | Type | Purpose |
|---|---|---|
| `"ui_component"` | `string` | Named UI component for field rendering |
| `"page_title"` | `string` | Page title for the rendered view |
| `"sortable_handle"` | `string` | CSS selector for drag handle |
| `"sortable_group"` | `string` | Group name for multi-list drag |
| `"clipboard"` | `bool` | Action copies a value to clipboard |
| `"copy_value"` | `string` | Value to copy |
| `"inline"` | `bool` | Render action form inline |
| `"variant"` | `string` | Style variant for rendering |
| `"dialog"` | `string` | Render in a named dialog |

These keys are not standardized. Applications that define custom keys SHOULD
document them for their codec and client consumers.

## 4. Examples

### 4.1 Tier 1 Hints on an Action

```json
{
  "name": "delete-contact",
  "rel": "delete",
  "method": "DELETE",
  "target": "/contacts/42",
  "hints": {
    "destructive": true,
    "confirm": "Are you sure you want to delete Ada Lovelace?",
    "group": "admin"
  }
}
```

A CLI client renders this as a red-highlighted command, prompts with the confirm
message, and groups it under an "admin" section. A web codec adds
`class="destructive"` and a confirmation dialog. An AI agent notes the action
is dangerous and asks the user before submitting.

### 4.2 Tier 2 Hints on a Representation

```json
{
  "kind": "contact-list",
  "hints": {
    "sortable": true,
    "element-id": "contact-table"
  },
  "state": { "count": 12 },
  "actions": [
    {
      "name": "reorder",
      "rel": "reorder",
      "method": "PUT",
      "target": "/contacts/order",
      "hints": { "hidden": true }
    }
  ]
}
```

A web codec enables drag-and-drop on the table and assigns `id="contact-table"`
for partial updates. The `"reorder"` action is not displayed but is available
for programmatic drag-and-drop submission.

### 4.3 Mixed Tiers with Codec-Specific Hints

```json
{
  "name": "search",
  "rel": "search",
  "method": "GET",
  "target": "/contacts",
  "hints": {
    "group": "navigation",
    "hx-trigger": "search, keyup delay:200ms changed",
    "hx-target": "#contact-list",
    "hx-swap": "innerHTML",
    "hx-push-url": "true",
    "hx-indicator": "#spinner"
  }
}
```

The `"group"` hint (Tier 1) is understood by all clients. The `hx-*` hints
(Tier 3) are emitted as HTML attributes by the HTML codec and ignored by CLI
clients.

### 4.4 Async Action with Streaming

```json
{
  "name": "generate-report",
  "rel": "create",
  "method": "POST",
  "target": "/reports",
  "hints": {
    "async": true,
    "stream": true,
    "auth-required": true,
    "confirm": "This may take several minutes. Continue?"
  }
}
```

## 5. Alternatives Considered

### 5.1 Flat Vocabulary (No Tiers)

A single flat list of well-known keys without tier classification. This is
simpler but provides no guidance on which keys a codec or client should
prioritize implementing. Tier classification helps codec authors focus on
Tier 1 first and add Tier 2 support incrementally.

### 5.2 Namespaced Keys for All Tiers

Require all hints to use a namespace prefix (e.g., `"hyper-destructive"`,
`"hyper-group"`). This avoids collision with codec-specific keys but adds
verbosity to the most common hints. The proposal instead reserves unprefixed
keys for standard (Tier 1 and Tier 2) vocabulary and requires prefixes only
for codec-specific keys.

### 5.3 Separate HintSet Type

Replace `map[string]any` with a typed `HintSet` struct that has fields for each
standard key and a `Custom map[string]any` for extensions:

```go
type HintSet struct {
    Destructive  bool
    Hidden       bool
    Async        bool
    Group        string
    Confirm      string
    Custom       map[string]any
}
```

This provides compile-time safety but sacrifices the simplicity and openness
of the current design. Adding a new standard key requires a library release.
The current `map[string]any` approach treats the vocabulary as a convention
rather than a schema, which better matches hyper's philosophy of progressive
enhancement.

### 5.4 Hints on Representation vs Meta

Some keys currently in `Meta` (e.g., `"element-id"`, `"sse-event"`) could
remain there. The distinction proposed here is: `Meta` carries
application-level metadata (data about the data), while `Hints` carries
rendering and behavioral directives for clients and codecs. This is a
guideline, not a hard rule.

## 6. Open Questions

### 6.1 Should Hints Be Inheritable?

When a representation embeds child representations, should the children inherit
hints from the parent? For example, if a collection has `"sortable": true`,
should each embedded item automatically be considered sortable?

**Arguments for:** Reduces boilerplate; parent context often implies child
behavior.

**Arguments against:** Implicit inheritance makes it harder to reason about what
hints are active on a given representation; overriding inherited hints adds
complexity.

**Recommendation:** Do not define inheritance semantics in this proposal. If
inheritance is needed, it should be a separate proposal that defines explicit
opt-in mechanisms.

### 6.2 Should There Be a Hint Registry Mechanism?

Should the library provide a programmatic registry where codecs declare which
hint keys they understand? This would enable runtime validation (e.g., warn when
an application sets a hint that no codec will interpret) and introspection.

**Arguments for:** Catches typos; enables tooling; documents codec capabilities.

**Arguments against:** Adds runtime machinery for what is currently a zero-cost
convention; registration ordering and plugin discovery add complexity.

**Recommendation:** Defer to a future proposal. The vocabulary defined here is
sufficient for interoperability without runtime machinery.

### 6.3 Should Field Have a Hints Map?

The `Field` type does not currently have a `Hints` field, yet use-case
explorations use field-level hints for UI component selection and file type
filtering. Should a `Hints map[string]any` field be added to `Field`?

**Recommendation:** This is a library API change and should be proposed
separately. This proposal documents the field-level hint patterns observed
in use cases to inform that future proposal.

### 6.4 Meta-to-Hints Migration

Several keys (`"element-id"`, `"sse-event"`, `"retry"`) currently appear in
`Meta` but are rendering directives. Should these be formally migrated to
`Hints`?

**Recommendation:** Yes, but with a deprecation period. Codecs should check
both `Meta` and `Hints` for these keys during migration, preferring `Hints`
when both are present.
