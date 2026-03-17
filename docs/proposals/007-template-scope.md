# Proposal 007: Template Scope Convention (`hyper.ToScope`)

- **Status:** Draft
- **Date:** 2026-03-17
- **Author:** hyper contributors

## 1. Problem Statement

Every template-based codec that renders hyper representations must convert a
`hyper.Representation` into a flat key/value scope suitable for its template
engine. Today each application writes its own `representationToScope` function.
This duplication causes three concrete problems:

- **No portability across codecs.** A Go `html/template` component that reads
  `scope["actions"]["delete"]["hxAttrs"]` will break if a Vue SFC codec puts
  the same data under a different key. Without a standard scope shape, templates
  cannot be reused across template engines even when they share the same
  hypermedia model.

- **Repeated, non-trivial boilerplate.** The htmlc codec use case
  (`use-cases/htmlc-codec.md` §2.3) required ~120 lines of scope conversion,
  including URL resolution, htmx attribute extraction, recursive embedding, and
  field mapping. The tasklist example (`examples/tasklist/main.go`) reimplemented
  the same logic with minor variations. Every new template codec starts by
  copy-pasting this code.

- **Inconsistent handling of edge cases.** Implementations diverge on: whether
  RichText is passed through or pre-rendered; whether actions with unresolvable
  targets are included or silently dropped; whether Collection state uses an
  `items` key or is ignored entirely; and whether an `actionList` array is
  provided alongside the `actions` map. These inconsistencies surface as subtle
  bugs when switching codecs.

## 2. Background

### 2.1 Survey of Existing Implementations

Three independent implementations of `representationToScope` exist in the
codebase:

**htmlc-codec use case** (`use-cases/htmlc-codec.md` §2.3) — the most complete
version. Signature: `func representationToScope(ctx context.Context, rep
hyper.Representation, opts hyper.EncodeOptions) map[string]any`. Flattens
Object state to top-level keys, resolves URLs via `opts.Resolver`, extracts
`hx-*` hints into `hxAttrs` maps, builds both `actions` (map by rel) and
`actionList` (ordered slice), recurses into `embedded`, and passes through
`meta` and `hints`.

**tasklist example** (`examples/tasklist/main.go`, lines 411–521) — a working
application implementation. Adds `renderDocument` and `rootHxSwapOob`
app-specific keys. Keys actions by `Name` with `Rel` override (diverges from
htmlc which keys by `Rel`). Uses an eager `resolveHref` helper that falls back
to `Target.URL` when the Resolver is absent.

**spec.md §16.5** — the canonical reference. Does *not* resolve targets within
the function; instead states that the codec is responsible for injecting
resolved URLs before rendering. This creates a lazy-vs-eager design tension
with the other implementations.

### 2.2 Spec Feedback

The htmlc-codec use case (§8) explicitly recommends standardization:

> `representationToScope` should be a library function. Every application that
> uses a template engine with hyper must write its own scope conversion. Consider
> providing `hyper.ToScope(ctx, rep, opts) map[string]any`.

Additional issues raised in §8:

- **Nested embedded scoping** — deeply nested representations produce verbose
  template expressions (e.g., `embedded.items[0].embedded.details[0].name`).
- **RichText in scope** — should it be pre-rendered to HTML or passed through?
- **`Action.Target` resolution timing** — what happens when resolution fails?
- **Collection state** — only Object is handled; Collection has no standard key.
- **`actionList` for enumeration** — both map and list forms should be documented.

### 2.3 Resolver Interface

The `Resolver` interface (`hyper.go`, lines 116–119) converts abstract `Target`
values to concrete URLs:

```go
type Resolver interface {
    ResolveTarget(context.Context, Target) (*url.URL, error)
}
```

`EncodeOptions` (`hyper.go`, lines 129–134) carries the Resolver alongside the
current request and render mode:

```go
type EncodeOptions struct {
    Request  *http.Request
    Resolver Resolver
    Mode     RenderMode
}
```

## 3. Proposal

Add a library function with the following signature:

```go
package hyper

// ToScope converts a Representation into a flat map suitable for template
// engines. It resolves targets using opts.Resolver when available.
func ToScope(ctx context.Context, rep Representation, opts EncodeOptions) map[string]any
```

### 3.1 Standard Scope Shape

The returned map has the following keys:

| Key | Type | Source | Always present |
|-----|------|--------|----------------|
| `kind` | `string` | `rep.Kind` | Yes (empty string if unset) |
| `self` | `string` | Resolved `rep.Self` URL | No — omitted when `Self` is nil or resolution fails |
| `meta` | `map[string]any` | `rep.Meta` | No — omitted when empty |
| `hints` | `map[string]any` | `rep.Hints` | No — omitted when empty |
| *(state keys)* | `any` | Flattened from `Object` | No — only when `State` is `Object` |
| `items` | `[]any` | Unwrapped from `Collection` | No — only when `State` is `Collection` |
| `links` | `map[string]map[string]any` | Keyed by `Rel` | No — omitted when empty |
| `linkList` | `[]map[string]any` | Declaration order | No — omitted when empty |
| `actions` | `map[string]map[string]any` | Keyed by `Rel` | No — omitted when empty |
| `actionList` | `[]map[string]any` | Declaration order | No — omitted when empty |
| `embedded` | `map[string][]map[string]any` | Keyed by slot | No — omitted when empty |

