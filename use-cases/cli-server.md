# Use Case: Hypermedia-Driven REST CLI — Server Side

This document explores the **server side** of a REST and hypermedia-compatible CLI. It is a companion to `use-cases/rest-cli.md` (the client perspective) and dreams up the ideal server implementation using the hypothetical `hyper` package. The central question: how few assumptions does a server need to make any HTTP API CLI-compatible?

## 1. Overview

The client-side document (`rest-cli.md`) showed a CLI that discovers all commands at runtime from `Links`, `Actions`, and `Fields`. That CLI works against *any* server that produces valid JSON per the wire format (§13.3). This document examines the server perspective:

- What does a server need to do to be CLI-compatible?
- How much of that work is CLI-specific vs. generic hypermedia?
- Can a server in *any* language be compatible without importing `hyper`?

Key findings:

- **The JSON wire format (§13.3) is the complete compatibility contract.** A server needs no `hyper` import — producing valid JSON per the wire format is sufficient.
- **Zero CLI-specific code is needed.** Every server responsibility (representations, actions, fields, hints, validation errors) serves all clients equally — CLI, browser, mobile, scripts.
- **Content negotiation eliminates client-specific logic.** A single `renderer.Respond` call serves all clients.
- **The server is the single source of truth.** Adding or removing features requires only server changes. The CLI discovers new actions and links automatically.

## 2. Minimal Server Contract

### 2.1 What the Wire Format Requires

A CLI-compatible server must produce JSON documents that conform to the wire format (§13.3). The requirements are minimal:

1. **Top-level object** with optional keys: `kind`, `self`, `state`, `links`, `actions`, `embedded`, `meta`
2. **Links** are objects with `rel`, `href`, and optional `title`
3. **Actions** are objects with `name`, `rel`, `method`, `href`, and optional `fields`, `consumes`, `produces`, `hints`
4. **Fields** are objects with `name`, `type`, and optional `label`, `help`, `value`, `required`, `options`, `error`
5. **Embedded** maps slot names to arrays of representations
6. **Content-Type** response header: `application/vnd.api+json`

That is the entire contract. No SDK import. No code generation. No special middleware.

### 2.2 What Is Explicitly Not Required

- No `hyper` package import — the server can emit raw JSON
- No special HTTP headers beyond `Content-Type`
- No server-side awareness that a CLI client exists
- No client-type sniffing or user-agent parsing
- No separate "API mode" vs. "UI mode" — the same representation serves both

## 3. Server Implementation with `hyper` (Go)

The `hyper` package provides the most ergonomic path for Go servers. The package types map directly to the wire format, and the `Renderer` (§10) handles content negotiation and encoding.

### 3.1 Handler: Root Representation

```go
func handleRoot(w http.ResponseWriter, r *http.Request) {
    root := hyper.Representation{
        Kind: "root",
        Self: hyper.Path().Ptr(),
        Links: []hyper.Link{
            {Rel: "contacts", Target: hyper.MustParseTarget("/contacts"), Title: "Contacts"},
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

    renderer.Respond(w, r, http.StatusOK, root)
}
```

The `renderer.Respond` call inspects the request's `Accept` header and selects the appropriate codec (§9). If the client sends `Accept: application/vnd.api+json`, the JSON codec encodes the representation. If a browser sends `Accept: text/html`, an HTML codec renders the same representation as a web page. The handler does not know or care which client is connecting.

### 3.2 Domain Layer and Representation Functions

Before looking at the remaining handlers, it is worth establishing a pattern that keeps them thin. The handlers in §3.1 (root) are simple enough to inline, but handlers that deal with domain logic (validation, persistence) and hypermedia construction benefit from separating those concerns into distinct functions.

**Domain layer** — plain Go functions with no `hyper` imports. These encapsulate validation rules and business logic:

```go
// Domain types and validation — no hyper imports

type ContactInput struct {
    Name  string `json:"name"`
    Email string `json:"email"`
    Phone string `json:"phone"`
}

type ValidationErrors map[string]string

func validateContactInput(input ContactInput) ValidationErrors {
    errs := ValidationErrors{}
    if input.Name == "" {
        errs["name"] = "Name is required"
    }
    if input.Email == "" {
        errs["email"] = "Email is required"
    } else if !isValidEmail(input.Email) {
        errs["email"] = "Invalid email address"
    }
    return errs
}
```

**Representation layer** — functions that map domain objects to `hyper.Representation` values. These are mechanical mappings with no business logic:

