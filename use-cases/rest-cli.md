# Use Case: Hypermedia-Driven REST CLI Client

This document explores building a CLI client that discovers all available commands from a `hyper` API at runtime. The CLI has **no hard-coded endpoints** — it fetches the root `Representation` (§14.5), parses its `Links` and `Actions`, and constructs a command tree dynamically. The example domain is a contacts application.

## 1. Overview

Traditional REST CLI clients hard-code URL patterns: `GET /contacts`, `POST /contacts`, `DELETE /contacts/:id`. Every API change requires a client update. A hypermedia-driven CLI inverts this: it starts with a single base URL, fetches the root `Representation`, and builds its entire command surface from the `Links` and `Actions` it discovers.

Key properties:

- **Zero hard-coded routes** — the CLI only knows the base URL
- **Self-documenting** — `Link.Title`, `Field.Label`, and `Field.Help` populate `--help` text
- **Server-driven evolution** — adding a new `Action` on the server automatically surfaces a new CLI command
- **Hints-aware** — `Action.Hints` keys like `confirm`, `destructive`, and `hidden` (§15.6) control CLI behavior

## 2. Discovery Flow

### 2.1 Initial Connection

The CLI starts with a single base URL and fetches the root `Representation`:

```
$ cli --base http://localhost:8080/
```

1. The client checks for stored credentials for the base URL (see [Section 13.6](#136-credential-storage)) and includes them in the request if present
2. The client sends `GET /` with `Accept: application/vnd.api+json` (and `Authorization: Bearer <token>` if credentials are stored)
3. The server returns a root `Representation` encoded per the JSON wire format (§13.3) — the representation's `Links` and `Actions` may vary based on authentication state (see [Section 13](#13-authentication))
4. The client parses top-level `Links` and `Actions` to build a command tree

> **Note:** The initial root fetch may return a limited representation if the client is not authenticated. An unauthenticated root might expose only a `login` action, while an authenticated root exposes the full set of links and actions. The CLI should always check for stored credentials before the initial request to ensure the richest possible command tree on startup.

### 2.2 Root Representation (Server Side)

The server builds the root `Representation` using `hyper` types:

```go
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
```

### 2.3 Root Representation (JSON Wire Format)

On the wire (§13.3), this becomes:

```json
{
  "kind": "root",
  "self": {"href": "/"},
  "links": [
    {"rel": "contacts", "href": "/contacts", "title": "Contacts"}
  ],
  "actions": [
    {
      "name": "Search",
      "rel": "search",
      "method": "GET",
      "href": "/search",
      "fields": [
        {"name": "q", "type": "text", "label": "Query"}
      ]
    }
  ]
}
```

### 2.4 Command Tree Construction

The client parses this JSON and builds a command tree:

```
cli
├── contacts      (from Link rel="contacts")
└── search        (from Action rel="search")
    └── --q       (from Field name="q")
```

Each `Link` becomes a navigable subcommand. Each `Action` becomes an executable command whose `Fields` map to flags.

> **Note:** This is the *initial* command tree built from the root `Representation`. As the user navigates via `Links`, the command tree grows dynamically — each fetched `Representation` contributes its own `Links` and `Actions` as subcommands. See [Section 7: Nested Subcommands and Deep Navigation](#7-nested-subcommands-and-deep-navigation) for the full recursive algorithm.

## 3. Content Type Negotiation

The CLI uses content negotiation rather than hard-coding a content type. The server's advertised types — via response `Content-Type` headers and `Action.Consumes`/`Action.Produces` — drive how the CLI formats requests and parses responses.

### 3.1 Content Type Strategy

The CLI follows these content negotiation rules:

- The CLI sends `Accept: application/vnd.api+json` as its preferred type for JSON-based hypermedia responses
- The server's `Content-Type` response header determines how the CLI parses the response
- `Action.Consumes` determines the `Content-Type` header for request bodies
- `Action.Produces` (when present) hints at expected response types

The CLI uses `application/vnd.api+json` rather than bare `application/json` for several reasons:

- Bare `application/json` is ambiguous — it could be any JSON structure
- `application/vnd.api+json` signals "this is a structured hypermedia JSON document"
- The spec's JSON wire format (§13.3) defines what that structure looks like
- Servers can serve plain `application/json` (just state, no controls) alongside `application/vnd.api+json` (full hypermedia representation)

This distinction allows a server to serve different representations to different clients: a simple mobile app might request `application/json` and receive a flat state-only payload, while the CLI requests `application/vnd.api+json` and receives the full hypermedia representation with links, actions, and embedded resources.

### 3.2 Accept Header Construction

The CLI constructs its `Accept` header to express a preference ordering:

```
Accept: application/vnd.api+json, text/markdown;q=0.5, application/json;q=0.3
```

The CLI prefers the hypermedia JSON content type, falls back to markdown (for read-only views), and accepts plain JSON as a last resort. The quality values ensure the server selects the richest format it supports.

### 3.3 Request Body Content Types

`Action.Consumes` drives the request `Content-Type` header. The CLI inspects `Consumes` and selects the appropriate encoding:

- `["application/vnd.api+json"]` — submit as the hypermedia JSON format
- `["application/x-www-form-urlencoded"]` — submit as form data
- `["multipart/form-data"]` — submit with file uploads
- When `Consumes` is empty or absent, the CLI defaults to `application/vnd.api+json` for actions with fields, and sends no body for actions without fields

For example, the `create` action on the contacts list specifies `Consumes: ["application/vnd.api+json"]`, so the CLI submits the request body with `Content-Type: application/vnd.api+json`. If the server exposed a file upload action with `Consumes: ["multipart/form-data"]`, the CLI would switch to multipart encoding instead.

## 4. Command Mapping

The following table defines how `hyper` types map to CLI concepts:

| Hyper Concept | CLI Concept |
|---|---|
| Root `Links` | Top-level subcommands (e.g., `cli contacts`) |
| `Link.Rel` | Subcommand name |
| `Link.Title` | Help text for the subcommand |
| `Action.Name` | Command name (e.g., `cli search`) |
| `Action.Rel` | Command alias / identifier |
| `Action.Fields` | Flags (`--name`, `--email`) or positional args |
| `Field.Required` | Required vs optional flags |
| `Field.Type` | Flag value validation and completion |
| `Field.Options` | Enum completion candidates |
| `Field.Label` | Flag help text |
| `Field.Help` | Extended flag description |
| `Field.Value` | Default flag value |
| `Action.Method` | Implicit — the user never sees HTTP methods |
| `Action.Consumes` | Request `Content-Type` header selection — determines submission encoding (`application/vnd.api+json`, form-encoded, multipart) |
| `Action.Produces` | Response format expectation — hints at the content type the server will return |
| `Embedded` representations | Table rows / list items in output |
| `Representation.Kind` | Output formatter selection |
| `Action.Hints["confirm"]` | Confirmation prompt before execution |
| `Action.Hints["destructive"]` | Red/warning styling in terminal |
| `Action.Hints["hidden"]` | Suppress from default command listings |

## 5. Navigating a Collection

### 5.1 Following the `contacts` Link

When the user runs `cli contacts`, the client follows the `contacts` link discovered from the root:

1. The client sends `GET /contacts` with `Accept: application/vnd.api+json`
2. The server returns a contacts list `Representation` with `Embedded` items (per §6.1)

### 5.2 Contacts List (Server Side)

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
                {Name: "phone", Type: "tel", Label: "Phone"},
            },
        },
    },
    Embedded: map[string][]hyper.Representation{
        "items": {
            {
                Kind: "contact",
                Self: hyper.MustParseTarget("/contacts/1").Ptr(),
                State: hyper.Object{
                    "id":    hyper.Scalar{V: 1},
                    "name":  hyper.Scalar{V: "Ada Lovelace"},
                    "email": hyper.Scalar{V: "ada@example.com"},
                },
                Links: []hyper.Link{
                    {Rel: "self", Target: hyper.MustParseTarget("/contacts/1"), Title: "Ada Lovelace"},
                },
            },
            {
                Kind: "contact",
                Self: hyper.MustParseTarget("/contacts/2").Ptr(),
                State: hyper.Object{
                    "id":    hyper.Scalar{V: 2},
                    "name":  hyper.Scalar{V: "Grace Hopper"},
                    "email": hyper.Scalar{V: "grace@example.com"},
                },
                Links: []hyper.Link{
                    {Rel: "self", Target: hyper.MustParseTarget("/contacts/2"), Title: "Grace Hopper"},
                },
            },
        },
    },
}
```

### 5.3 Contacts List (JSON Wire Format)

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
        {"name": "email", "type": "email", "label": "Email", "required": true},
        {"name": "phone", "type": "tel", "label": "Phone"}
      ]
    }
  ],
  "embedded": {
    "items": [
      {
        "kind": "contact",
        "self": {"href": "/contacts/1"},
        "state": {"id": 1, "name": "Ada Lovelace", "email": "ada@example.com"},
        "links": [
          {"rel": "self", "href": "/contacts/1", "title": "Ada Lovelace"}
        ]
      },
      {
        "kind": "contact",
        "self": {"href": "/contacts/2"},
        "state": {"id": 2, "name": "Grace Hopper", "email": "grace@example.com"},
        "links": [
          {"rel": "self", "href": "/contacts/2", "title": "Grace Hopper"}
        ]
      }
    ]
  }
}
```

### 5.4 CLI Output

The CLI renders `Embedded` items from the `"items"` slot as a table and lists available actions:

```
$ cli contacts

Contacts
┌────┬────────────────┬───────────────────┐
│ ID │ Name           │ Email             │
├────┼────────────────┼───────────────────┤
│  1 │ Ada Lovelace   │ ada@example.com   │
│  2 │ Grace Hopper   │ grace@example.com │
└────┴────────────────┴───────────────────┘

Available actions:
  create    Create a new contact (cli contacts create --name NAME --email EMAIL [--phone PHONE])

Navigate to a contact:
  cli contacts show <id>
```

Each embedded `Representation` carries its own `Self` `Target`, `Links`, and `Actions`, enabling the CLI to offer follow-up navigation per item.

## 6. Viewing a Single Resource

### 6.1 Following an Item Link

When the user runs `cli contacts show 1`, the client resolves the `Self` target of the embedded item and fetches `/contacts/1`.

### 6.2 Single Contact (Server Side)

```go
contact := hyper.Representation{
    Kind: "contact",
    Self: hyper.MustParseTarget("/contacts/1").Ptr(),
    State: hyper.Object{
        "id":    hyper.Scalar{V: 1},
        "name":  hyper.Scalar{V: "Ada Lovelace"},
        "email": hyper.Scalar{V: "ada@example.com"},
        "phone": hyper.Scalar{V: "+1-555-0100"},
        "bio": hyper.RichText{
            MediaType: "text/markdown",
            Source:    "Wrote the *first* algorithm intended for a machine.",
        },
    },
    Links: []hyper.Link{
        {Rel: "contacts", Target: hyper.MustParseTarget("/contacts"), Title: "All Contacts"},
    },
    Actions: []hyper.Action{
        {
            Name:     "Update Contact",
            Rel:      "update",
            Method:   "PUT",
            Target:   hyper.MustParseTarget("/contacts/1"),
            Consumes: []string{"application/vnd.api+json"},
            Fields: []hyper.Field{
                {Name: "name", Type: "text", Label: "Name", Value: "Ada Lovelace", Required: true},
                {Name: "email", Type: "email", Label: "Email", Value: "ada@example.com", Required: true},
                {Name: "phone", Type: "tel", Label: "Phone", Value: "+1-555-0100"},
            },
        },
        {
            Name:   "Delete Contact",
            Rel:    "delete",
            Method: "DELETE",
            Target: hyper.MustParseTarget("/contacts/1"),
            Hints: map[string]any{
                "confirm":     "Are you sure you want to delete Ada Lovelace?",
                "destructive": true,
            },
        },
    },
}
```

### 6.3 Single Contact (JSON Wire Format)

```json
{
  "kind": "contact",
  "self": {"href": "/contacts/1"},
  "state": {
    "id": 1,
    "name": "Ada Lovelace",
    "email": "ada@example.com",
    "phone": "+1-555-0100",
    "bio": {"_type": "richtext", "mediaType": "text/markdown", "source": "Wrote the *first* algorithm intended for a machine."}
  },
  "links": [
    {"rel": "contacts", "href": "/contacts", "title": "All Contacts"}
  ],
  "actions": [
    {
      "name": "Update Contact",
      "rel": "update",
      "method": "PUT",
      "href": "/contacts/1",
      "consumes": ["application/vnd.api+json"],
      "fields": [
        {"name": "name", "type": "text", "label": "Name", "value": "Ada Lovelace", "required": true},
        {"name": "email", "type": "email", "label": "Email", "value": "ada@example.com", "required": true},
        {"name": "phone", "type": "tel", "label": "Phone", "value": "+1-555-0100"}
      ]
    },
    {
      "name": "Delete Contact",
      "rel": "delete",
      "method": "DELETE",
      "href": "/contacts/1",
      "hints": {
        "confirm": "Are you sure you want to delete Ada Lovelace?",
        "destructive": true
      }
    }
  ]
}
```

### 6.4 CLI Output

The CLI renders `State` as key-value pairs and lists available actions as subcommands:

```
$ cli contacts show 1

Contact #1
  Name:  Ada Lovelace
  Email: ada@example.com
  Phone: +1-555-0100
  Bio:   Wrote the *first* algorithm intended for a machine.

Available actions:
  update    Update this contact (cli contacts update 1 --name NAME --email EMAIL [--phone PHONE])
  delete    Delete this contact (cli contacts delete 1) [destructive]

Navigate:
  contacts  All Contacts (cli contacts)
```

## 7. Nested Subcommands and Deep Navigation