### 3.2 State Flattening

**Object state.** Each key in the `Object` map is promoted to a top-level scope
key. Values are converted as follows:

- `hyper.Scalar` → the underlying `Scalar.V` value (string, number, bool, nil).
- `hyper.RichText` → a map with three keys:
  ```go
  map[string]any{
      "mediaType": val.MediaType,
      "source":    val.Source,
      "html":      val.Source, // only when MediaType is "text/html"
  }
  ```
  The `html` key contains pre-rendered HTML only when `MediaType` is
  `"text/html"`. For other media types, `html` is omitted and the template
  is responsible for rendering.

**Collection state.** The `Collection` slice is unwrapped into `scope["items"]`
as a `[]any` where each element is the underlying value (Scalar.V, RichText
map, or recursively-converted Object).

**Conflict resolution.** If a state key collides with a reserved scope key
(`kind`, `self`, `meta`, `hints`, `items`, `links`, `linkList`, `actions`,
`actionList`, `embedded`), the state key is prefixed with `state_` (e.g.,
`state_kind`). In practice, applications should avoid naming state fields after
reserved keys.

### 3.3 Links

Each link produces a map:

```go
map[string]any{
    "rel":   l.Rel,
    "href":  "<resolved URL>",  // omitted on resolution failure
    "title": l.Title,           // omitted when empty
    "type":  l.Type,            // omitted when empty
}
```

Links are stored in two forms:
- `scope["links"]` — map keyed by `Rel` for named access (e.g.,
  `links.next.href`).
- `scope["linkList"]` — slice preserving declaration order for iteration.

When multiple links share the same `Rel`, the *last* one wins in the map, but
all appear in `linkList`.

### 3.4 Actions

Each action produces a map:

```go
map[string]any{
    "name":     a.Name,
    "rel":      a.Rel,
    "method":   a.Method,
    "href":     "<resolved URL>",    // omitted on resolution failure
    "fields":   fieldsToScope(a.Fields),
    "hints":    a.Hints,             // omitted when empty
    "hxAttrs":  hxAttrsFromAction(a, resolvedURL),
    "consumes": a.Consumes,          // omitted when empty
    "produces": a.Produces,          // omitted when empty
}
```

Actions are keyed by `Rel` in the `actions` map. The `actionList` slice
preserves declaration order.

**Target resolution failure.** When `opts.Resolver` is non-nil but
`ResolveTarget` returns an error for an action, that action is **omitted from
the scope entirely**. This ensures templates never render a form or button
without a working URL. If the Resolver is nil, all actions are included without
`href` — the template is responsible for handling the absence.

#### 3.4.1 htmx Attribute Extraction

The `hxAttrs` map is built by:

1. Copying all entries from `Action.Hints` whose key starts with `hx-`.
2. Injecting `hx-{method}` (lowercased) with the resolved URL as value (e.g.,
   `"hx-post": "/contacts"`).

This allows templates to spread attributes directly:

```html
<button {{spreadAttrs .actions.delete.hxAttrs}}>Delete</button>
```

#### 3.4.2 Fields

Each `hyper.Field` is converted to:

```go
map[string]any{
    "name":     f.Name,
    "type":     f.Type,
    "required": f.Required,
    "readOnly": f.ReadOnly,    // omitted when false
    "value":    f.Value,       // omitted when nil
    "label":    f.Label,       // omitted when empty
    "help":     f.Help,        // omitted when empty
    "error":    f.Error,       // omitted when empty
    "options":  []map[string]any{  // omitted when empty
        {"value": o.Value, "label": o.Label, "selected": o.Selected},
    },
}
```

### 3.5 Embedded Representations

Each slot in `rep.Embedded` maps to `scope["embedded"][slot]`, which is a
`[]map[string]any` of recursively-scoped representations (i.e., each embedded
representation is passed through `ToScope` with the same `ctx` and `opts`).

### 3.6 Options

`ToScope` uses the existing `EncodeOptions` struct. No new option types are
introduced.

Future extensions may add an optional `ToScopeOptions` parameter (via
variadic argument or options struct) for:

- `RichTextRenderer` — a callback for pre-rendering non-HTML RichText.
- `IncludeUnresolved` — include actions/links even when resolution fails.

These are deferred to avoid premature API surface.

## 4. Examples

### 4.1 Go Code Using `hyper.ToScope`

```go
func (c *myCodec) Encode(w io.Writer, rep hyper.Representation, opts hyper.EncodeOptions) error {
    scope := hyper.ToScope(ctx, rep, opts)
    return c.template.Execute(w, scope)
}
```

### 4.2 Resulting Scope Map

Given a representation:

```go
rep := hyper.Representation{
    Kind: "Contact",
    Self: &hyper.Target{Route: hyper.RouteRef{Name: "contact", Params: map[string]string{"id": "42"}}},
    State: hyper.Object{
        "name":  hyper.Scalar{V: "Alice"},
        "email": hyper.Scalar{V: "alice@example.com"},
    },
    Links: []hyper.Link{
        {Rel: "collection", Target: hyper.Target{Route: hyper.RouteRef{Name: "contacts"}}, Title: "All Contacts"},
    },
    Actions: []hyper.Action{
        {
            Name: "Delete Contact", Rel: "delete", Method: "DELETE",
            Target: hyper.Target{Route: hyper.RouteRef{Name: "contact", Params: map[string]string{"id": "42"}}},
            Hints: map[string]any{"hx-confirm": "Are you sure?", "hx-target": "closest tr", "hx-swap": "outerHTML"},
        },
    },
}
```

`hyper.ToScope(ctx, rep, opts)` returns:

```go
map[string]any{
    "kind":  "Contact",
    "self":  "/contacts/42",
    "name":  "Alice",
    "email": "alice@example.com",
    "links": map[string]map[string]any{
        "collection": {"rel": "collection", "href": "/contacts", "title": "All Contacts"},
    },
    "linkList": []map[string]any{
        {"rel": "collection", "href": "/contacts", "title": "All Contacts"},
    },
    "actions": map[string]map[string]any{
        "delete": {
            "name": "Delete Contact", "rel": "delete", "method": "DELETE",
            "href": "/contacts/42",
            "hints": map[string]any{"hx-confirm": "Are you sure?", "hx-target": "closest tr", "hx-swap": "outerHTML"},
            "hxAttrs": map[string]any{
                "hx-delete":  "/contacts/42",
                "hx-confirm": "Are you sure?",
                "hx-target":  "closest tr",
                "hx-swap":    "outerHTML",
            },
        },
    },
    "actionList": []map[string]any{
        { /* same as actions["delete"] */ },
    },
}
```

### 4.3 Template Consuming the Scope

**Go `html/template`:**

```html
<article>
  <h1>{{.name}}</h1>
  <p>{{.email}}</p>

  {{with index .actions "delete"}}
  <button {{spreadAttrs .hxAttrs}}>{{.name}}</button>
  {{end}}

  {{with index .links "collection"}}
  <a href="{{.href}}">{{.title}}</a>
  {{end}}
</article>
```

**Vue SFC (data-driven attributes):**

```vue
<template>
  <article>
    <h1>{{ name }}</h1>
    <p>{{ email }}</p>
    <button v-bind="actions.delete.hxAttrs">{{ actions.delete.name }}</button>
    <a :href="links.collection.href">{{ links.collection.title }}</a>
  </article>
</template>
```

## 5. Alternatives Considered

### 5.1 Documented Convention Only (No Library Function)

Instead of providing `hyper.ToScope`, document the scope shape and let each
codec implement it.

**Rejected** because the implementation is non-trivial (~120 lines with URL
resolution, recursive embedding, field conversion, and htmx extraction).
Documentation alone does not prevent divergence; the tasklist example already
diverges from the htmlc use case in action keying strategy. A library function
is the only way to enforce consistency.

### 5.2 Namespace State Under a `state` Key

Instead of flattening Object keys to the top level, put them under
`scope["state"]["name"]`.

**Rejected** because it makes the common case (accessing state) more verbose in
templates (`{{.state.name}}` vs `{{.name}}`). Every surveyed implementation
flattens state. The conflict-resolution rule (prefix with `state_`) handles the
rare case where a state key collides with a reserved scope key.

### 5.3 Lazy Resolution (Codec Injects URLs Later)

The spec.md §16.5 convention has `representationToScope` return the scope
without resolving targets, leaving URL injection to the codec.

**Rejected as default** because it adds a mandatory post-processing step that
every codec must remember to perform. Both real implementations (htmlc, tasklist)
resolve eagerly. The proposed `ToScope` resolves eagerly when a Resolver is
available and gracefully degrades when it is not.

## 6. Open Questions

1. **Should `ToScope` accept a `RichTextRenderer` interface?** A callback like
   `func(RichText) (string, error)` would let codecs pre-render Markdown or
   other formats to HTML before template execution. This is useful but adds API
   surface. The current proposal defers this to a future extension.

2. **Should single-slot embedding be auto-flattened?** When `rep.Embedded` has
   exactly one slot, should its contents be promoted directly to `scope` instead
   of nesting under `scope["embedded"]["slotName"]`? This reduces verbosity but
   creates an inconsistent scope shape that depends on the number of slots.
   The current proposal always nests under `embedded` for predictability.

3. **Should `linkList` and `actionList` include unresolvable entries?** The
   current proposal omits actions with resolution failures entirely. An
   alternative is to include them without `href` and let the template decide.
   The `IncludeUnresolved` option is reserved for this purpose.

4. **Should app-specific keys be supported?** The tasklist example adds
   `renderDocument` and `rootHxSwapOob`. Should `ToScope` support an extension
   mechanism (e.g., `func(scope map[string]any, rep Representation)` callback)?
   The current proposal does not include this — applications can modify the
   returned map after calling `ToScope`.