```go
// Representation functions — map domain objects to hyper types

func contactRepresentation(c Contact) hyper.Representation {
    return hyper.Representation{
        Kind: "contact",
        Self: hyper.Pathf("/contacts/%d", c.ID).Ptr(),
        State: hyper.Object{
            "id":    hyper.Scalar{V: c.ID},
            "name":  hyper.Scalar{V: c.Name},
            "email": hyper.Scalar{V: c.Email},
            "phone": hyper.Scalar{V: c.Phone},
        },
        Links: []hyper.Link{
            {Rel: "contacts", Target: hyper.MustParseTarget("/contacts"), Title: "All Contacts"},
        },
    }
}

func contactSummaryRepresentation(c Contact) hyper.Representation {
    return hyper.Representation{
        Kind: "contact",
        Self: hyper.Pathf("/contacts/%d", c.ID).Ptr(),
        State: hyper.Object{
            "id":    hyper.Scalar{V: c.ID},
            "name":  hyper.Scalar{V: c.Name},
            "email": hyper.Scalar{V: c.Email},
        },
        Links: []hyper.Link{
            {Rel: "self", Target: hyper.Pathf("/contacts/%d", c.ID), Title: c.Name},
        },
    }
}

func createContactFormRepresentation(input ContactInput, fieldErrors ValidationErrors) hyper.Representation {
    fields := []hyper.Field{
        {Name: "name", Type: "text", Label: "Name", Value: input.Name, Required: true},
        {Name: "email", Type: "email", Label: "Email", Value: input.Email, Required: true},
        {Name: "phone", Type: "tel", Label: "Phone", Value: input.Phone},
    }

    for i, f := range fields {
        if errMsg, ok := fieldErrors[f.Name]; ok {
            fields[i].Error = errMsg
        }
    }

    return hyper.Representation{
        Kind: "contact-form",
        Self: hyper.MustParseTarget("/contacts").Ptr(),
        Actions: []hyper.Action{
            {
                Name:     "Create Contact",
                Rel:      "create",
                Method:   "POST",
                Target:   hyper.MustParseTarget("/contacts"),
                Consumes: []string{"application/vnd.api+json"},
                Fields:   fields,
            },
        },
    }
}
```

This separation has three benefits:

1. **Representation functions are reusable.** The same `contactRepresentation` function serves the single-contact GET handler (§3.4), the create handler's success path (§3.5), and the update handler (§11.2). If the contact wire format changes, there is one place to update.
2. **Validation is a domain concern that produces data, not hypermedia.** `validateContactInput` returns a plain `map[string]string` — the representation layer maps those errors into `Field.Error` values. This makes validation testable without importing `hyper`.
3. **Handlers become thin orchestrators**: decode, validate, persist, represent, respond. Each step is a single function call.

### 3.3 Handler: Contact List

```go
func handleContactList(w http.ResponseWriter, r *http.Request) {
    contacts, err := store.ListContacts(r.Context())
    if err != nil {
        renderError(w, r, err)
        return
    }

    items := make([]hyper.Representation, len(contacts))
    for i, c := range contacts {
        items[i] = contactSummaryRepresentation(c)
    }

    list := hyper.Representation{
        Kind: "contact-list",
        Self: hyper.MustParseTarget("/contacts").Ptr(),
        Links: []hyper.Link{
            {Rel: "root", Target: hyper.Path(), Title: "Home"},
        },
        Actions: []hyper.Action{
            {
                Name:     "Create Contact",
                Rel:      "create",
                Method:   "POST",
                Target:   hyper.MustParseTarget("/contacts"),
                Consumes: []string{"application/vnd.api+json"},
                Fields: []hyper.Field{
                    {Name: "name", Type: "text", Label: "Name", Required: true},
                    {Name: "email", Type: "email", Label: "Email", Required: true},
                    {Name: "phone", Type: "tel", Label: "Phone"},
                },
            },
        },
        Embedded: map[string][]hyper.Representation{
            "items": items,
        },
    }

    renderer.Respond(w, r, http.StatusOK, list)
}
```

The loop body is now a single call to `contactSummaryRepresentation`. There is nothing CLI-specific here. The `Fields` on the `create` action describe what the server accepts — the CLI reads `Label`, `Required`, and `Type` to build its flags, but a browser reads the same metadata to render a form. The server is the single source of truth for what inputs are needed and what constraints apply.

### 3.4 Handler: Single Contact

The single-contact handler uses `contactRepresentation` for the base representation and adds handler-specific links and actions:

```go
func handleContact(w http.ResponseWriter, r *http.Request) {
    id := extractID(r)
    c, err := store.GetContact(r.Context(), id)
    if err != nil {
        renderError(w, r, err)
        return
    }

    rep := contactRepresentation(c)

    // Add links and actions specific to the detail view
    rep.Links = append(rep.Links,
        hyper.Link{Rel: "notes", Target: hyper.Pathf("/contacts/%d/notes", c.ID), Title: "Notes"},
    )
    rep.Actions = []hyper.Action{
        {
            Name:     "Update Contact",
            Rel:      "update",
            Method:   "PUT",
            Target:   hyper.Pathf("/contacts/%d", c.ID),
            Consumes: []string{"application/vnd.api+json"},
            Fields: []hyper.Field{
                {Name: "name", Type: "text", Label: "Name", Value: c.Name, Required: true},
                {Name: "email", Type: "email", Label: "Email", Value: c.Email, Required: true},
                {Name: "phone", Type: "tel", Label: "Phone", Value: c.Phone},
            },
        },
        {
            Name:   "Delete Contact",
            Rel:    "delete",
            Method: "DELETE",
            Target: hyper.Pathf("/contacts/%d", c.ID),
            Hints: map[string]any{
                "confirm":     fmt.Sprintf("Are you sure you want to delete %s?", c.Name),
                "destructive": true,
            },
        },
    }

    renderer.Respond(w, r, http.StatusOK, rep)
}
```

The handler starts from `contactRepresentation(c)` — the same function that the create handler uses for its success response — and extends it with detail-view-specific affordances. The `update` action pre-populates `Field.Value` with the current values. This is pure server logic — the CLI uses these as flag defaults, a browser pre-fills form inputs, and a mobile app pre-populates text fields. The `delete` action carries `Hints` (§15.6) that the CLI renders as a confirmation prompt with warning styling. The server does not know how the hints will be rendered — it only declares semantics.

### 3.5 Handler: Create Contact

With the domain and representation layers from §3.2, the create handler becomes a thin orchestrator — decode, validate, persist, represent, respond:

```go
func handleCreateContact(w http.ResponseWriter, r *http.Request) {
    var input ContactInput
    if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
        renderError(w, r, err)
        return
    }

    if errs := validateContactInput(input); len(errs) > 0 {
        renderer.Respond(w, r, http.StatusUnprocessableEntity,
            createContactFormRepresentation(input, errs))
        return
    }

    c, err := store.CreateContact(r.Context(), input.Name, input.Email, input.Phone)
    if err != nil {
        renderError(w, r, err)
        return
    }

    renderer.Respond(w, r, http.StatusCreated, contactRepresentation(c))
}
```

Compare this to the original version where validation logic, field metadata, and representation construction were interleaved in a single function. The refactored handler contains no business rules and no `hyper` type construction — it delegates both concerns to the appropriate layer. Each line maps to exactly one step: decode the request, validate the input, persist the contact, respond with a representation.

This matters because it makes the example honest about what hypermedia construction actually involves. Building a `hyper.Representation` is a mechanical mapping from domain data to wire types — it does not need to live alongside validation rules or persistence calls. A reader looking at this example for guidance will see that `contactRepresentation` and `createContactFormRepresentation` are thin, testable functions — not tangled handler code.

### 3.6 Validation Errors

When validation fails, the handler calls `createContactFormRepresentation` (defined in §3.2) to build the error response. The representation layer maps `ValidationErrors` into `Field.Error` values — the handler does not need to know about field metadata or error attachment logic:

```go
// The handler's validation error path (from §3.5) is just:
renderer.Respond(w, r, http.StatusUnprocessableEntity,
    createContactFormRepresentation(input, errs))
```

For reference, `createContactFormRepresentation` builds the same representation that the original `renderValidationError` produced — fields with submitted values preserved and errors attached — but as a pure function that takes domain data in and returns a `hyper.Representation` out.

On the wire (§13.3):

```json
{
  "kind": "contact-form",
  "self": {"href": "/contacts"},
  "actions": [
    {
      "name": "Create Contact",
      "rel": "create",
      "method": "POST",
      "href": "/contacts",
      "consumes": ["application/vnd.api+json"],
      "fields": [
        {"name": "name", "type": "text", "label": "Name", "value": "", "required": true, "error": "Name is required"},
        {"name": "email", "type": "email", "label": "Email", "value": "not-an-email", "required": true, "error": "Invalid email address"},
        {"name": "phone", "type": "tel", "label": "Phone", "value": ""}
      ]
    }
  ]
}
```

The CLI maps `Field.Error` back to flags and displays them. A browser re-renders the form with inline error messages. The same response body drives both experiences — the server does not need to know which client is consuming it.

## 4. Server Implementation Without `hyper` (Go)

A server can be CLI-compatible by producing raw JSON that conforms to the wire format. No `hyper` import is required.

### 4.1 Raw JSON Handler