The examples so far show a flat command tree: `cli contacts`, `cli contacts show 1`. Real APIs have nested resources — a contact has notes, a note has attachments. The hypermedia-driven CLI handles arbitrary nesting by following `Links` discovered at each level. The CLI never hard-codes nesting depth; each navigation step applies the same algorithm to whatever `Representation` the server returns.

### 7.1 Nested Resource Discovery

When the server returns a single contact `Representation`, it can expose `Links` to sub-resources alongside its existing `Actions`. Here the contact carries links to its notes and tags:

```go
contact := hyper.Representation{
    Kind: "contact",
    Self: hyper.MustParseTarget("/contacts/1").Ptr(),
    State: hyper.Object{
        "id":   hyper.Scalar{V: 1},
        "name": hyper.Scalar{V: "Ada Lovelace"},
    },
    Links: []hyper.Link{
        {Rel: "contacts", Target: hyper.MustParseTarget("/contacts"), Title: "All Contacts"},
        {Rel: "notes", Target: hyper.MustParseTarget("/contacts/1/notes"), Title: "Notes"},
        {Rel: "tags", Target: hyper.MustParseTarget("/contacts/1/tags"), Title: "Tags"},
    },
    Actions: []hyper.Action{
        {
            Name:     "Update Contact",
            Rel:      "update",
            Method:   "PUT",
            Target:   hyper.MustParseTarget("/contacts/1"),
            Consumes: []string{"application/vnd.api+json"},
            Fields: []hyper.Field{
                {Name: "name", Type: "text", Label: "Name", Value: "Ada Lovelace", Required: true},
                {Name: "email", Type: "email", Label: "Email", Value: "ada@example.com", Required: true},
            },
        },
        {
            Name:   "Delete Contact",
            Rel:    "delete",
            Method: "DELETE",
            Target: hyper.MustParseTarget("/contacts/1"),
            Hints:  map[string]any{"confirm": "Are you sure you want to delete Ada Lovelace?", "destructive": true},
        },
    },
}
```

On the wire (§13.3):

```json
{
  "kind": "contact",
  "self": {"href": "/contacts/1"},
  "state": {
    "id": 1,
    "name": "Ada Lovelace"
  },
  "links": [
    {"rel": "contacts", "href": "/contacts", "title": "All Contacts"},
    {"rel": "notes", "href": "/contacts/1/notes", "title": "Notes"},
    {"rel": "tags", "href": "/contacts/1/tags", "title": "Tags"}
  ],
  "actions": [
    {
      "name": "Update Contact",
      "rel": "update",
      "method": "PUT",
      "href": "/contacts/1",
      "consumes": ["application/vnd.api+json"],
      "fields": [
        {"name": "name", "type": "text", "label": "Name", "value": "Ada Lovelace", "required": true},
        {"name": "email", "type": "email", "label": "Email", "value": "ada@example.com", "required": true}
      ]
    },
    {
      "name": "Delete Contact",
      "rel": "delete",
      "method": "DELETE",
      "href": "/contacts/1",
      "hints": {"confirm": "Are you sure you want to delete Ada Lovelace?", "destructive": true}
    }
  ]
}
```

The CLI parses this `Representation` and builds a subcommand tree for the current context:

```
cli contacts show 1
├── notes       (from Link rel="notes")
├── tags        (from Link rel="tags")
├── update      (from Action rel="update")
└── delete      (from Action rel="delete")
```

Each `Link` becomes a navigable subcommand that will fetch a new `Representation` and repeat the process. Each `Action` becomes an executable command with flags derived from its `Fields`.

### 7.2 Following Nested Links

When the user runs `cli contacts 1 notes`, the CLI follows the `notes` link from the contact `Representation`:

1. The client sends `GET /contacts/1/notes` with `Accept: application/vnd.api+json`
2. The server returns a notes list `Representation` with its own `Embedded` items, `Actions`, and `Links`

#### Notes List (Server Side)

```go
notesList := hyper.Representation{
    Kind: "note-list",
    Self: hyper.MustParseTarget("/contacts/1/notes").Ptr(),
    Links: []hyper.Link{
        {Rel: "contact", Target: hyper.MustParseTarget("/contacts/1"), Title: "Ada Lovelace"},
    },
    Actions: []hyper.Action{
        {
            Name:     "Add Note",
            Rel:      "create",
            Method:   "POST",
            Target:   hyper.MustParseTarget("/contacts/1/notes"),
            Consumes: []string{"application/vnd.api+json"},
            Fields: []hyper.Field{
                {Name: "title", Type: "text", Label: "Title", Required: true},
                {Name: "body", Type: "text", Label: "Body", Required: true},
            },
        },
    },
    Embedded: map[string][]hyper.Representation{
        "items": {
            {
                Kind: "note",
                Self: hyper.MustParseTarget("/contacts/1/notes/3").Ptr(),
                State: hyper.Object{
                    "id":      hyper.Scalar{V: 3},
                    "title":   hyper.Scalar{V: "Meeting notes"},
                    "created": hyper.Scalar{V: "2025-11-20"},
                },
                Links: []hyper.Link{
                    {Rel: "self", Target: hyper.MustParseTarget("/contacts/1/notes/3"), Title: "Meeting notes"},
                },
            },
            {
                Kind: "note",
                Self: hyper.MustParseTarget("/contacts/1/notes/7").Ptr(),
                State: hyper.Object{
                    "id":      hyper.Scalar{V: 7},
                    "title":   hyper.Scalar{V: "Follow-up tasks"},
                    "created": hyper.Scalar{V: "2025-12-03"},
                },
                Links: []hyper.Link{
                    {Rel: "self", Target: hyper.MustParseTarget("/contacts/1/notes/7"), Title: "Follow-up tasks"},
                },
            },
        },
    },
}
```

#### Notes List (JSON Wire Format)

```json
{
  "kind": "note-list",
  "self": {"href": "/contacts/1/notes"},
  "links": [
    {"rel": "contact", "href": "/contacts/1", "title": "Ada Lovelace"}
  ],
  "actions": [
    {
      "name": "Add Note",
      "rel": "create",
      "method": "POST",
      "href": "/contacts/1/notes",
      "consumes": ["application/vnd.api+json"],
      "fields": [
        {"name": "title", "type": "text", "label": "Title", "required": true},
        {"name": "body", "type": "text", "label": "Body", "required": true}
      ]
    }
  ],
  "embedded": {
    "items": [
      {
        "kind": "note",
        "self": {"href": "/contacts/1/notes/3"},
        "state": {"id": 3, "title": "Meeting notes", "created": "2025-11-20"},
        "links": [
          {"rel": "self", "href": "/contacts/1/notes/3", "title": "Meeting notes"}
        ]
      },
      {
        "kind": "note",
        "self": {"href": "/contacts/1/notes/7"},
        "state": {"id": 7, "title": "Follow-up tasks", "created": "2025-12-03"},
        "links": [
          {"rel": "self", "href": "/contacts/1/notes/7", "title": "Follow-up tasks"}
        ]
      }
    ]
  }
}
```

#### CLI Output

```
$ cli contacts 1 notes

Notes for Ada Lovelace
┌────┬──────────────────┬────────────┐
│ ID │ Title            │ Created    │
├────┼──────────────────┼────────────┤
│  3 │ Meeting notes    │ 2025-11-20 │
│  7 │ Follow-up tasks  │ 2025-12-03 │
└────┴──────────────────┴────────────┘

Available actions:
  create    Add a new note (cli contacts 1 notes create --title TITLE --body BODY)

Navigate:
  contact   Ada Lovelace (cli contacts show 1)

Navigate to a note:
  cli contacts 1 notes show <id>
```

The notes list `Representation` is structurally identical to the contacts list — it has `Embedded` items, `Actions`, and `Links`. The CLI applies the same rendering and command-building logic at every level.

### 7.3 Deeply Nested Navigation

When the user runs `cli contacts 1 notes 3`, the CLI resolves the `Self` target of the embedded note and fetches `/contacts/1/notes/3`:

#### Single Note (Server Side)

```go
note := hyper.Representation{
    Kind: "note",
    Self: hyper.MustParseTarget("/contacts/1/notes/3").Ptr(),
    State: hyper.Object{
        "id":      hyper.Scalar{V: 3},
        "title":   hyper.Scalar{V: "Meeting notes"},
        "created": hyper.Scalar{V: "2025-11-20"},
        "body": hyper.RichText{
            MediaType: "text/markdown",
            Source:    "Discussed the *analytical engine* project timeline.",
        },
    },
    Links: []hyper.Link{
        {Rel: "notes", Target: hyper.MustParseTarget("/contacts/1/notes"), Title: "All Notes"},
        {Rel: "contact", Target: hyper.MustParseTarget("/contacts/1"), Title: "Ada Lovelace"},
    },
    Actions: []hyper.Action{
        {
            Name:     "Edit Note",
            Rel:      "update",
            Method:   "PUT",
            Target:   hyper.MustParseTarget("/contacts/1/notes/3"),
            Consumes: []string{"application/vnd.api+json"},
            Fields: []hyper.Field{
                {Name: "title", Type: "text", Label: "Title", Value: "Meeting notes", Required: true},
                {Name: "body", Type: "text", Label: "Body", Value: "Discussed the *analytical engine* project timeline.", Required: true},
            },
        },
        {
            Name:   "Delete Note",
            Rel:    "delete",
            Method: "DELETE",
            Target: hyper.MustParseTarget("/contacts/1/notes/3"),
            Hints:  map[string]any{"confirm": "Delete this note?", "destructive": true},
        },
    },
}
```

#### Single Note (JSON Wire Format)

```json
{
  "kind": "note",
  "self": {"href": "/contacts/1/notes/3"},
  "state": {
    "id": 3,
    "title": "Meeting notes",
    "created": "2025-11-20",
    "body": {"_type": "richtext", "mediaType": "text/markdown", "source": "Discussed the *analytical engine* project timeline."}
  },
  "links": [
    {"rel": "notes", "href": "/contacts/1/notes", "title": "All Notes"},
    {"rel": "contact", "href": "/contacts/1", "title": "Ada Lovelace"}
  ],
  "actions": [
    {
      "name": "Edit Note",
      "rel": "update",
      "method": "PUT",
      "href": "/contacts/1/notes/3",
      "consumes": ["application/vnd.api+json"],
      "fields": [
        {"name": "title", "type": "text", "label": "Title", "value": "Meeting notes", "required": true},
        {"name": "body", "type": "text", "label": "Body", "value": "Discussed the *analytical engine* project timeline.", "required": true}
      ]
    },
    {
      "name": "Delete Note",
      "rel": "delete",
      "method": "DELETE",
      "href": "/contacts/1/notes/3",
      "hints": {"confirm": "Delete this note?", "destructive": true}
    }
  ]
}
```

#### CLI Output

```
$ cli contacts 1 notes 3

Note #3
  Title:   Meeting notes
  Created: 2025-11-20
  Body:    Discussed the *analytical engine* project timeline.

Available actions:
  update    Edit this note (cli contacts 1 notes update 3 --title TITLE --body BODY)
  delete    Delete this note (cli contacts 1 notes delete 3) [destructive]

Navigate:
  notes     All Notes (cli contacts 1 notes)
  contact   Ada Lovelace (cli contacts show 1)
```

The note `Representation` carries its own `Links` back to the notes list and the parent contact, plus its own `Actions` for editing and deleting. The CLI constructs all of this from the server response — it has no built-in knowledge of the contact/note relationship.

### 7.4 Command Tree Construction Algorithm

The CLI uses a single recursive algorithm for every navigation step:

1. **Fetch** the current `Representation` from the server
2. **For each `Link`**, register a navigable subcommand named after `Link.Rel`. When the user invokes it, follow `Link.Target.URL` and recurse from step 1
3. **For each `Action`**, register an executable command named after `Action.Rel`. Generate flags from `Action.Fields` — `Field.Name` becomes the flag name, `Field.Required` determines whether it is mandatory, `Field.Value` provides the default, and `Field.Type` drives validation
4. **For `Embedded` items** with `Self` targets, register `show <id>` subcommands. When the user invokes one, fetch `Self.URL` and recurse from step 1

In pseudocode:

```go
func buildCommands(rep hyper.Representation) *CommandGroup {
    group := &CommandGroup{Kind: rep.Kind, Self: rep.Self}

    // Links become navigable subcommands
    for _, link := range rep.Links {
        group.AddSubcommand(link.Rel, link.Title, func() {
            next := fetch(link.Target.URL)
            buildCommands(next)  // recurse
        })
    }

    // Actions become executable commands with flags
    for _, action := range rep.Actions {
        cmd := group.AddCommand(action.Rel, action.Name)
        for _, field := range action.Fields {
            cmd.AddFlag(field.Name, field.Label, field.Value, field.Required)
        }
        cmd.OnExecute = func(flags map[string]any) {
            submit(action, flags)
        }
    }

    // Embedded items with Self targets become "show <id>" subcommands
    for _, items := range rep.Embedded {
        for _, item := range items {
            if item.Self != nil {
                id := item.State["id"]
                group.AddSubcommand("show "+id.String(), item.Kind, func() {
                    next := fetch(item.Self.URL)
                    buildCommands(next)  // recurse
                })
            }
        }
    }

    return group
}
```

The critical insight: the CLI never hard-codes nesting depth. Whether the user navigates to `cli contacts`, `cli contacts 1 notes`, or `cli contacts 1 notes 3 attachments`, each step is the same algorithm applied to whatever `Representation` the server returns. The server controls the shape of the resource hierarchy through the `Links` it includes in each response.

### 7.5 Breadcrumb / Path Display

The interactive REPL prompt reflects nesting depth using the `Representation.Kind` and `Self.URL` at each level. As the user navigates deeper, the prompt updates to show the current position in the resource hierarchy:

```
root> contacts
contact-list> show 1
contact(/contacts/1)> notes
note-list(/contacts/1/notes)> show 3
note(/contacts/1/notes/3)> back
note-list(/contacts/1/notes)> back
contact(/contacts/1)> back
contact-list> back
root>
```

Each prompt segment is derived from the current `Representation`:
- `Kind` provides the human-readable context type (e.g., `note-list`)
- `Self.URL` provides the path (e.g., `/contacts/1/notes`)
- `back` pops the navigation stack and returns to the previous `Representation`

### 7.6 CLI Output for Nested Resources

The CLI output for `cli contacts 1 notes` follows the same pattern as any collection — `Embedded` items render as a table, `Actions` list as available commands, and `Links` provide navigation options:

```
$ cli contacts 1 notes

Notes for Ada Lovelace
┌────┬──────────────────┬────────────┐
│ ID │ Title            │ Created    │
├────┼──────────────────┼────────────┤
│  3 │ Meeting notes    │ 2025-11-20 │
│  7 │ Follow-up tasks  │ 2025-12-03 │
└────┴──────────────────┴────────────┘

Available actions:
  create    Add a new note (cli contacts 1 notes create --title TITLE --body BODY)

Navigate:
  contact   Ada Lovelace (cli contacts show 1)

Navigate to a note:
  cli contacts 1 notes show <id>
```

The output structure is identical to the top-level contacts list (Section 5.4). The CLI uses the same rendering logic regardless of nesting depth — the `Representation.Kind` selects the formatter, `Embedded["items"]` provides the table rows, `Actions` list the available mutations, and `Links` offer navigation back to the parent resource.

## 8. Executing Actions

### 8.1 Create

The `create` `Action` is discovered on the contacts list `Representation` (§7.2). Its `Fields` map to CLI flags:

```
$ cli contacts create --name "Alan Turing" --email "alan@example.com"
```

The CLI:

1. Finds the `Action` with `Rel: "create"` on the contacts list `Representation`
2. Maps `--name` and `--email` to `Field` values
3. Validates that required `Fields` (`name`, `email`) are present
4. Submits a JSON body (from `Action.Consumes: ["application/vnd.api+json"]`) to the `Action.Target`:

```
POST /contacts
Content-Type: application/vnd.api+json

{"name": "Alan Turing", "email": "alan@example.com"}
```

5. Parses the response `Representation` and displays the created resource:

```
Created contact #3
  Name:  Alan Turing
  Email: alan@example.com
```

### 8.2 Update

The `update` `Action` is discovered on the single contact `Representation`. `Field.Value` provides defaults so the user only needs to specify fields being changed:

```
$ cli contacts update 1 --email "ada.lovelace@example.com"
```

The CLI:

1. Fetches `/contacts/1` to discover available `Actions`
2. Finds the `Action` with `Rel: "update"`
3. Pre-populates flags from `Field.Value` — `name` defaults to `"Ada Lovelace"`, `email` defaults to `"ada@example.com"`
4. Overrides `email` with the user-provided value
5. Submits:

```
PUT /contacts/1
Content-Type: application/vnd.api+json

{"name": "Ada Lovelace", "email": "ada.lovelace@example.com", "phone": "+1-555-0100"}
```

6. Displays the updated resource from the response.

### 8.3 Delete

The `delete` `Action` carries `Hints` (§15.6) that affect CLI behavior:

```
$ cli contacts delete 1
```

The CLI:

1. Fetches `/contacts/1` to discover available `Actions`
2. Finds the `Action` with `Rel: "delete"`
3. Reads `Action.Hints["confirm"]` — displays the confirmation prompt
4. Reads `Action.Hints["destructive"]` — renders the prompt in red

```
$ cli contacts delete 1
⚠  Are you sure you want to delete Ada Lovelace? [y/N] y

Deleted contact #1.
```

If the user declines, the CLI exits without sending a request. If `Hints["hidden"]` were `true`, the action would not appear in default action listings but could still be invoked explicitly.

## 9. Search as a GET Action with Fields

The root `Representation` exposes a `Search` `Action` with `Method: "GET"` and a `Field` for the query parameter:

```
$ cli search --q "Ada"
```

The CLI:

1. Finds the `Action` with `Rel: "search"` on the root `Representation`
2. Since `Method` is `GET`, maps `Fields` to query parameters instead of a request body
3. Sends `GET /search?q=Ada` with `Accept: application/vnd.api+json`

### 9.1 Search Results (Server Side)

```go
results := hyper.Representation{
    Kind: "search-results",
    Self: hyper.MustParseTarget("/search?q=Ada").Ptr(),
    State: hyper.Object{
        "query": hyper.Scalar{V: "Ada"},
    },
    Embedded: map[string][]hyper.Representation{
        "items": {
            {
                Kind: "contact",
                Self: hyper.MustParseTarget("/contacts/1").Ptr(),
                State: hyper.Object{
                    "id":    hyper.Scalar{V: 1},
                    "name":  hyper.Scalar{V: "Ada Lovelace"},
                    "email": hyper.Scalar{V: "ada@example.com"},
                },
            },
        },
    },
}
```

### 9.2 Search Results (JSON Wire Format)

```json
{
  "kind": "search-results",
  "self": {"href": "/search?q=Ada"},
  "state": {"query": "Ada"},
  "embedded": {
    "items": [
      {
        "kind": "contact",
        "self": {"href": "/contacts/1"},
        "state": {"id": 1, "name": "Ada Lovelace", "email": "ada@example.com"}
      }
    ]
  }
}
```

### 9.3 CLI Output

```
$ cli search --q "Ada"

Search results for "Ada"
┌────┬────────────────┬───────────────────┐
│ ID │ Name           │ Email             │
├────┼────────────────┼───────────────────┤
│  1 │ Ada Lovelace   │ ada@example.com   │
└────┴────────────────┴───────────────────┘
```

## 10. Interactive Mode

When invoked with no arguments, the CLI enters an interactive REPL that mirrors the hypermedia navigation model:

```
$ cli --base http://localhost:8080/
Connected to http://localhost:8080/

root> help
Available commands:
  contacts    Contacts
  search      Search (--q QUERY)

root> contacts
Contacts
┌────┬────────────────┬───────────────────┐
│ ID │ Name           │ Email             │
├────┼────────────────┼───────────────────┤
│  1 │ Ada Lovelace   │ ada@example.com   │
│  2 │ Grace Hopper   │ grace@example.com │
└────┴────────────────┴───────────────────┘

contact-list> show 1

Contact #1
  Name:  Ada Lovelace
  Email: ada@example.com

contact(/contacts/1)> help
Available actions:
  update    Update this contact
  delete    Delete this contact [destructive]

Navigate:
  contacts  All Contacts
  notes     Notes
  tags      Tags
  back      Return to previous view

contact(/contacts/1)> notes

Notes for Ada Lovelace
┌────┬──────────────────┬────────────┐
│ ID │ Title            │ Created    │
├────┼──────────────────┼────────────┤
│  3 │ Meeting notes    │ 2025-11-20 │
│  7 │ Follow-up tasks  │ 2025-12-03 │
└────┴──────────────────┴────────────┘

note-list(/contacts/1/notes)> show 3

Note #3
  Title:   Meeting notes
  Created: 2025-11-20
  Body:    Discussed the *analytical engine* project timeline.

note(/contacts/1/notes/3)> help
Available actions:
  update    Edit this note
  delete    Delete this note [destructive]

Navigate:
  notes     All Notes
  contact   Ada Lovelace
  back      Return to previous view

note(/contacts/1/notes/3)> back
note-list(/contacts/1/notes)> back
contact(/contacts/1)> back
contact-list>
```

Key behaviors:

- The prompt reflects the current `Representation.Kind` and path
- `help` is generated dynamically from the current `Representation`'s `Links` and `Actions`
- Tab completion is populated from `Field.Options` and discovered `Link.Rel` / `Action.Rel` values
- After each response, the CLI re-parses the new `Representation` and updates available commands
- `Action.Hints["hidden"]` actions are omitted from `help` output but remain invocable
- Nested navigation works identically to top-level navigation — each `Representation` defines the available commands at that level

## 11. Output Formatting

The CLI supports multiple output modes, driven by the `Representation` and user flags:

### 11.1 Default Formatting

- **Collections** (`Embedded` with `"items"` slot): rendered as tables, with columns inferred from the `State` keys of the first embedded `Representation`
- **Single resources**: rendered as key-value pairs
- **`Representation.Kind`** can select specialized formatters — a kind of `"contact"` might use a contact-specific layout, while an unrecognized kind falls back to generic formatting

### 11.2 `--json` Flag

Outputs the raw response body as received from the server. When the server responds with `application/vnd.api+json`, this is the full hypermedia representation including links, actions, and embedded resources. When the server responds with `application/json`, it may be a reduced state-only payload without hypermedia controls.

```
$ cli contacts --json
{
  "kind": "contact-list",
  "self": {"href": "/contacts"},
  ...
}
```

### 11.3 `--markdown` Flag

Requests `Accept: text/markdown` from the server, which triggers the Markdown codec (§12):

```
$ cli contacts show 1 --markdown

# Ada Lovelace

- **Email:** ada@example.com
- **Phone:** +1-555-0100
- **Bio:** Wrote the *first* algorithm intended for a machine.
```

### 11.4 Kind-Based Formatting

The `Representation.Kind` field allows the CLI to register specialized formatters:

```go
formatters := map[string]Formatter{
    "contact":      contactFormatter,
    "contact-list": contactTableFormatter,
    "search-results": searchResultsFormatter,
}

// Fallback to generic formatter for unknown kinds
func render(rep Representation) {
    if f, ok := formatters[rep.Kind]; ok {
        f.Render(rep)
    } else {
        genericFormatter.Render(rep)
    }
}
```

## 12. Error Handling

### 12.1 HTTP Errors

When the server returns an error status, the CLI displays the status code and any error `Representation` body:

```
$ cli contacts show 999

Error 404: Not Found
  Contact not found.
```

### 12.2 Validation Errors

When the server returns `422 Unprocessable Entity`, it includes a `Representation` whose `Action.Fields` carry `Field.Error` values (§7.3):

```go
validationRep := hyper.Representation{
    Kind: "contact-form",
    Self: hyper.MustParseTarget("/contacts").Ptr(),
    Actions: []hyper.Action{
        {
            Name:     "Create Contact",
            Rel:      "create",
            Method:   "POST",
            Target:   hyper.MustParseTarget("/contacts"),
            Consumes: []string{"application/vnd.api+json"},
            Fields: []hyper.Field{
                {Name: "name", Type: "text", Label: "Name", Value: "", Required: true, Error: "Name is required"},
                {Name: "email", Type: "email", Label: "Email", Value: "not-an-email", Required: true, Error: "Invalid email address"},
                {Name: "phone", Type: "tel", Label: "Phone"},
            },
        },
    },
}
```

The JSON wire format:

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
        {"name": "phone", "type": "tel", "label": "Phone"}
      ]
    }
  ]
}
```

The CLI maps `Field.Error` back to the corresponding flags:

```
$ cli contacts create --email "not-an-email"

Validation failed:
  --name:  Name is required
  --email: Invalid email address
```

### 12.3 Network Errors

When the server is unreachable, the CLI reports the connection failure and suggests retrying:

```
$ cli contacts

Error: could not connect to http://localhost:8080/contacts
  Connection refused. Is the server running?
```

## 13. Authentication

A hypermedia-driven CLI discovers authentication affordances the same way it discovers everything else: through `Links` and `Actions` on the root `Representation`. The server controls what is available based on auth state — an unauthenticated root representation exposes a `login` action, while an authenticated one exposes a `logout` action and additional protected links.

### 13.1 Unauthenticated Root Representation

When the CLI connects without stored credentials, the server returns a root `Representation` with limited affordances:

```go
root := hyper.Representation{
    Kind: "root",
    Self: hyper.Path().Ptr(),
    Links: []hyper.Link{
        // No "contacts" link — requires auth
    },
    Actions: []hyper.Action{
        {
            Name:   "Login",
            Rel:    "login",
            Method: "POST",
            Target: hyper.MustParseTarget("/auth/login"),
            Consumes: []string{"application/vnd.api+json"},
            Fields: []hyper.Field{
                {Name: "username", Type: "text", Label: "Username", Required: true},
                {Name: "password", Type: "password", Label: "Password", Required: true},
            },
        },
    },
}
```

The JSON wire format:

```json
{
  "kind": "root",
  "self": {"href": "/"},
  "links": [],
  "actions": [
    {
      "name": "Login",
      "rel": "login",
      "method": "POST",
      "href": "/auth/login",
      "consumes": ["application/vnd.api+json"],
      "fields": [
        {"name": "username", "type": "text", "label": "Username", "required": true},
        {"name": "password", "type": "password", "label": "Password", "required": true}
      ]
    }
  ]
}
```

The CLI builds a minimal command tree from this unauthenticated representation:

```
cli
└── login      (from Action rel="login")
    ├── --username
    └── --password