```go
func handleRoot(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/vnd.api+json")
    w.WriteHeader(http.StatusOK)

    json.NewEncoder(w).Encode(map[string]any{
        "kind": "root",
        "self": map[string]any{"href": "/"},
        "links": []map[string]any{
            {"rel": "contacts", "href": "/contacts", "title": "Contacts"},
        },
        "actions": []map[string]any{
            {
                "name":   "Search",
                "rel":    "search",
                "method": "GET",
                "href":   "/search",
                "fields": []map[string]any{
                    {"name": "q", "type": "text", "label": "Query"},
                },
            },
        },
    })
}
```

This handler produces byte-identical JSON to the `hyper`-based handler in Section 3.1. The CLI cannot distinguish between the two. The `hyper` package adds ergonomics (type safety, `Renderer` content negotiation, codec pluggability) but is not required for compatibility.

### 4.2 Trade-offs

| Concern | With `hyper` | Without `hyper` |
|---|---|---|
| Type safety | Compile-time checked | Runtime `map[string]any` |
| Content negotiation | `renderer.Respond` handles `Accept` | Manual header inspection |
| Multiple formats | Register codecs for JSON, HTML, Markdown | Write separate encoding per format |
| Wire format compliance | Guaranteed by types | Developer responsibility |
| `Target` / `RouteRef` resolution | `Resolver` interface (§8.1) | Manual URL construction |

The `hyper` package eliminates an entire class of bugs (malformed representations, missing required fields, incorrect key names) while also enabling multi-format support. But the *compatibility contract* is the wire format, not the package.

## 5. Third-Party Server (TypeScript)

Any language can produce CLI-compatible responses. Here is a TypeScript server that the same CLI can navigate:

### 5.1 Root Handler

```typescript
import express from "express";

const app = express();
app.use(express.json());

app.get("/", (req, res) => {
  res.type("application/vnd.api+json").json({
    kind: "root",
    self: { href: "/" },
    links: [
      { rel: "contacts", href: "/contacts", title: "Contacts" },
    ],
    actions: [
      {
        name: "Search",
        rel: "search",
        method: "GET",
        href: "/search",
        fields: [
          { name: "q", type: "text", label: "Query" },
        ],
      },
    ],
  });
});
```

### 5.2 Collection Handler

```typescript
app.get("/contacts", async (req, res) => {
  const contacts = await db.listContacts();

  res.type("application/vnd.api+json").json({
    kind: "contact-list",
    self: { href: "/contacts" },
    links: [
      { rel: "root", href: "/", title: "Home" },
    ],
    actions: [
      {
        name: "Create Contact",
        rel: "create",
        method: "POST",
        href: "/contacts",
        consumes: ["application/vnd.api+json"],
        fields: [
          { name: "name", type: "text", label: "Name", required: true },
          { name: "email", type: "email", label: "Email", required: true },
          { name: "phone", type: "tel", label: "Phone" },
        ],
      },
    ],
    embedded: {
      items: contacts.map((c) => ({
        kind: "contact",
        self: { href: `/contacts/${c.id}` },
        state: { id: c.id, name: c.name, email: c.email },
        links: [
          { rel: "self", href: `/contacts/${c.id}`, title: c.name },
        ],
      })),
    },
  });
});
```

### 5.3 Validation Error Handler

```typescript
app.post("/contacts", async (req, res) => {
  const { name, email, phone } = req.body;
  const errors: Record<string, string> = {};

  if (!name) errors.name = "Name is required";
  if (!email) errors.email = "Email is required";
  else if (!isValidEmail(email)) errors.email = "Invalid email address";

  if (Object.keys(errors).length > 0) {
    res.status(422).type("application/vnd.api+json").json({
      kind: "contact-form",
      self: { href: "/contacts" },
      actions: [
        {
          name: "Create Contact",
          rel: "create",
          method: "POST",
          href: "/contacts",
          consumes: ["application/vnd.api+json"],
          fields: [
            { name: "name", type: "text", label: "Name", value: name ?? "", required: true, error: errors.name },
            { name: "email", type: "email", label: "Email", value: email ?? "", required: true, error: errors.email },
            { name: "phone", type: "tel", label: "Phone", value: phone ?? "" },
          ],
        },
      ],
    });
    return;
  }

  const contact = await db.createContact({ name, email, phone });

  res.status(201).type("application/vnd.api+json").json({
    kind: "contact",
    self: { href: `/contacts/${contact.id}` },
    state: { id: contact.id, name: contact.name, email: contact.email, phone: contact.phone },
    links: [
      { rel: "contacts", href: "/contacts", title: "All Contacts" },
    ],
  });
});
```

This TypeScript server is fully navigable by the CLI described in `rest-cli.md`. No shared code, no shared types, no `hyper` package — just conformance to the JSON wire format.

## 6. Content Negotiation

### 6.1 How `renderer.Respond` Works

The `Renderer` (§10) inspects the `Accept` header and selects a registered codec:

```go
renderer := hyper.NewRenderer(
    hyper.WithCodec("application/vnd.api+json", jsonCodec),
    hyper.WithCodec("text/html", htmlCodec),
    hyper.WithCodec("text/markdown", markdownCodec),
)
```

A single `renderer.Respond(w, r, status, rep)` call handles all clients:

- CLI sends `Accept: application/vnd.api+json` — gets JSON with full hypermedia controls
- Browser sends `Accept: text/html` — gets an HTML page with forms and links
- Script sends `Accept: application/json` — gets plain JSON (if a plain JSON codec is registered)
- Docs tool sends `Accept: text/markdown` — gets a Markdown rendering

### 6.2 Adding a New Format

Adding HTML support to a JSON-only server is a one-line codec registration change:

```go
// Before: JSON only
renderer := hyper.NewRenderer(
    hyper.WithCodec("application/vnd.api+json", jsonCodec),
)

// After: JSON + HTML
renderer := hyper.NewRenderer(
    hyper.WithCodec("application/vnd.api+json", jsonCodec),
    hyper.WithCodec("text/html", htmlCodec),
)
```

No handler changes. No new routes. No conditional logic. The `Representation` is the same — only the encoding changes.

### 6.3 No Client-Specific Logic

The server never branches on client type. There is no:

```go
// This never happens in a hyper server
if isCLI(r) {
    writeJSON(w, rep)
} else if isBrowser(r) {
    writeHTML(w, rep)
}
```

Content negotiation via the `Accept` header and codec registry replaces all client-type sniffing. The server builds one representation and the renderer encodes it appropriately.

## 7. Server as Single Source of Truth

### 7.1 Adding a Feature

When the server adds a new feature, CLI clients discover it automatically. No client update is needed.

**Example: Adding a "tags" sub-resource to contacts.**

Server change — add a link to the contact representation:

```go
// Add one link to the existing contact handler
Links: []hyper.Link{
    {Rel: "contacts", Target: hyper.MustParseTarget("/contacts"), Title: "All Contacts"},
    {Rel: "notes", Target: hyper.Pathf("/contacts/%d/notes", c.ID), Title: "Notes"},
    // New: tags link
    {Rel: "tags", Target: hyper.Pathf("/contacts/%d/tags", c.ID), Title: "Tags"},
},
```

Add a handler for the new resource:

```go
func handleContactTags(w http.ResponseWriter, r *http.Request) {
    id := extractID(r)
    tags, _ := store.GetContactTags(r.Context(), id)

    items := make([]hyper.Representation, len(tags))
    for i, t := range tags {
        items[i] = hyper.Representation{
            Kind: "tag",
            Self: hyper.Pathf("/contacts/%d/tags/%s", id, t.Slug).Ptr(),
            State: hyper.Object{
                "name":  hyper.Scalar{V: t.Name},
                "color": hyper.Scalar{V: t.Color},
            },
        }
    }

    rep := hyper.Representation{
        Kind: "tag-list",
        Self: hyper.Pathf("/contacts/%d/tags", id).Ptr(),
        Links: []hyper.Link{
            {Rel: "contact", Target: hyper.Pathf("/contacts/%d", id), Title: "Back to Contact"},
        },
        Actions: []hyper.Action{
            {
                Name:     "Add Tag",
                Rel:      "create",
                Method:   "POST",
                Target:   hyper.Pathf("/contacts/%d/tags", id),
                Consumes: []string{"application/vnd.api+json"},
                Fields: []hyper.Field{
                    {Name: "name", Type: "text", Label: "Tag Name", Required: true},
                    {
                        Name:  "color",
                        Type:  "text",
                        Label: "Color",
                        Options: []hyper.Option{
                            {Value: "red", Label: "Red"},
                            {Value: "blue", Label: "Blue"},
                            {Value: "green", Label: "Green"},
                        },
                    },
                },
            },
        },
        Embedded: map[string][]hyper.Representation{
            "items": items,
        },
    }

    renderer.Respond(w, r, http.StatusOK, rep)
}
```

The CLI result — without any client-side changes:

```
$ cli contacts show 1

Contact #1
  Name:  Ada Lovelace
  ...

Navigate:
  contacts  All Contacts
  notes     Notes
  tags      Tags              <-- new, discovered automatically

$ cli contacts 1 tags

Tags for Ada Lovelace
┌──────────┬───────┐
│ Name     │ Color │
├──────────┼───────┤
│ VIP      │ red   │
│ Speaker  │ blue  │
└──────────┴───────┘

Available actions:
  create    Add a new tag (cli contacts 1 tags create --name NAME [--color COLOR])
```