```

The only available command is `login`. Protected resources like `contacts` are not exposed — the server simply omits those `Links` and `Actions` from the representation.

### 13.2 Login Flow

The user authenticates with `cli login --username ada --password secret`:

1. The CLI finds the `login` `Action` on the root `Representation` by matching `Action.Rel == "login"`
2. `Field.Type: "password"` triggers masked input if the `--password` flag is omitted — the CLI prompts interactively without echoing characters
3. The CLI submits credentials to the action's `Target.URL` (`/auth/login`) using the method (`POST`) and content type specified in `Action.Consumes` (`application/vnd.api+json`)
4. The server validates credentials and returns a response `Representation` with a token
5. The CLI stores the credential locally (e.g., `~/.config/cli/credentials.json`)
6. The CLI re-fetches the root representation with the stored credential in an `Authorization` header, revealing new links and actions

The server returns a response `Representation` containing the auth token:

```go
loginResult := hyper.Representation{
    Kind: "auth-token",
    State: hyper.Object{
        "token":      hyper.Scalar{V: "eyJhbGci..."},
        "expires_at": hyper.Scalar{V: "2026-04-01T00:00:00Z"},
    },
    Links: []hyper.Link{
        {Rel: "root", Target: hyper.Path(), Title: "Home"},
    },
}
```

The JSON wire format:

```json
{
  "kind": "auth-token",
  "state": {
    "token": "eyJhbGci...",
    "expires_at": "2026-04-01T00:00:00Z"
  },
  "links": [
    {"rel": "root", "href": "/", "title": "Home"}
  ]
}
```

The CLI stores the token keyed by base URL and follows the `root` link to re-fetch the root representation with the new credential.

### 13.3 Authenticated Root Representation

After login, the CLI re-fetches the root with `Authorization: Bearer eyJhbGci...`. The server now returns a richer `Representation` with protected links and a `logout` action:

```go
root := hyper.Representation{
    Kind: "root",
    Self: hyper.Path().Ptr(),
    State: hyper.Object{
        "user": hyper.Scalar{V: "ada"},
    },
    Links: []hyper.Link{
        {Rel: "contacts", Target: hyper.MustParseTarget("/contacts"), Title: "Contacts"},
        {Rel: "profile", Target: hyper.MustParseTarget("/profile"), Title: "My Profile"},
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
        {
            Name:   "Logout",
            Rel:    "logout",
            Method: "POST",
            Target: hyper.MustParseTarget("/auth/logout"),
            Hints: map[string]any{
                "confirm": "Are you sure you want to log out?",
            },
        },
    },
}
```

The JSON wire format:

```json
{
  "kind": "root",
  "self": {"href": "/"},
  "state": {
    "user": "ada"
  },
  "links": [
    {"rel": "contacts", "href": "/contacts", "title": "Contacts"},
    {"rel": "profile", "href": "/profile", "title": "My Profile"}
  ],
  "actions": [
    {
      "name": "Search",
      "rel": "search",
      "method": "GET",
      "href": "/search",
      "fields": [
        {"name": "q", "type": "text", "label": "Query"}
      ]
    },
    {
      "name": "Logout",
      "rel": "logout",
      "method": "POST",
      "href": "/auth/logout",
      "hints": {
        "confirm": "Are you sure you want to log out?"
      }
    }
  ]
}
```

The updated command tree now includes protected resources:

```
cli
├── contacts    (from Link rel="contacts")
├── profile     (from Link rel="profile")
├── search      (from Action rel="search")
└── logout      (from Action rel="logout")
```

The `login` action is no longer present — the server omits it for authenticated clients. The `logout` action appears with a `confirm` hint (§15.6) that the CLI uses to prompt the user before executing.

### 13.4 Logout Flow

The user logs out with `cli logout`:

1. The CLI finds the `logout` `Action` on the root `Representation` by matching `Action.Rel == "logout"`
2. `Hints["confirm"]` contains `"Are you sure you want to log out?"` — the CLI displays this message and waits for user confirmation before proceeding
3. The CLI submits `POST` to the logout target (`/auth/logout`) with the stored credential in the `Authorization` header
4. On success, the CLI removes the stored credential from `~/.config/cli/credentials.json`
5. The CLI re-fetches the root representation without credentials, returning to the unauthenticated state

```
$ cli logout
Are you sure you want to log out? [y/N] y
Logged out successfully.
```

### 13.5 Token Refresh and Expiry

#### 401 Responses

When a stored token expires, the server returns `401 Unauthorized`. The CLI detects this and informs the user. The 401 response body can itself be a `Representation` with a `login` action, guiding the user back to authentication:

```go
unauthorized := hyper.Representation{
    Kind: "error",
    State: hyper.Object{
        "message": hyper.Scalar{V: "Token expired"},
    },
    Actions: []hyper.Action{
        {
            Name:   "Login",
            Rel:    "login",
            Method: "POST",
            Target: hyper.MustParseTarget("/auth/login"),
            Consumes: []string{"application/vnd.api+json"},
            Fields: []hyper.Field{
                {Name: "username", Type: "text", Label: "Username", Required: true},
                {Name: "password", Type: "password", Label: "Password", Required: true},
            },
        },
    },
}
```

The JSON wire format:

```json
{
  "kind": "error",
  "state": {
    "message": "Token expired"
  },
  "actions": [
    {
      "name": "Login",
      "rel": "login",
      "method": "POST",
      "href": "/auth/login",
      "consumes": ["application/vnd.api+json"],
      "fields": [
        {"name": "username", "type": "text", "label": "Username", "required": true},
        {"name": "password", "type": "password", "label": "Password", "required": true}
      ]
    }
  ]
}
```

The CLI displays the error and suggests re-login:

```
$ cli contacts
Error 401: Token expired
  Session expired. Run "cli login" to re-authenticate.
```

#### Token Refresh

If the server provides a `refresh` action on the auth-token representation, the CLI can automatically refresh before expiry:

```go
loginResult := hyper.Representation{
    Kind: "auth-token",
    State: hyper.Object{
        "token":      hyper.Scalar{V: "eyJhbGci..."},
        "expires_at": hyper.Scalar{V: "2026-04-01T00:00:00Z"},
    },
    Links: []hyper.Link{
        {Rel: "root", Target: hyper.Path(), Title: "Home"},
    },
    Actions: []hyper.Action{
        {
            Name:   "Refresh Token",
            Rel:    "refresh",
            Method: "POST",
            Target: hyper.MustParseTarget("/auth/refresh"),
        },
    },
}
```

When the CLI detects that `expires_at` is approaching, it submits the `refresh` action automatically. The server returns a new auth-token `Representation` with an updated token and expiry. The CLI stores the new token and continues without interrupting the user.

### 13.6 Credential Storage

The CLI manages credentials locally with the following conventions:

- Credentials are stored per base URL in `~/.config/cli/credentials.json`, allowing the user to be authenticated against multiple servers simultaneously
- `Field.Type: "password"` (§7.3.1) signals the CLI to prompt interactively with masked input when the value is not provided as a flag — this prevents passwords from appearing in shell history
- The `--token` flag allows passing a bearer token directly for scripting and CI/CD use cases, bypassing the interactive login flow: `cli --token eyJhbGci... contacts`
- Stored tokens are included in all subsequent requests as `Authorization: Bearer <token>` headers

### 13.7 Interactive Mode Auth

The interactive REPL reflects auth state changes in real time. When the user logs in, the available commands update immediately:

```
$ cli --base http://localhost:8080/
Connected to http://localhost:8080/ (not authenticated)

root> help
Available commands:
  login     Log in (--username USERNAME --password PASSWORD)

root> login --username ada --password secret
Logged in as ada.

root> help
Available commands:
  contacts    Contacts
  profile     My Profile
  search      Search (--q QUERY)
  logout      Log out
```

The REPL re-fetches the root representation after login and rebuilds the command tree. The same happens after logout — the command tree collapses back to just `login`. This is the same discovery mechanism described in [Section 2](#2-discovery-flow) and [Section 10](#10-interactive-mode), applied to auth state transitions.

## 14. Representations and Types Exercised

This use case exercises the following `hyper` types:

| Type | Role in This Use Case |
|---|---|
| `Representation` | Every server response — root, list, single contact, nested note list, single note, search results, errors |
| `Link` | Navigation and subcommand discovery (`contacts`, `root`, `self`, `notes`, `tags`, `contact`). Recursive `Link` following drives nested subcommands — the CLI follows `Links` at each level to build an arbitrarily deep command tree |
| `Action` | All mutations (`create`, `update`, `delete`) and parameterized queries (`search`). Actions on nested resources (e.g., "Add Note", "Edit Note") work identically to top-level actions |
| `Field` | Flag generation, validation, completion candidates, default values |
| `Option` | Enum completion candidates (not shown in contacts but supported for `select` fields) |
| `Target` with `URL` | Resolved URLs from JSON codec (§13.3.6) |
| `Embedded` representations | Collection items in list and search results (§6.1). `Embedded` representations within nested resources carry their own full hypermedia controls — `Links`, `Actions`, and `Self` targets — enabling recursive navigation at every level |
| `Node` (`Object`) | Contact and note state as key-value pairs |
| `Value` (`Scalar`, `RichText`) | Primitive fields, bio content, and note body content |
| JSON `RepresentationCodec` | Primary codec for all CLI communication (§13) |
| `Action.Hints` | CLI-specific keys: `confirm`, `destructive`, `hidden` (§15.6) |
| `Action.Consumes` | Determines request body content type — `application/vnd.api+json` for hypermedia JSON submissions, `application/x-www-form-urlencoded` for form data, `multipart/form-data` for file uploads |
| `Action.Produces` | Hints at expected response content type — helps the CLI anticipate response format |
| `Field.Type: "password"` | Masked interactive input — the CLI prompts without echoing characters when this field type is encountered |
| `Meta` | Could carry pagination info, total counts (gap identified below) |

## 15. Alternative: Non-CRUD Operations as Resources

The contacts scenario so far covers standard CRUD — create, update, delete, list, show. Real applications also need operations like archiving, merging, and exporting. Rather than introducing a new pattern, this section explores modelling every operation as a standard CRUD action on a new resource type. The philosophy is "everything is a resource": an archive is not a verb applied to a contact, but a first-class entity you create, list, view, and delete. This approach requires no new `Action` patterns — every operation is a `POST` to create a new resource, discovered through `Links` on existing `Representations`.

### 15.1 Discovering Operation Resources

The contacts list `Representation` gains `Links` to the sub-resource collections for archives, merges, and exports. The CLI discovers these as navigable subcommands through the standard link-following algorithm (see [Section 7](#7-nested-subcommands-and-deep-navigation)).

#### 15.1.1 Enhanced Contacts List (Server Side)

```go
list := hyper.Representation{
    Kind: "contact-list",
    Self: hyper.MustParseTarget("/contacts").Ptr(),
    Links: []hyper.Link{
        {Rel: "root", Target: hyper.Path(), Title: "Home"},
        {Rel: "archives", Target: hyper.MustParseTarget("/contacts/archives"), Title: "Archives"},
        {Rel: "merges", Target: hyper.MustParseTarget("/contacts/merges"), Title: "Merges"},
        {Rel: "exports", Target: hyper.MustParseTarget("/contacts/exports"), Title: "Exports"},
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
    // Embedded items omitted for brevity — same as Section 5.2
}
```

#### 15.1.2 Enhanced Contacts List (JSON Wire Format)

```json
{
  "kind": "contact-list",
  "self": {"href": "/contacts"},
  "links": [
    {"rel": "root", "href": "/", "title": "Home"},
    {"rel": "archives", "href": "/contacts/archives", "title": "Archives"},
    {"rel": "merges", "href": "/contacts/merges", "title": "Merges"},
    {"rel": "exports", "href": "/contacts/exports", "title": "Exports"}
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
        {"name": "email", "type": "email", "label": "Email", "required": true},
        {"name": "phone", "type": "tel", "label": "Phone"}
      ]
    }
  ]
}
```

### 15.2 Archive as a Resource

An archive is a first-class resource. Creating an archive moves the specified contacts into an archived state as a side effect. The archive resource is browsable, and deleting it restores the contacts — making the operation undo-able.

#### 15.2.1 Archives Collection (Server Side)

```go
archives := hyper.Representation{
    Kind: "archive-list",
    Self: hyper.MustParseTarget("/contacts/archives").Ptr(),
    Links: []hyper.Link{
        {Rel: "contacts", Target: hyper.MustParseTarget("/contacts"), Title: "Contacts"},
    },
    Actions: []hyper.Action{
        {
            Name:     "Archive Contacts",
            Rel:      "create",
            Method:   "POST",
            Target:   hyper.MustParseTarget("/contacts/archives"),
            Consumes: []string{"application/vnd.api+json"},
            Fields: []hyper.Field{
                {
                    Name:     "contacts",
                    Type:     "text",
                    Label:    "Contact IDs",
                    Help:     "Comma-separated list of contact IDs to archive",
                    Required: true,
                },
            },
        },
    },
    Embedded: map[string][]hyper.Representation{
        "items": {
            {
                Kind: "archive",
                Self: hyper.MustParseTarget("/contacts/archives/1").Ptr(),
                State: hyper.Object{
                    "id":         hyper.Scalar{V: 1},
                    "created_at": hyper.Scalar{V: "2025-06-15T10:30:00Z"},
                    "contacts":   hyper.Scalar{V: "Ada Lovelace, Grace Hopper"},
                },
                Links: []hyper.Link{
                    {Rel: "self", Target: hyper.MustParseTarget("/contacts/archives/1"), Title: "Archive #1"},
                },
            },
        },
    },
}
```

#### 15.2.2 Archives Collection (JSON Wire Format)

```json
{
  "kind": "archive-list",
  "self": {"href": "/contacts/archives"},
  "links": [
    {"rel": "contacts", "href": "/contacts", "title": "Contacts"}
  ],
  "actions": [
    {
      "name": "Archive Contacts",
      "rel": "create",
      "method": "POST",
      "href": "/contacts/archives",
      "consumes": ["application/vnd.api+json"],
      "fields": [
        {
          "name": "contacts",
          "type": "text",
          "label": "Contact IDs",
          "help": "Comma-separated list of contact IDs to archive",
          "required": true
        }
      ]
    }
  ],
  "embedded": {
    "items": [
      {
        "kind": "archive",
        "self": {"href": "/contacts/archives/1"},
        "state": {
          "id": 1,
          "created_at": "2025-06-15T10:30:00Z",
          "contacts": "Ada Lovelace, Grace Hopper"
        },
        "links": [
          {"rel": "self", "href": "/contacts/archives/1", "title": "Archive #1"}
        ]
      }
    ]
  }
}
```

#### 15.2.3 Single Archive with Undo (Server Side)

The individual archive resource carries a `delete` `Action` with a `confirm` hint (§15.6). Deleting the archive restores the archived contacts to their previous state.

```go
archive := hyper.Representation{
    Kind: "archive",
    Self: hyper.MustParseTarget("/contacts/archives/1").Ptr(),
    State: hyper.Object{
        "id":          hyper.Scalar{V: 1},
        "created_at":  hyper.Scalar{V: "2025-06-15T10:30:00Z"},
        "contact_ids": hyper.Scalar{V: []int{1, 2}},
        "contacts":    hyper.Scalar{V: "Ada Lovelace, Grace Hopper"},
    },
    Links: []hyper.Link{
        {Rel: "archives", Target: hyper.MustParseTarget("/contacts/archives"), Title: "All Archives"},
        {Rel: "contacts", Target: hyper.MustParseTarget("/contacts"), Title: "Contacts"},
    },
    Actions: []hyper.Action{
        {
            Name:   "Restore Contacts",
            Rel:    "delete",
            Method: "DELETE",
            Target: hyper.MustParseTarget("/contacts/archives/1"),
            Hints: map[string]any{
                "confirm": "This will restore 2 contacts and remove this archive. Continue?",
            },
        },
    },
}
```

#### 15.2.4 CLI Interaction

```
$ cli contacts archives
Archives
┌────┬─────────────────────┬────────────────────────────┐
│ ID │ Created             │ Contacts                   │
├────┼─────────────────────┼────────────────────────────┤
│  1 │ 2025-06-15T10:30:00 │ Ada Lovelace, Grace Hopper │
└────┴─────────────────────┴────────────────────────────┘