The `--color` flag offers tab completion from `Field.Options` (§7.3). The CLI learned about the `tags` sub-resource, the `create` action, the `name` and `color` fields, and the color options — all from a single server-side change.

### 7.2 Removing a Feature

Removing a feature is equally simple — remove the link or action from the server's representation. The CLI stops discovering it. No deprecation warnings, no version negotiation, no client update. The affordance simply disappears from the command tree.

### 7.3 Conditional Affordances

The server can conditionally include actions based on authorization, resource state, or business rules:

```go
func contactActions(c Contact, user User) []hyper.Action {
    actions := []hyper.Action{}

    // Only the owner can update
    if user.ID == c.OwnerID {
        actions = append(actions, hyper.Action{
            Name:     "Update Contact",
            Rel:      "update",
            Method:   "PUT",
            Target:   hyper.Pathf("/contacts/%d", c.ID),
            Consumes: []string{"application/vnd.api+json"},
            Fields: []hyper.Field{
                {Name: "name", Type: "text", Label: "Name", Value: c.Name, Required: true},
                {Name: "email", Type: "email", Label: "Email", Value: c.Email, Required: true},
            },
        })
    }

    // Only admins can delete
    if user.IsAdmin {
        actions = append(actions, hyper.Action{
            Name:   "Delete Contact",
            Rel:    "delete",
            Method: "DELETE",
            Target: hyper.Pathf("/contacts/%d", c.ID),
            Hints:  map[string]any{"confirm": fmt.Sprintf("Delete %s?", c.Name), "destructive": true},
        })
    }

    return actions
}
```

The CLI displays only the actions the current user is authorized to perform. A non-admin user never sees the `delete` command. This is not access control enforcement (the server still validates on submission) — it is affordance-driven UI that eliminates "forbidden" errors by not offering forbidden operations.

## 8. Hints Are Server-Declared, Client-Interpreted

The `Hints` map (§15.6) lets the server declare semantic metadata about actions without prescribing presentation. The server says *what* an action means; the client decides *how* to present it.

### 8.1 Hint Examples

```go
// Server declares hints
action := hyper.Action{
    Name:   "Delete Contact",
    Rel:    "delete",
    Method: "DELETE",
    Target: hyper.MustParseTarget("/contacts/1"),
    Hints: map[string]any{
        "confirm":     "Are you sure you want to delete Ada Lovelace?",
        "destructive": true,
        "hidden":      false,
    },
}
```

On the wire:

```json
{
  "name": "Delete Contact",
  "rel": "delete",
  "method": "DELETE",
  "href": "/contacts/1",
  "hints": {
    "confirm": "Are you sure you want to delete Ada Lovelace?",
    "destructive": true,
    "hidden": false
  }
}
```

How different clients interpret the same hints:

| Hint | CLI Behavior | Browser Behavior | Mobile Behavior |
|---|---|---|---|
| `confirm` | Displays prompt, waits for `y/N` | Shows `confirm()` dialog | Shows alert dialog |
| `destructive` | Red/warning text styling | Red button color | Red-tinted action |
| `hidden` | Omits from `help` listing | Omits from navigation menu | Omits from action sheet |

The server declares semantics once. Every client renders them idiomatically.

## 9. Empty Collections and Edge Cases

### 9.1 Empty Collection

When a collection has no items, the server returns an empty `Embedded` slot:

```go
list := hyper.Representation{
    Kind: "contact-list",
    Self: hyper.MustParseTarget("/contacts").Ptr(),
    Links: []hyper.Link{
        {Rel: "root", Target: hyper.Path(), Title: "Home"},
    },
    Actions: []hyper.Action{
        {
            Name:     "Create Contact",
            Rel:      "create",
            Method:   "POST",
            Target:   hyper.MustParseTarget("/contacts"),
            Consumes: []string{"application/vnd.api+json"},
            Fields: []hyper.Field{
                {Name: "name", Type: "text", Label: "Name", Required: true},
                {Name: "email", Type: "email", Label: "Email", Required: true},
            },
        },
    },
    Embedded: map[string][]hyper.Representation{
        "items": {},
    },
}
```

On the wire:

```json
{
  "kind": "contact-list",
  "self": {"href": "/contacts"},
  "links": [
    {"rel": "root", "href": "/", "title": "Home"}
  ],
  "actions": [
    {
      "name": "Create Contact",
      "rel": "create",
      "method": "POST",
      "href": "/contacts",
      "consumes": ["application/vnd.api+json"],
      "fields": [
        {"name": "name", "type": "text", "label": "Name", "required": true},
        {"name": "email", "type": "email", "label": "Email", "required": true}
      ]
    }
  ],
  "embedded": {
    "items": []
  }
}
```

The CLI renders this gracefully:

```
$ cli contacts

Contacts
  (no items)

Available actions:
  create    Create a new contact (cli contacts create --name NAME --email EMAIL)
```

The `create` action is still available. The empty collection is not an error — it is a valid state with available transitions.

> **Open question:** Should an empty collection omit the `"items"` key entirely, use an empty array, or use `null`? The current approach (empty array) is explicit and avoids ambiguity, but the spec should clarify this.

### 9.2 Error Representations

The server returns error responses as representations. This means even error pages can carry hypermedia controls:

```go
func renderError(w http.ResponseWriter, r *http.Request, status int, message string) {
    rep := hyper.Representation{
        Kind: "error",
        State: hyper.Object{
            "status":  hyper.Scalar{V: status},
            "message": hyper.Scalar{V: message},
        },
        Links: []hyper.Link{
            {Rel: "root", Target: hyper.Path(), Title: "Home"},
        },
    }

    renderer.Respond(w, r, status, rep)
}
```

On the wire:

```json
{
  "kind": "error",
  "state": {
    "status": 404,
    "message": "Contact not found"
  },
  "links": [
    {"rel": "root", "href": "/", "title": "Home"}
  ]
}
```

The `root` link lets the CLI offer navigation back to the root even after an error. The server could include additional links or actions (e.g., a `search` action to help the user find what they were looking for).

## 10. Target and RouteRef

### 10.1 RouteRef Is Server-Internal

The `RouteRef` type (§8.1) allows the server to reference routes by name rather than by URL. This is a server-side convenience — it never appears on the wire.

```go
// Server-side: use RouteRef for type-safe URL generation
contact := hyper.Representation{
    Kind: "contact",
    Self: &hyper.Target{
        RouteRef: hyper.RouteRef{Name: "contact", Params: map[string]string{"id": "1"}},
    },
    Links: []hyper.Link{
        {
            Rel: "contacts",
            Target: hyper.Target{
                RouteRef: hyper.RouteRef{Name: "contact-list"},
            },
            Title: "All Contacts",
        },
    },
}
```

The `Resolver` (§8.2) resolves `RouteRef` values to `*url.URL` values before encoding. On the wire, only `href` strings appear:

```json
{
  "kind": "contact",
  "self": {"href": "/contacts/1"},
  "links": [
    {"rel": "contacts", "href": "/contacts", "title": "All Contacts"}
  ]
}
```

`RouteRef` provides compile-time route safety and decouples handlers from URL structure. If the URL pattern for contacts changes from `/contacts/:id` to `/people/:id`, only the router configuration changes — handlers continue to reference `RouteRef{Name: "contact"}` and the resolver produces the new URL.

### 10.2 Why This Matters for Interoperability

Because `RouteRef` never appears on the wire, third-party servers do not need to implement it. The wire format only requires `href` strings. `RouteRef` is a Go-side ergonomic that the `hyper` package offers, but it is not part of the compatibility contract.

## 11. Mutation Response Conventions

The spec does not currently prescribe what a server should return after a successful mutation (POST, PUT, DELETE). This document explores reasonable conventions.

### 11.1 Create (POST)

Return the created resource as a full representation with `201 Created`:

```go
renderer.Respond(w, r, http.StatusCreated, createdContact)
```

The CLI displays the created resource. The `Self` link on the returned representation gives the client the canonical URL of the new resource.

### 11.2 Update (PUT)

Return the updated resource as a full representation with `200 OK`:

```go
renderer.Respond(w, r, http.StatusOK, updatedContact)
```

The CLI displays the updated resource, confirming the changes.

### 11.3 Delete (DELETE)

Return a minimal representation with `200 OK` that links back to the parent collection:

```go
deleted := hyper.Representation{
    Kind: "deleted",
    State: hyper.Object{
        "message": hyper.Scalar{V: "Contact deleted"},
    },
    Links: []hyper.Link{
        {Rel: "contacts", Target: hyper.MustParseTarget("/contacts"), Title: "All Contacts"},
    },
}

renderer.Respond(w, r, http.StatusOK, deleted)
```

The CLI displays the confirmation message and offers navigation back to the collection. Alternatively, the server could return `204 No Content` with no body — but a representation with a navigation link provides a better client experience.

## 12. Putting It All Together

### 12.1 Full Server Skeleton (Go with `hyper`)