Available actions:
  create    Archive contacts (cli contacts archives create --contacts CONTACTS)

Navigate to an archive:
  cli contacts archives show <id>

$ cli contacts archives create --contacts 1,2
Archive created.

  ID:       2
  Contacts: Ada Lovelace, Grace Hopper

$ cli contacts archives show 1
Archive #1
  ID:          1
  Created:     2025-06-15T10:30:00Z
  Contact IDs: [1, 2]
  Contacts:    Ada Lovelace, Grace Hopper

Available actions:
  delete    Restore contacts (cli contacts archives delete 1) [confirm]

$ cli contacts archives delete 1
? This will restore 2 contacts and remove this archive. Continue? (y/N) y
Archive deleted. Contacts restored.
```

### 15.3 Merge as a Resource

A merge records which contacts were combined and what the result was. The merge resource is immutable — it has no `update` or `delete` `Actions` — and serves as an audit trail.

#### 15.3.1 Merges Collection (Server Side)

```go
merges := hyper.Representation{
    Kind: "merge-list",
    Self: hyper.MustParseTarget("/contacts/merges").Ptr(),
    Links: []hyper.Link{
        {Rel: "contacts", Target: hyper.MustParseTarget("/contacts"), Title: "Contacts"},
    },
    Actions: []hyper.Action{
        {
            Name:     "Merge Contacts",
            Rel:      "create",
            Method:   "POST",
            Target:   hyper.MustParseTarget("/contacts/merges"),
            Consumes: []string{"application/vnd.api+json"},
            Fields: []hyper.Field{
                {
                    Name:     "sources",
                    Type:     "text",
                    Label:    "Source Contact IDs",
                    Help:     "Comma-separated IDs of contacts to merge into the target",
                    Required: true,
                },
                {
                    Name:     "target",
                    Type:     "text",
                    Label:    "Target Contact ID",
                    Help:     "The contact that absorbs the others",
                    Required: true,
                },
            },
        },
    },
    Embedded: map[string][]hyper.Representation{
        "items": {
            {
                Kind: "merge",
                Self: hyper.MustParseTarget("/contacts/merges/1").Ptr(),
                State: hyper.Object{
                    "id":         hyper.Scalar{V: 1},
                    "created_at": hyper.Scalar{V: "2025-07-01T14:00:00Z"},
                    "target":     hyper.Scalar{V: "Ada Lovelace (#1)"},
                    "sources":    hyper.Scalar{V: "Grace Hopper (#2), Charles Babbage (#3)"},
                },
                Links: []hyper.Link{
                    {Rel: "self", Target: hyper.MustParseTarget("/contacts/merges/1"), Title: "Merge #1"},
                },
            },
        },
    },
}
```

#### 15.3.2 Merges Collection (JSON Wire Format)

```json
{
  "kind": "merge-list",
  "self": {"href": "/contacts/merges"},
  "links": [
    {"rel": "contacts", "href": "/contacts", "title": "Contacts"}
  ],
  "actions": [
    {
      "name": "Merge Contacts",
      "rel": "create",
      "method": "POST",
      "href": "/contacts/merges",
      "consumes": ["application/vnd.api+json"],
      "fields": [
        {
          "name": "sources",
          "type": "text",
          "label": "Source Contact IDs",
          "help": "Comma-separated IDs of contacts to merge into the target",
          "required": true
        },
        {
          "name": "target",
          "type": "text",
          "label": "Target Contact ID",
          "help": "The contact that absorbs the others",
          "required": true
        }
      ]
    }
  ],
  "embedded": {
    "items": [
      {
        "kind": "merge",
        "self": {"href": "/contacts/merges/1"},
        "state": {
          "id": 1,
          "created_at": "2025-07-01T14:00:00Z",
          "target": "Ada Lovelace (#1)",
          "sources": "Grace Hopper (#2), Charles Babbage (#3)"
        },
        "links": [
          {"rel": "self", "href": "/contacts/merges/1", "title": "Merge #1"}
        ]
      }
    ]
  }
}
```

#### 15.3.3 Single Merge Resource (Server Side)

The merge resource provides a detailed audit record. It has no mutation `Actions` — it is read-only by design.

```go
merge := hyper.Representation{
    Kind: "merge",
    Self: hyper.MustParseTarget("/contacts/merges/1").Ptr(),
    State: hyper.Object{
        "id":         hyper.Scalar{V: 1},
        "created_at": hyper.Scalar{V: "2025-07-01T14:00:00Z"},
        "target_id":  hyper.Scalar{V: 1},
        "source_ids": hyper.Scalar{V: []int{2, 3}},
        "result": hyper.Object{
            "kept_name":    hyper.Scalar{V: "Ada Lovelace"},
            "kept_email":   hyper.Scalar{V: "ada@example.com"},
            "merged_phone": hyper.Scalar{V: "+1-555-0200 (from Grace Hopper)"},
        },
    },
    Links: []hyper.Link{
        {Rel: "merges", Target: hyper.MustParseTarget("/contacts/merges"), Title: "All Merges"},
        {Rel: "target", Target: hyper.MustParseTarget("/contacts/1"), Title: "Ada Lovelace"},
    },
}
```

#### 15.3.4 CLI Interaction

```
$ cli contacts merges create --sources 2,3 --target 1
Merge created.

  ID:      1
  Target:  Ada Lovelace (#1)
  Sources: Grace Hopper (#2), Charles Babbage (#3)

$ cli contacts merges
Merges
┌────┬─────────────────────┬──────────────────┬───────────────────────────────────────────┐
│ ID │ Created             │ Target           │ Sources                                   │
├────┼─────────────────────┼──────────────────┼───────────────────────────────────────────┤
│  1 │ 2025-07-01T14:00:00 │ Ada Lovelace (#1)│ Grace Hopper (#2), Charles Babbage (#3)   │
└────┴─────────────────────┴──────────────────┴───────────────────────────────────────────┘

Available actions:
  create    Merge contacts (cli contacts merges create --sources SOURCES --target TARGET)

Navigate to a merge:
  cli contacts merges show <id>

$ cli contacts merges show 1
Merge #1
  ID:         1
  Created:    2025-07-01T14:00:00Z
  Target ID:  1
  Source IDs: [2, 3]
  Result:
    Kept Name:    Ada Lovelace
    Kept Email:   ada@example.com
    Merged Phone: +1-555-0200 (from Grace Hopper)

Navigate:
  merges    All Merges (cli contacts merges)
  target    Ada Lovelace (cli contacts show 1)
```

Note the absence of `update` or `delete` actions — the CLI shows no mutation commands because the server provides none. The merge is an immutable audit record.

### 15.4 Export as a Resource

An export represents an asynchronous job. Creating an export starts the process; the export resource has state that transitions from `pending` to `complete`. Once complete, the resource gains a `Link` with `rel: "download"` pointing to the generated file.

#### 15.4.1 Exports Collection (Server Side)

```go
exports := hyper.Representation{
    Kind: "export-list",
    Self: hyper.MustParseTarget("/contacts/exports").Ptr(),
    Links: []hyper.Link{
        {Rel: "contacts", Target: hyper.MustParseTarget("/contacts"), Title: "Contacts"},
    },
    Actions: []hyper.Action{
        {
            Name:     "Export Contacts",
            Rel:      "create",
            Method:   "POST",
            Target:   hyper.MustParseTarget("/contacts/exports"),
            Consumes: []string{"application/vnd.api+json"},
            Fields: []hyper.Field{
                {
                    Name:     "format",
                    Type:     "select",
                    Label:    "Format",
                    Required: true,
                    Options: []hyper.Option{
                        {Value: "csv", Label: "CSV"},
                        {Value: "vcf", Label: "vCard"},
                        {Value: "json", Label: "JSON"},
                    },
                },
                {
                    Name:  "query",
                    Type:  "text",
                    Label: "Filter Query",
                    Help:  "Optional search query to filter exported contacts",
                },
            },
        },
    },
    Embedded: map[string][]hyper.Representation{
        "items": {
            {
                Kind: "export",
                Self: hyper.MustParseTarget("/contacts/exports/1").Ptr(),
                State: hyper.Object{
                    "id":     hyper.Scalar{V: 1},
                    "format": hyper.Scalar{V: "csv"},
                    "status": hyper.Scalar{V: "complete"},
                },
                Links: []hyper.Link{
                    {Rel: "self", Target: hyper.MustParseTarget("/contacts/exports/1"), Title: "Export #1"},
                },
            },
        },
    },
}
```

#### 15.4.2 Exports Collection (JSON Wire Format)

```json
{
  "kind": "export-list",
  "self": {"href": "/contacts/exports"},
  "links": [
    {"rel": "contacts", "href": "/contacts", "title": "Contacts"}
  ],
  "actions": [
    {
      "name": "Export Contacts",
      "rel": "create",
      "method": "POST",
      "href": "/contacts/exports",
      "consumes": ["application/vnd.api+json"],
      "fields": [
        {
          "name": "format",
          "type": "select",
          "label": "Format",
          "required": true,
          "options": [
            {"value": "csv", "label": "CSV"},
            {"value": "vcf", "label": "vCard"},
            {"value": "json", "label": "JSON"}
          ]
        },
        {
          "name": "query",
          "type": "text",
          "label": "Filter Query",
          "help": "Optional search query to filter exported contacts"
        }
      ]
    }
  ],
  "embedded": {
    "items": [
      {
        "kind": "export",
        "self": {"href": "/contacts/exports/1"},
        "state": {"id": 1, "format": "csv", "status": "complete"},
        "links": [
          {"rel": "self", "href": "/contacts/exports/1", "title": "Export #1"}
        ]
      }
    ]
  }
}
```

#### 15.4.3 Pending Export (Server Side)

When first created, the export is in `pending` state with no download link:

```go
pendingExport := hyper.Representation{
    Kind: "export",
    Self: hyper.MustParseTarget("/contacts/exports/2").Ptr(),
    State: hyper.Object{
        "id":         hyper.Scalar{V: 2},
        "format":     hyper.Scalar{V: "csv"},
        "status":     hyper.Scalar{V: "pending"},
        "created_at": hyper.Scalar{V: "2025-08-10T09:00:00Z"},
    },
    Links: []hyper.Link{
        {Rel: "exports", Target: hyper.MustParseTarget("/contacts/exports"), Title: "All Exports"},
    },
}
```

#### 15.4.4 Complete Export (Server Side)

Once processing finishes, the server adds a `download` `Link`:

```go
completeExport := hyper.Representation{
    Kind: "export",
    Self: hyper.MustParseTarget("/contacts/exports/2").Ptr(),
    State: hyper.Object{
        "id":           hyper.Scalar{V: 2},
        "format":       hyper.Scalar{V: "csv"},
        "status":       hyper.Scalar{V: "complete"},
        "created_at":   hyper.Scalar{V: "2025-08-10T09:00:00Z"},
        "completed_at": hyper.Scalar{V: "2025-08-10T09:00:04Z"},
        "record_count": hyper.Scalar{V: 142},
    },
    Links: []hyper.Link{
        {Rel: "exports", Target: hyper.MustParseTarget("/contacts/exports"), Title: "All Exports"},
        {Rel: "download", Target: hyper.MustParseTarget("/contacts/exports/2/file"), Title: "Download CSV"},
    },
}
```

#### 15.4.5 Complete Export (JSON Wire Format)

```json
{
  "kind": "export",
  "self": {"href": "/contacts/exports/2"},
  "state": {
    "id": 2,
    "format": "csv",
    "status": "complete",
    "created_at": "2025-08-10T09:00:00Z",
    "completed_at": "2025-08-10T09:00:04Z",
    "record_count": 142
  },
  "links": [
    {"rel": "exports", "href": "/contacts/exports", "title": "All Exports"},
    {"rel": "download", "href": "/contacts/exports/2/file", "title": "Download CSV"}
  ]
}
```

#### 15.4.6 CLI Interaction

```
$ cli contacts exports create --format csv
Export created.

  ID:     2
  Format: csv
  Status: pending

$ cli contacts exports show 2
Export #2
  ID:      2
  Format:  csv
  Status:  pending
  Created: 2025-08-10T09:00:00Z

Navigate:
  exports   All Exports (cli contacts exports)

$ cli contacts exports show 2
Export #2
  ID:           2
  Format:       csv
  Status:       complete
  Created:      2025-08-10T09:00:00Z
  Completed:    2025-08-10T09:00:04Z
  Record Count: 142

Navigate:
  exports    All Exports (cli contacts exports)
  download   Download CSV (cli contacts exports download 2)
```

The `download` `Link` only appears once the export is complete. The CLI's standard link-following renders it as a navigable subcommand — no special-casing needed. When the user follows the `download` link, the CLI fetches the file and writes it to stdout or a local file.

### 15.5 CLI Command Tree

With all three operation-as-resource sub-collections discovered, the full command tree looks like this:

```
cli
├── contacts
│   ├── show <id>
│   ├── create
│   ├── archives
│   │   ├── create
│   │   ├── show <id>
│   │   └── delete <id>   (restore/undo)
│   ├── merges
│   │   ├── create
│   │   └── show <id>
│   └── exports
│       ├── create
│       └── show <id>
└── search
```

Every command in this tree is discovered through the standard link-following and action-executing logic described in [Section 7](#7-nested-subcommands-and-deep-navigation) and [Section 8](#8-executing-actions). The CLI code requires zero changes to support archives, merges, and exports — the server adds `Links` and the CLI follows them.

### 15.6 Trade-offs

**Advantages:**

- **Fully RESTful.** Every operation follows the same create/read/delete pattern. No custom verbs, no RPC-style endpoints. The API is uniform.
- **Auditable.** Merges and archives are first-class resources with timestamps and details. Browsing `/contacts/merges` shows a complete history of merge operations without a separate audit log.
- **Undo-able.** Archive supports undo via `DELETE` — the resource metaphor naturally provides a handle to reverse the operation.
- **Discoverable via standard hypermedia.** The CLI's existing link-following and action-executing logic handles everything. No new client-side concepts are needed.
- **Evolvable.** Adding a new operation (e.g., "import") means adding a new `Link` to the contacts list `Representation`. The CLI discovers it automatically.

**Disadvantages:**

- **More resources to manage.** Three new collections, each with their own endpoints, representations, and storage. The API surface area grows.
- **More round-trips.** "Archive contacts 1 and 2" requires a `POST` to create the archive, then optionally a `GET` to confirm. Direct mutation (`POST /contacts/1/archive`) would be a single request.
- **Indirect mental model.** "Archive a contact" becomes "create an archive" — the user must think in terms of resource creation rather than direct action. This is natural for developers comfortable with REST but can feel unintuitive for others.
- **Forced resource metaphor.** Merges and exports feel more like processes or jobs than entities. A merge is a one-time transformation; modelling it as a persistent resource adds storage and complexity that may not be warranted unless audit is a requirement.
- **Async complexity for exports.** The export resource has state transitions (`pending` to `complete`) that the CLI must handle — polling or re-fetching to check status. The spec does not define conventions for async/job-like resources, so the polling behavior must be invented by the client.

## 16. Alternative: Non-CRUD Operations as Actions with History

Where [Section 15](#15-alternative-non-crud-operations-as-resources) models every operation as a new resource you create, this section explores the opposite emphasis: operations are `Actions` on the contact (or contact list) `Representation` itself, and each operation also writes to a separate, read-only history resource that serves as an audit trail. The user's mental model is "do something to a contact" (verb-oriented), not "create an archive object" (noun-oriented). The history resources exist for auditability but are not the primary interface for performing operations.

### 16.1 Archive as an Action with History

The single contact `Representation` (from [Section 6.2](#62-single-contact-server-side)) gains an `Action` with `rel: "archive"`. Submitting it archives the contact directly. A separate history collection at `/contacts/1/history` tracks lifecycle events.

#### 16.1.1 Contact with Archive Action (Server Side)

```go
contact := hyper.Representation{
    Kind: "contact",
    Self: hyper.MustParseTarget("/contacts/1").Ptr(),
    State: hyper.Object{
        "id":       hyper.Scalar{V: 1},
        "name":     hyper.Scalar{V: "Ada Lovelace"},
        "email":    hyper.Scalar{V: "ada@example.com"},
        "phone":    hyper.Scalar{V: "+1-555-0100"},
        "archived": hyper.Scalar{V: false},
    },
    Links: []hyper.Link{
        {Rel: "contacts", Target: hyper.MustParseTarget("/contacts"), Title: "All Contacts"},
        {Rel: "history", Target: hyper.MustParseTarget("/contacts/1/history"), Title: "History"},
    },
    Actions: []hyper.Action{
        {
            Name:     "Update Contact",
            Rel:      "update",
            Method:   "PUT",
            Target:   hyper.MustParseTarget("/contacts/1"),
            Consumes: []string{"application/vnd.api+json"},
            Fields: []hyper.Field{
                {Name: "name", Type: "text", Label: "Name", Value: "Ada Lovelace", Required: true},
                {Name: "email", Type: "email", Label: "Email", Value: "ada@example.com", Required: true},
                {Name: "phone", Type: "tel", Label: "Phone", Value: "+1-555-0100"},
            },
        },
        {
            Name:   "Delete Contact",
            Rel:    "delete",
            Method: "DELETE",
            Target: hyper.MustParseTarget("/contacts/1"),
            Hints: map[string]any{
                "confirm":     "Are you sure you want to delete Ada Lovelace?",
                "destructive": true,
            },
        },
        {
            Name:   "Archive Contact",
            Rel:    "archive",
            Method: "POST",
            Target: hyper.MustParseTarget("/contacts/1/archive"),
            Hints: map[string]any{
                "confirm": "Are you sure you want to archive Ada Lovelace?",
            },
        },
    },
}
```

#### 16.1.2 Contact with Archive Action (JSON Wire Format)

```json
{
  "kind": "contact",
  "self": {"href": "/contacts/1"},
  "state": {
    "id": 1,
    "name": "Ada Lovelace",
    "email": "ada@example.com",
    "phone": "+1-555-0100",
    "archived": false
  },
  "links": [
    {"rel": "contacts", "href": "/contacts", "title": "All Contacts"},
    {"rel": "history", "href": "/contacts/1/history", "title": "History"}
  ],
  "actions": [
    {
      "name": "Update Contact",
      "rel": "update",
      "method": "PUT",
      "href": "/contacts/1",
      "consumes": ["application/vnd.api+json"],
      "fields": [
        {"name": "name", "type": "text", "label": "Name", "value": "Ada Lovelace", "required": true},
        {"name": "email", "type": "email", "label": "Email", "value": "ada@example.com", "required": true},
        {"name": "phone", "type": "tel", "label": "Phone", "value": "+1-555-0100"}
      ]
    },
    {
      "name": "Delete Contact",
      "rel": "delete",
      "method": "DELETE",
      "href": "/contacts/1",
      "hints": {
        "confirm": "Are you sure you want to delete Ada Lovelace?",
        "destructive": true
      }
    },
    {
      "name": "Archive Contact",
      "rel": "archive",
      "method": "POST",
      "href": "/contacts/1/archive",
      "hints": {
        "confirm": "Are you sure you want to archive Ada Lovelace?"
      }
    }
  ]
}
```

#### 16.1.3 CLI Interaction: Archiving

The CLI maps the `archive` `Action` to a direct verb command. Since `Action.Rel` is `"archive"` (not one of the standard CRUD rels), the CLI surfaces it as a named subcommand on the contact.

```
$ cli contacts show 1
Ada Lovelace
  ID:       1
  Email:    ada@example.com
  Phone:    +1-555-0100
  Archived: false

Available actions:
  update    Update contact (cli contacts update 1 --name NAME --email EMAIL)
  delete    Delete contact (cli contacts delete 1) [confirm] [destructive]
  archive   Archive contact (cli contacts archive 1) [confirm]

Navigate:
  contacts  All Contacts (cli contacts)
  history   History (cli contacts history 1)

$ cli contacts archive 1
? Are you sure you want to archive Ada Lovelace? (y/N) y
Contact archived.

  ID:       1
  Name:     Ada Lovelace
  Archived: true
```

#### 16.1.4 Archived Contact Representation (Server Side)

After archiving, the contact's `Representation` changes: `"archived"` becomes `true`, the `archive` `Action` is replaced by an `unarchive` `Action`, and the `update` and `delete` `Actions` are removed (the server no longer offers them for archived contacts).

```go
archivedContact := hyper.Representation{
    Kind: "contact",
    Self: hyper.MustParseTarget("/contacts/1").Ptr(),
    State: hyper.Object{
        "id":       hyper.Scalar{V: 1},
        "name":     hyper.Scalar{V: "Ada Lovelace"},
        "email":    hyper.Scalar{V: "ada@example.com"},
        "phone":    hyper.Scalar{V: "+1-555-0100"},
        "archived": hyper.Scalar{V: true},
    },
    Links: []hyper.Link{
        {Rel: "contacts", Target: hyper.MustParseTarget("/contacts"), Title: "All Contacts"},
        {Rel: "history", Target: hyper.MustParseTarget("/contacts/1/history"), Title: "History"},
    },
    Actions: []hyper.Action{
        {
            Name:   "Unarchive Contact",
            Rel:    "unarchive",
            Method: "POST",
            Target: hyper.MustParseTarget("/contacts/1/unarchive"),
        },
    },
}
```

#### 16.1.5 Archived Contact (JSON Wire Format)

```json
{
  "kind": "contact",
  "self": {"href": "/contacts/1"},
  "state": {
    "id": 1,
    "name": "Ada Lovelace",
    "email": "ada@example.com",
    "phone": "+1-555-0100",
    "archived": true
  },
  "links": [
    {"rel": "contacts", "href": "/contacts", "title": "All Contacts"},
    {"rel": "history", "href": "/contacts/1/history", "title": "History"}
  ],
  "actions": [
    {
      "name": "Unarchive Contact",
      "rel": "unarchive",
      "method": "POST",
      "href": "/contacts/1/unarchive"
    }
  ]
}
```

#### 16.1.6 Contact History (Server Side)

The `Link` with `rel: "history"` on the contact points to a read-only collection of timestamped lifecycle events. The history `Representation` has no mutation `Actions` — it is an audit trail.

```go
history := hyper.Representation{
    Kind: "history",
    Self: hyper.MustParseTarget("/contacts/1/history").Ptr(),
    Links: []hyper.Link{
        {Rel: "contact", Target: hyper.MustParseTarget("/contacts/1"), Title: "Ada Lovelace"},
    },
    Embedded: map[string][]hyper.Representation{
        "items": {
            {
                Kind: "history-entry",
                State: hyper.Object{
                    "event": hyper.Scalar{V: "created"},
                    "at":    hyper.Scalar{V: "2025-01-10T08:00:00Z"},
                    "by":    hyper.Scalar{V: "user@example.com"},
                },
            },
            {
                Kind: "history-entry",
                State: hyper.Object{
                    "event": hyper.Scalar{V: "archived"},
                    "at":    hyper.Scalar{V: "2025-01-15T10:30:00Z"},
                    "by":    hyper.Scalar{V: "user@example.com"},
                },
            },
        },
    },
}
```

#### 16.1.7 Contact History (JSON Wire Format)

```json
{
  "kind": "history",
  "self": {"href": "/contacts/1/history"},
  "links": [
    {"rel": "contact", "href": "/contacts/1", "title": "Ada Lovelace"}
  ],
  "embedded": {
    "items": [
      {
        "kind": "history-entry",
        "state": {
          "event": "created",
          "at": "2025-01-10T08:00:00Z",
          "by": "user@example.com"
        }
      },
      {
        "kind": "history-entry",
        "state": {
          "event": "archived",
          "at": "2025-01-15T10:30:00Z",
          "by": "user@example.com"
        }
      }
    ]
  }
}
```

#### 16.1.8 CLI Interaction: Viewing History

```
$ cli contacts history 1
History for Ada Lovelace
┌──────────┬──────────────────────┬──────────────────┐
│ Event    │ At                   │ By               │
├──────────┼──────────────────────┼──────────────────┤
│ created  │ 2025-01-10T08:00:00Z │ user@example.com │
│ archived │ 2025-01-15T10:30:00Z │ user@example.com │
└──────────┴──────────────────────┴──────────────────┘

Navigate:
  contact   Ada Lovelace (cli contacts show 1)
```

### 16.2 Merge as an Action with History

Merge operates across contacts, so the `Action` lives on the contact list `Representation` rather than on a single contact. The merge action accepts source IDs and a target ID.

#### 16.2.1 Contact List with Merge Action (Server Side)

The contacts list `Representation` (from [Section 5.2](#52-contacts-list-server-side)) gains `Actions` with `rel: "merge"` and `rel: "export"`, plus a `Link` to the exports history.

```go
list := hyper.Representation{
    Kind: "contact-list",
    Self: hyper.MustParseTarget("/contacts").Ptr(),
    Links: []hyper.Link{
        {Rel: "root", Target: hyper.Path(), Title: "Home"},
        {Rel: "exports", Target: hyper.MustParseTarget("/contacts/exports"), Title: "Export History"},
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
        {
            Name:     "Merge Contacts",
            Rel:      "merge",
            Method:   "POST",
            Target:   hyper.MustParseTarget("/contacts/merge"),
            Consumes: []string{"application/vnd.api+json"},
            Fields: []hyper.Field{
                {
                    Name:     "sources",
                    Type:     "text",
                    Label:    "Source Contact IDs",
                    Help:     "Comma-separated IDs of contacts to merge into the target",
                    Required: true,
                },
                {
                    Name:     "target",
                    Type:     "text",
                    Label:    "Target Contact ID",
                    Help:     "The contact that absorbs the others",
                    Required: true,
                },
            },
        },
        {
            Name:     "Export Contacts",
            Rel:      "export",
            Method:   "POST",
            Target:   hyper.MustParseTarget("/contacts/export"),
            Consumes: []string{"application/vnd.api+json"},
            Fields: []hyper.Field{
                {
                    Name:     "format",
                    Type:     "select",
                    Label:    "Format",
                    Required: true,
                    Options: []hyper.Option{
                        {Value: "csv", Label: "CSV"},
                        {Value: "vcf", Label: "vCard"},
                        {Value: "json", Label: "JSON"},
                    },
                },
            },
            Hints: map[string]any{
                "async": true,
            },
        },
    },
    // Embedded items omitted for brevity — same as Section 5.2
}
```

#### 16.2.2 Contact List with Actions (JSON Wire Format)

```json
{
  "kind": "contact-list",
  "self": {"href": "/contacts"},
  "links": [
    {"rel": "root", "href": "/", "title": "Home"},
    {"rel": "exports", "href": "/contacts/exports", "title": "Export History"}
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
        {"name": "email", "type": "email", "label": "Email", "required": true},
        {"name": "phone", "type": "tel", "label": "Phone"}
      ]
    },
    {
      "name": "Merge Contacts",
      "rel": "merge",
      "method": "POST",
      "href": "/contacts/merge",
      "consumes": ["application/vnd.api+json"],
      "fields": [
        {
          "name": "sources",
          "type": "text",
          "label": "Source Contact IDs",
          "help": "Comma-separated IDs of contacts to merge into the target",
          "required": true
        },
        {
          "name": "target",
          "type": "text",
          "label": "Target Contact ID",
          "help": "The contact that absorbs the others",
          "required": true
        }
      ]
    },
    {
      "name": "Export Contacts",
      "rel": "export",
      "method": "POST",
      "href": "/contacts/export",
      "consumes": ["application/vnd.api+json"],
      "fields": [
        {
          "name": "format",
          "type": "select",
          "label": "Format",
          "required": true,
          "options": [
            {"value": "csv", "label": "CSV"},
            {"value": "vcf", "label": "vCard"},
            {"value": "json", "label": "JSON"}
          ]
        }
      ],
      "hints": {"async": true}
    }
  ]
}
```

#### 16.2.3 CLI Interaction: Merging

```
$ cli contacts merge --sources 2,3 --target 1
Contacts merged.

  Target: Ada Lovelace (#1)
  Merged: Grace Hopper (#2), Charles Babbage (#3)
  Changes:
    phone: added +1-555-0200 (from Grace Hopper)
    notes: combined 3 notes from 2 sources
```

The response is the merged contact's `Representation`, which the server returns directly from the `POST /contacts/merge` endpoint. The merge also writes an entry to the target contact's history.

#### 16.2.4 Merge History Entry

After the merge, the target contact's history at `/contacts/1/history` gains a merge entry:

```json
{
  "kind": "history-entry",
  "state": {
    "event": "merged",
    "at": "2025-07-01T14:00:00Z",
    "by": "user@example.com",
    "details": {
      "sources": [2, 3],
      "changes": {
        "phone": "added +1-555-0200 (from Grace Hopper)",
        "notes": "combined 3 notes from 2 sources"
      }
    }
  }
}
```

The history entry records what was merged and what changed, providing the same auditability as the resource-based approach in [Section 15.3](#153-merge-as-a-resource), but as a read-only log entry rather than a first-class resource.

### 16.3 Export as an Action with History

Export is an `Action` on the contact list `Representation`. Since export is asynchronous, the action signals this via `Hints: {"async": true}` (per spec §7.2). The action's response returns a `Representation` of the running export job with `Meta.poll-interval` guiding client polling behavior. The history of past exports lives at `/contacts/exports` as a read-only log.

#### 16.3.1 CLI Interaction: Exporting

The `export` `Action` is discovered on the contact list (see [Section 16.2.1](#1621-contact-list-with-merge-action-server-side)). The CLI maps `Action.Rel: "export"` to a subcommand. Because the action's `Hints` include `"async": true`, the CLI detects that the response will be an async job and automatically polls for completion.

```
$ cli contacts export --format csv
Exporting contacts...
⠋ Status: pending (polling every 2s)
⠙ Status: processing (35%)
⠹ Status: processing (78%)
✓ Export complete (142 records)

Navigate:
  exports    All Exports (cli contacts exports)
  download   Download CSV (cli contacts exports download 1)
```

The CLI detects `"async": true` on the action's hints and, upon receiving the job response, reads `meta.poll-interval` to determine the polling frequency. It re-fetches the job's `Self` URL until status transitions to `"complete"` or `"failed"`. Progress percentage comes from the optional `"progress"` key in the job's `State`.

Without the `"async"` hint, the CLI would display the job representation as a static result (as shown in the manual polling example below).

**Manual polling fallback:** If the user prefers not to wait, `--no-poll` skips auto-polling:

```
$ cli contacts export --format csv --no-poll
Export started.

  ID:     1
  Format: csv
  Status: pending

Check status:
  cli contacts exports show 1
```

The `POST /contacts/export` response is an `export-job` `Representation`:

#### 16.3.2 Pending Export Job (Server Side)

```go
pendingJob := hyper.Representation{
    Kind: "export-job",
    Self: hyper.MustParseTarget("/contacts/exports/1").Ptr(),
    State: hyper.Object{
        "id":         hyper.Scalar{V: 1},
        "format":     hyper.Scalar{V: "csv"},
        "status":     hyper.Scalar{V: "pending"},
        "created_at": hyper.Scalar{V: "2025-08-10T09:00:00Z"},
    },
    Links: []hyper.Link{
        {Rel: "exports", Target: hyper.MustParseTarget("/contacts/exports"), Title: "All Exports"},
    },
    Meta: map[string]any{
        "poll-interval": 2,
    },
}
```

#### 16.3.3 Pending Export Job (JSON Wire Format)

```json
{
  "kind": "export-job",
  "self": {"href": "/contacts/exports/1"},
  "state": {
    "id": 1,
    "format": "csv",
    "status": "pending",
    "created_at": "2025-08-10T09:00:00Z"
  },
  "links": [
    {"rel": "exports", "href": "/contacts/exports", "title": "All Exports"}
  ],
  "meta": {
    "poll-interval": 2
  }
}
```

#### 16.3.4 Complete Export Job (Server Side)

Once processing finishes, the job gains a `download` `Link` and updated state:

```go
completeJob := hyper.Representation{
    Kind: "export-job",
    Self: hyper.MustParseTarget("/contacts/exports/1").Ptr(),
    State: hyper.Object{
        "id":           hyper.Scalar{V: 1},
        "format":       hyper.Scalar{V: "csv"},
        "status":       hyper.Scalar{V: "complete"},
        "created_at":   hyper.Scalar{V: "2025-08-10T09:00:00Z"},
        "completed_at": hyper.Scalar{V: "2025-08-10T09:00:04Z"},
        "record_count": hyper.Scalar{V: 142},
    },
    Links: []hyper.Link{
        {Rel: "exports", Target: hyper.MustParseTarget("/contacts/exports"), Title: "All Exports"},
        {Rel: "download", Target: hyper.MustParseTarget("/contacts/exports/1/file"), Title: "Download CSV"},
    },
}
```

#### 16.3.5 Complete Export Job (JSON Wire Format)

```json
{
  "kind": "export-job",
  "self": {"href": "/contacts/exports/1"},
  "state": {
    "id": 1,
    "format": "csv",
    "status": "complete",
    "created_at": "2025-08-10T09:00:00Z",
    "completed_at": "2025-08-10T09:00:04Z",
    "record_count": 142
  },
  "links": [
    {"rel": "exports", "href": "/contacts/exports", "title": "All Exports"},
    {"rel": "download", "href": "/contacts/exports/1/file", "title": "Download CSV"}
  ]
}
```

#### 16.3.6 CLI Interaction: Checking Export Status

```
$ cli contacts exports show 1
Export #1
  ID:           1
  Format:       csv
  Status:       complete
  Created:      2025-08-10T09:00:00Z
  Completed:    2025-08-10T09:00:04Z
  Record Count: 142

Navigate:
  exports    All Exports (cli contacts exports)
  download   Download CSV (cli contacts exports download 1)
```

#### 16.3.7 Export History Collection

The `/contacts/exports` collection is a read-only log of all past exports. It has no `create` `Action` — exports are initiated via the `export` `Action` on the contact list, not by creating export resources directly.

```go
exportHistory := hyper.Representation{
    Kind: "export-history",
    Self: hyper.MustParseTarget("/contacts/exports").Ptr(),
    Links: []hyper.Link{
        {Rel: "contacts", Target: hyper.MustParseTarget("/contacts"), Title: "Contacts"},
    },
    Embedded: map[string][]hyper.Representation{
        "items": {
            {
                Kind: "export-job",
                Self: hyper.MustParseTarget("/contacts/exports/1").Ptr(),
                State: hyper.Object{
                    "id":     hyper.Scalar{V: 1},
                    "format": hyper.Scalar{V: "csv"},
                    "status": hyper.Scalar{V: "complete"},
                },
                Links: []hyper.Link{
                    {Rel: "self", Target: hyper.MustParseTarget("/contacts/exports/1"), Title: "Export #1"},
                },
            },
        },
    },
}
```

#### 16.3.8 Export History (JSON Wire Format)

```json
{
  "kind": "export-history",
  "self": {"href": "/contacts/exports"},
  "links": [
    {"rel": "contacts", "href": "/contacts", "title": "Contacts"}
  ],
  "embedded": {
    "items": [
      {
        "kind": "export-job",
        "self": {"href": "/contacts/exports/1"},
        "state": {"id": 1, "format": "csv", "status": "complete"},
        "links": [
          {"rel": "self", "href": "/contacts/exports/1", "title": "Export #1"}
        ]
      }
    ]
  }
}
```

#### 16.3.9 CLI Interaction: Browsing Export History

```
$ cli contacts exports
Export History
┌────┬────────┬──────────┐
│ ID │ Format │ Status   │
├────┼────────┼──────────┤
│  1 │ csv    │ complete │
└────┴────────┴──────────┘

Navigate to an export:
  cli contacts exports show <id>
```

### 16.4 CLI Command Tree

With actions and history resources discovered, the full command tree looks like this:

```
cli
├── contacts
│   ├── show <id>
│   ├── create
│   ├── archive <id>       (action on contact)
│   ├── unarchive <id>     (action on archived contact)
│   ├── merge              (action on collection)
│   ├── export             (action on collection)
│   ├── history <id>       (read-only log per contact)
│   └── exports            (read-only log of past exports)
└── search
```

The `archive` and `unarchive` commands come from `Actions` on individual contact `Representations`. The `merge` and `export` commands come from `Actions` on the contact list `Representation`. The `history` and `exports` subcommands come from `Links` — `history` from the per-contact `Link` with `rel: "history"`, and `exports` from the collection-level `Link` with `rel: "exports"`. The CLI maps `Action.Rel` values beyond the standard CRUD set (`create`, `update`, `delete`) to named subcommands.

### 16.5 Trade-offs

**Advantages:**

- **Verb-oriented UX.** `cli contacts archive 1` reads naturally as an imperative command. The user thinks "archive this contact," not "create an archive resource." The CLI surface mirrors the user's intent.
- **Fewer resources and endpoints.** Archive and merge do not require their own collection endpoints. The API has `/contacts`, `/contacts/1/archive`, `/contacts/merge`, and `/contacts/export` — no `/contacts/archives`, `/contacts/merges` collections to manage.
- **History is separate from the operation.** You do not have to "create a merge" to merge contacts. The action happens directly; the history entry is a side effect. This keeps the primary interaction simple.
- **No vocabulary list needed.** Because `Action.Rel` is an open vocabulary (per spec §7.2), the CLI does not need to maintain a list of known rels. It renders all `Actions` in the `Representation` as available commands, using `Action.Name` for the human-readable label and `Action.Rel` as the subcommand identifier. Well-known rels (`create`, `update`, `delete`) may get special treatment, but unknown rels like `archive`, `merge`, and `export` are rendered as named subcommands without any client-side vocabulary mapping.

**Disadvantages:**

- **Disconnected history.** The history resources are read-only and somewhat decoupled from the actions that produce them. There is no `Link` from a history entry back to the action that created it (because the action is not a resource). In contrast, the resource-based approach in Section 15 lets you navigate from a merge record to the involved contacts.
- **Export blurs action and resource.** The async export produces a job `Representation` that the user polls — the job itself becomes a resource with state transitions. This is the same pattern as [Section 15.4](#154-export-as-a-resource), undermining the "actions, not resources" philosophy. The distinction is that the job is a consequence of the action, not the primary interface for initiating the export.
- **Non-uniform undo semantics.** Archive has a clean inverse (`unarchive`), but merge may be irreversible — there is no `unmerge` action. The history records what happened but cannot reverse it. In contrast, the resource-based approach can offer `DELETE` on an archive resource for undo, giving a more uniform reversal pattern.

### 16.6 Actions Discovery with the `"actions"` Link

The examples above embed all available actions directly in the `Representation`'s `Actions` array. This is the RECOMMENDED default (per spec §7.1). However, when the set of available actions is large, lazily computed, or depends on additional authorization checks, a server MAY use a `Link` with `rel: "actions"` to point to a separate actions catalog.

#### 16.6.1 Contact with Embedded Actions (Default Pattern)

The contact representation from [Section 16.1.1](#1611-contact-with-archive-action-server-side) already demonstrates the default pattern: all domain-specific actions (`archive`, `update`, `delete`) are embedded directly in the `Actions` array. The CLI discovers them from the representation itself, with no additional fetch required.

The CLI renders these actions as subcommands using a simple algorithm:

1. List all `Actions` from the `Representation`.
2. For well-known rels (`create`, `update`, `delete`, `search`), apply standard behavior (e.g., map `delete` to a destructive command style).
3. For all other rels, surface them as named subcommands using `Action.Rel` as the command name and `Action.Name` as the description.
4. Use `Action.Hints` to adjust rendering (e.g., `"confirm"` triggers a prompt, `"destructive"` shows a warning).

#### 16.6.2 Separate Actions Catalog (Escape Hatch)

When a resource has many possible actions — for example, a contact in a CRM with dozens of workflow automations — embedding them all would bloat the primary representation. In this case, the server provides a `Link` with `rel: "actions"` instead:

```go
contact := hyper.Representation{
    Kind: "contact",
    Self: hyper.MustParseTarget("/contacts/1").Ptr(),
    State: hyper.Object{
        "id":    hyper.Scalar{V: 1},
        "name":  hyper.Scalar{V: "Ada Lovelace"},
        "email": hyper.Scalar{V: "ada@example.com"},
    },
    Links: []hyper.Link{
        {Rel: "contacts", Target: hyper.MustParseTarget("/contacts"), Title: "All Contacts"},
        {Rel: "actions", Target: hyper.MustParseTarget("/contacts/1/actions"), Title: "Available Actions"},
    },
    Actions: []hyper.Action{
        // Only the most common actions are embedded directly
        {
            Name:     "Update Contact",
            Rel:      "update",
            Method:   "PUT",
            Target:   hyper.MustParseTarget("/contacts/1"),
            Consumes: []string{"application/vnd.api+json"},
            Fields: []hyper.Field{
                {Name: "name", Type: "text", Label: "Name", Value: "Ada Lovelace", Required: true},
                {Name: "email", Type: "email", Label: "Email", Value: "ada@example.com", Required: true},
            },
        },
    },
}
```

The `/contacts/1/actions` endpoint returns a `Representation` whose `Actions` array lists all available actions:

```json
{
  "kind": "action-catalog",
  "self": {"href": "/contacts/1/actions"},
  "links": [
    {"rel": "contact", "href": "/contacts/1", "title": "Ada Lovelace"}
  ],
  "actions": [
    {"name": "Archive Contact", "rel": "archive", "method": "POST", "href": "/contacts/1/archive"},
    {"name": "Merge Into", "rel": "merge", "method": "POST", "href": "/contacts/1/merge"},
    {"name": "Send Welcome Email", "rel": "send-welcome", "method": "POST", "href": "/contacts/1/send-welcome"},
    {"name": "Add to Campaign", "rel": "add-to-campaign", "method": "POST", "href": "/contacts/1/add-to-campaign",
      "fields": [{"name": "campaign_id", "type": "text", "label": "Campaign ID", "required": true}]
    },
    {"name": "Export vCard", "rel": "export-vcard", "method": "GET", "href": "/contacts/1/vcard"}
  ]
}
```

#### 16.6.3 CLI Interaction: Actions Discovery

When the CLI encounters a `Link` with `rel: "actions"`, it can offer an `actions` subcommand that lists and invokes available actions:

```
$ cli contacts show 1
Ada Lovelace
  ID:    1
  Email: ada@example.com

Available actions:
  update    Update Contact (cli contacts update 1 --name NAME --email EMAIL)

More actions available:
  cli contacts actions 1

Navigate:
  contacts  All Contacts (cli contacts)
  actions   Available Actions (cli contacts actions 1)

$ cli contacts actions 1
Available actions for Ada Lovelace:
  archive           Archive Contact
  merge             Merge Into
  send-welcome      Send Welcome Email
  add-to-campaign   Add to Campaign
  export-vcard      Export vCard

Usage:
  cli contacts archive 1
  cli contacts send-welcome 1
  cli contacts add-to-campaign 1 --campaign_id ID
```

The CLI merges actions from the embedded `Actions` array with those discovered via the `"actions"` link, giving a complete view of available operations.

### 16.7 Spec Feedback

The actions-with-history approach surfaces additional questions for the spec:

- **`Action.Rel` vocabulary for domain-specific verbs (RESOLVED).** The spec now clarifies (§7.2) that `Action.Rel` is an open string vocabulary — any string is acceptable. The spec recommends well-known rels (`create`, `update`, `delete`, `search`) for CRUD operations and states that all other rels are domain-specific. Clients should treat unknown rels as opaque identifiers and surface them by name. No namespace prefix (e.g., `x-archive`) is required — `Action.Name` and `Action.Hints` provide sufficient context for rendering.

- **`"actions"` link rel for action discovery (RESOLVED).** The spec now defines (§7.1) a recommended `Link` with `rel: "actions"` that points to a resource enumerating available actions. Servers should prefer embedding actions directly in the `Representation`'s `Actions` array when possible. The `"actions"` link is an escape hatch for large or lazily-computed action sets. See [Section 16.6](#166-actions-discovery-with-the-actions-link) for the demonstration.

- **`Action` producing an async result (RESOLVED).** The spec now defines (§7.2) async action conventions: `Action.Hints` MAY include `"async": true` to signal that the response is an async job representation. Job representations use a `"status"` state key with recommended values (`"pending"`, `"processing"`, `"complete"`, `"failed"`), `Meta.poll-interval` for polling guidance, and dynamic `Links` for result delivery. See [Section 16.3](#163-export-as-an-action-with-history) for the demonstration.

- **Conventions for history/audit log `Representations`.** The history collection in this section uses `Kind: "history"` with embedded `Kind: "history-entry"` items. The spec does not define conventions for audit log representations — standard `State` keys for event entries (e.g., `"event"`, `"at"`, `"by"`), or a recommended `Kind` naming pattern. Establishing lightweight conventions would help clients render history views generically without needing to understand each API's custom history schema.

- **`Action` confirm hint for non-destructive operations.** The `confirm` hint (§15.6) is used here for archive, which is not destructive (it is reversible via `unarchive`). The spec should clarify that `confirm` is appropriate for any operation that warrants user confirmation, not just destructive ones. Alternatively, the spec could introduce a separate hint like `"prompt"` for non-destructive confirmations, reserving `"confirm"` for destructive actions paired with `"destructive": true`.

- **State-dependent `Action` availability.** The archived contact loses its `update` and `delete` `Actions` and gains an `unarchive` `Action`. This is standard hypermedia — the server tailors affordances to current state. The spec should explicitly note that `Actions` on a `Representation` MAY change between requests as the resource's state evolves, and clients MUST NOT cache or assume a fixed set of `Actions` for a given resource.

## 17. Spec Feedback

After playing through the full contacts CLI scenario, the following gaps and questions emerge.

**Resolved items** (addressed in the spec):

- **`Action.Rel` is an open vocabulary (RESOLVED).** The spec (§7.2) now clarifies that `Action.Rel` is an open string vocabulary. Well-known rels (`create`, `update`, `delete`, `search`) are recommended for CRUD; all other rels are domain-specific and require no namespace prefix. Clients treat unknown rels as opaque identifiers.

- **`"actions"` link rel convention (RESOLVED).** The spec (§7.1) now defines a recommended `Link` with `rel: "actions"` for discovering available actions. Actions should be embedded directly in the `Actions` array when possible; the `"actions"` link is an escape hatch for large or lazily-computed action sets.

- **Async / job-like resource conventions (RESOLVED).** The spec (§7.2) now defines async action conventions: `Action.Hints` MAY include `"async": true` to signal that the response is an async job. Job representations use a `"status"` state key with recommended values (`"pending"`, `"processing"`, `"complete"`, `"failed"`), `Meta.poll-interval` for polling guidance, dynamic `Links` for result delivery, and optional `"progress"` and `"error"` state keys. See [Section 16.3](use-cases/rest-cli.md#163-export-as-an-action-with-history) for a worked example.

**Open items:**

- **Action discoverability on collections vs. items.** The contacts list `Representation` carries a `create` `Action`, and each embedded contact carries `update`/`delete` `Actions`. The spec does not formally distinguish "collection-level" actions from "item-level" actions. A CLI needs to know whether an `Action` applies to the collection itself or to individual items within `Embedded`. Consider clarifying in §6.1 or §7.2 that `Actions` on the outer `Representation` are collection-level, while `Actions` on `Embedded` representations are item-level — or introduce a convention for this.

- **Pagination.** A contacts collection with thousands of items needs `next`/`prev` navigation. The spec has no pagination model. The CLI would need `Links` with rels like `next`, `prev`, `first`, `last` on the collection `Representation`. Should the spec define standard link rels for pagination, or recommend a convention? Without this, every API invents its own pagination discovery.

- **Collection metadata.** Total count, page size, current page — these are essential for CLI output like `"Showing 1-20 of 142 contacts"`. Should these live in `Meta` on the collection `Representation`? The spec does not recommend any standard `Meta` keys. A convention like `meta.total`, `meta.page`, `meta.pageSize` would help clients render pagination info without parsing `Links`.

- **Field type `search`.** The recommended field type vocabulary (§7.3.1) does not include `search`. The root `Search` action uses `Field.Type: "text"` for the query parameter, but a `search` type would let CLI clients offer search-specific behavior (e.g., history, fuzzy matching). Consider adding `search` to the vocabulary table.

- **Action grouping / categorization.** A CLI with many actions (contacts CRUD + search + import + export + merge) needs grouping for readable `--help` output. `Action.Hints` could support a `group` or `category` key (e.g., `"group": "management"`) so the CLI can organize commands into sections. Should this be a recommended hint key?

- **Machine-readable error representations.** The spec does not define how error responses should be structured. This use case assumes the server returns a `Representation` with `Field.Error` values on a 422, but what about 404, 500, or other errors? Should there be a conventional error `Representation.Kind` (e.g., `"error"`) with standard `State` keys like `"message"`, `"code"`, `"details"`? Without this, every server invents its own error format.

- **Field value type coercion.** `Field.Value` is typed as `any` in Go, but the JSON wire format does not specify how to distinguish between `"42"` (string) and `42` (number). A CLI needs to know whether `--id` expects a number or string. `Field.Type` helps (`number` vs `text`), but the spec could clarify that clients SHOULD use `Field.Type` to coerce `Field.Value` from JSON.

- **Action ordering.** The spec does not define whether `Actions` ordering is significant. A CLI displaying actions in `--help` would benefit from knowing that the server's ordering is intentional (e.g., primary action first). Consider stating that `Actions` ordering SHOULD be preserved by codecs and MAY be treated as significant by clients.

- **Parent/child link rel conventions.** Nested subcommands reveal the need for conventional `Link.Rel` values for parent/child navigation. This use case uses `"contact"` as a rel to navigate from a notes list back to the parent contact, but there is no standard rel for "go to my parent resource." The spec should consider recommending `up` as a rel value to navigate to a parent resource (analogous to the IANA-registered `up` link relation), and `collection` to indicate "this link goes to the parent collection" (e.g., from a single note back to the notes list). Without conventions, every API invents its own parent navigation rels, making generic CLI clients harder to build.

- **Auth-driven representation variation.** The spec should clarify that servers MAY return different `Links` and `Actions` based on the client's authentication state. This is implicit in the hypermedia model — the server is always free to tailor the representation — but worth stating explicitly. A server that omits the `contacts` link for unauthenticated clients and includes a `login` action is behaving correctly, and clients should expect the set of affordances to change between requests as auth state evolves.

- **Standard hint for auth-required actions.** Should there be a hint key like `"auth-required": true` on actions that will fail without authentication? This would let a CLI pre-check and prompt for login before attempting the action, rather than waiting for a 401 response. Without this, the client must either attempt the action optimistically and handle the 401, or infer auth requirements from the absence of the action in unauthenticated representations. A standard hint key would make this explicit.

- **Recommended JSON content type.** The spec's JSON codec section (§13) does not specify a media type identifier for the `hyper` JSON wire format. The spec SHOULD recommend `application/vnd.api+json` as the content type for JSON representations that include full hypermedia controls (links, actions, embedded). This distinguishes hypermedia-aware JSON responses from plain `application/json` data payloads. Servers that support both can use content negotiation to serve the appropriate format.

- **Default `Consumes` behavior.** The spec does not state what content type a client should assume when `Action.Consumes` is empty. The spec SHOULD clarify: when `Consumes` is absent, clients SHOULD default to `application/vnd.api+json` for actions with fields, and send no body for actions without fields.

- **Content type and codec selection.** The spec should clarify how `Action.Consumes` and `Action.Produces` relate to the codec system (§9). When a client sees `Consumes: ["application/vnd.api+json"]`, it should use the JSON `SubmissionCodec` to encode the request body. The spec should state this relationship explicitly.

- **Multi-select field type.** The "operations as resources" pattern (Section 15) requires `Fields` that accept multiple entity IDs — e.g., selecting which contacts to archive or merge. The current `Field.Type` vocabulary has `select` for single-value selection, but no `multi-select` or equivalent for choosing multiple values from a set. The spec should consider adding a `multi-select` field type, or clarify conventions for accepting comma-separated IDs in a `text` field. Without this, servers must use `text` fields with help text explaining the expected format, which is fragile and not machine-parseable.

- **Async / job-like resource conventions (RESOLVED).** The spec (§7.2) now defines async action conventions. See the resolved items above for details.

- **Immutable resources.** The merge resource (Section 15.3) is intentionally immutable — it has no `update` or `delete` `Actions`. The spec does not provide a way for a server to signal that a resource is immutable. A `Hints` key like `"immutable": true` on the `Representation` (not just on `Actions`) would let clients communicate this clearly in their UI — e.g., a CLI could display "(read-only)" next to the resource, and a web UI could suppress edit buttons entirely.