```go
func main() {
    renderer := hyper.NewRenderer(
        hyper.WithCodec("application/vnd.api+json", hyper.JSONCodec{}),
    )

    mux := http.NewServeMux()
    mux.HandleFunc("GET /", handleRoot)
    mux.HandleFunc("GET /contacts", handleContactList)
    mux.HandleFunc("POST /contacts", handleCreateContact)
    mux.HandleFunc("GET /contacts/{id}", handleContact)
    mux.HandleFunc("PUT /contacts/{id}", handleUpdateContact)
    mux.HandleFunc("DELETE /contacts/{id}", handleDeleteContact)
    mux.HandleFunc("GET /search", handleSearch)

    http.ListenAndServe(":8080", mux)
}
```

Every handler follows the same pattern:

1. Load data from the store
2. Build a `hyper.Representation` with state, links, actions, and embedded items
3. Call `renderer.Respond(w, r, status, rep)`

There is no CLI-specific code. There is no browser-specific code. There is no mobile-specific code. The server builds representations and the renderer encodes them. Adding a new client type (CLI, browser, mobile, script) requires zero server changes — the client reads the same representation and interprets it according to its own UI idiom.

### 12.2 What the Server Provides to All Clients

| Server Responsibility | CLI Uses It For | Browser Uses It For |
|---|---|---|
| `Representation.Kind` | Output formatter selection | Template/component selection |
| `Representation.State` | Key-value display, table columns | Page content |
| `Link` with `Rel` and `Title` | Subcommand names and help text | Navigation menu items |
| `Action` with `Fields` | Command flags with validation | HTML forms |
| `Field.Required` | Mandatory flag enforcement | Required field indicators |
| `Field.Value` | Default flag values | Pre-filled form inputs |
| `Field.Options` | Tab completion candidates | Select/dropdown options |
| `Field.Error` on 422 | Per-flag error messages | Inline form validation |
| `Action.Hints` | Confirmation prompts, styling | Button colors, visibility |
| `Embedded` items | Table rows, list items | Sub-components, cards |
| `Self` target | Resource URL for follow-up | Canonical link, breadcrumbs |
| Content negotiation | JSON hypermedia format | HTML rendering |

Every row in this table is a single server-side concern that serves multiple client types. The server never branches on client identity.

## 13. Spec Feedback

The following items surfaced while writing this server-side exploration:

- **No standard `SubmissionCodec` for `application/vnd.api+json`.** The spec defines `RepresentationCodec` for encoding outbound responses and `SubmissionCodec` for decoding inbound submissions (§9), but does not provide a standard `SubmissionCodec` for the `application/vnd.api+json` media type. Servers currently need to decode the flat JSON field submission manually. A standard submission codec would reduce boilerplate.

- **No guidance on mutation response conventions.** The spec does not describe what a server should return after a successful POST, PUT, or DELETE. Section 11 of this document proposes conventions (return the created/updated resource, return a "deleted" representation with navigation links), but these should be documented in the spec.

- **No recommended error representation structure.** The `kind: "error"` representation used throughout this document is an ad hoc convention. The spec should define or recommend a standard error representation shape, including how error messages map to `State` fields.

- **Validation error pattern (`Field.Error` on 422) should be documented explicitly.** The pattern of returning a representation with `Field.Error` values on `422 Unprocessable Entity` is powerful and client-agnostic, but it is only implicitly supported by the `Field` type definition (§7.3). The spec should document this as an explicit convention.

- **Empty collection `Embedded` handling needs clarification.** Should an empty collection use `"items": []`, omit the `"items"` key entirely, or allow `null`? The current spec is silent on this. The most explicit approach (empty array) is recommended but should be specified.

- **`RouteRef` should be emphasized as server-internal (never on the wire).** The spec defines `RouteRef` (§8.1) but does not explicitly state that it MUST NOT appear in encoded output. Third-party implementers in other languages need to know that `RouteRef` is a Go-side convenience, not part of the interoperability contract.

- **Portable compliance test suite would strengthen the interoperability story.** The TypeScript example in Section 5 demonstrates that any language can produce CLI-compatible responses, but there is no way to verify compliance. A test suite that validates JSON documents against the wire format schema would let server authors in any language confirm compatibility.

- **`Target.URL` (`*url.URL`) and convenience constructors improve ergonomics.** The spec now defines `Target.URL` as `*url.URL` instead of the original `string` `Href` field (§8.1), along with `Path`, `Pathf`, `MustParseTarget`, and `ParseTarget` constructors. This document's examples have been updated to use these constructors throughout. The wire format (§13.3.6) is unchanged — `url.URL` serializes to the same `{"href": "..."}` string.
