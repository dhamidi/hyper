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

1. The client checks for stored credentials for the base URL (see [Section 12.6](#126-credential-storage)) and includes them in the request if present
2. The client sends `GET /` with `Accept: application/json` (and `Authorization: Bearer <token>` if credentials are stored)
3. The server returns a root `Representation` encoded per the JSON wire format (§13.3) — the representation's `Links` and `Actions` may vary based on authentication state (see [Section 12](#12-authentication))
4. The client parses top-level `Links` and `Actions` to build a command tree

> **Note:** The initial root fetch may return a limited representation if the client is not authenticated. An unauthenticated root might expose only a `login` action, while an authenticated root exposes the full set of links and actions. The CLI should always check for stored credentials before the initial request to ensure the richest possible command tree on startup.

### 2.2 Root Representation (Server Side)

The server builds the root `Representation` using `hyper` types:

```go
root := hyper.Representation{
    Kind: "root",
    Self: &hyper.Target{Href: "/"},
    Links: []hyper.Link{
        {Rel: "contacts", Target: hyper.Target{Href: "/contacts"}, Title: "Contacts"},
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

> **Note:** This is the *initial* command tree built from the root `Representation`. As the user navigates via `Links`, the command tree grows dynamically — each fetched `Representation` contributes its own `Links` and `Actions` as subcommands. See [Section 6: Nested Subcommands and Deep Navigation](#6-nested-subcommands-and-deep-navigation) for the full recursive algorithm.

## 3. Command Mapping

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
| `Action.Consumes` | Determines submission encoding (JSON, form-encoded) |
| `Embedded` representations | Table rows / list items in output |
| `Representation.Kind` | Output formatter selection |
| `Action.Hints["confirm"]` | Confirmation prompt before execution |
| `Action.Hints["destructive"]` | Red/warning styling in terminal |
| `Action.Hints["hidden"]` | Suppress from default command listings |

## 4. Navigating a Collection

### 4.1 Following the `contacts` Link

When the user runs `cli contacts`, the client follows the `contacts` link discovered from the root:

1. The client sends `GET /contacts` with `Accept: application/json`
2. The server returns a contacts list `Representation` with `Embedded` items (per §6.1)

### 4.2 Contacts List (Server Side)

```go
list := hyper.Representation{
    Kind: "contact-list",
    Self: &hyper.Target{Href: "/contacts"},
    Links: []hyper.Link{
        {Rel: "root", Target: hyper.Target{Href: "/"}, Title: "Home"},
    },
    Actions: []hyper.Action{
        {
            Name:     "Create Contact",
            Rel:      "create",
            Method:   "POST",
            Target:   hyper.Target{Href: "/contacts"},
            Consumes: []string{"application/json"},
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
                Self: &hyper.Target{Href: "/contacts/1"},
                State: hyper.Object{
                    "id":    hyper.Scalar{V: 1},
                    "name":  hyper.Scalar{V: "Ada Lovelace"},
                    "email": hyper.Scalar{V: "ada@example.com"},
                },
                Links: []hyper.Link{
                    {Rel: "self", Target: hyper.Target{Href: "/contacts/1"}, Title: "Ada Lovelace"},
                },
            },
            {
                Kind: "contact",
                Self: &hyper.Target{Href: "/contacts/2"},
                State: hyper.Object{
                    "id":    hyper.Scalar{V: 2},
                    "name":  hyper.Scalar{V: "Grace Hopper"},
                    "email": hyper.Scalar{V: "grace@example.com"},
                },
                Links: []hyper.Link{
                    {Rel: "self", Target: hyper.Target{Href: "/contacts/2"}, Title: "Grace Hopper"},
                },
            },
        },
    },
}
```

### 4.3 Contacts List (JSON Wire Format)

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
      "consumes": ["application/json"],
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

### 4.4 CLI Output

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

## 5. Viewing a Single Resource

### 5.1 Following an Item Link

When the user runs `cli contacts show 1`, the client resolves the `Self` target of the embedded item and fetches `/contacts/1`.

### 5.2 Single Contact (Server Side)

```go
contact := hyper.Representation{
    Kind: "contact",
    Self: &hyper.Target{Href: "/contacts/1"},
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
        {Rel: "contacts", Target: hyper.Target{Href: "/contacts"}, Title: "All Contacts"},
    },
    Actions: []hyper.Action{
        {
            Name:     "Update Contact",
            Rel:      "update",
            Method:   "PUT",
            Target:   hyper.Target{Href: "/contacts/1"},
            Consumes: []string{"application/json"},
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
            Target: hyper.Target{Href: "/contacts/1"},
            Hints: map[string]any{
                "confirm":     "Are you sure you want to delete Ada Lovelace?",
                "destructive": true,
            },
        },
    },
}
```

### 5.3 Single Contact (JSON Wire Format)

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
      "consumes": ["application/json"],
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

### 5.4 CLI Output

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

## 6. Nested Subcommands and Deep Navigation

The examples so far show a flat command tree: `cli contacts`, `cli contacts show 1`. Real APIs have nested resources — a contact has notes, a note has attachments. The hypermedia-driven CLI handles arbitrary nesting by following `Links` discovered at each level. The CLI never hard-codes nesting depth; each navigation step applies the same algorithm to whatever `Representation` the server returns.

### 6.1 Nested Resource Discovery

When the server returns a single contact `Representation`, it can expose `Links` to sub-resources alongside its existing `Actions`. Here the contact carries links to its notes and tags:

```go
contact := hyper.Representation{
    Kind: "contact",
    Self: &hyper.Target{Href: "/contacts/1"},
    State: hyper.Object{
        "id":   hyper.Scalar{V: 1},
        "name": hyper.Scalar{V: "Ada Lovelace"},
    },
    Links: []hyper.Link{
        {Rel: "contacts", Target: hyper.Target{Href: "/contacts"}, Title: "All Contacts"},
        {Rel: "notes", Target: hyper.Target{Href: "/contacts/1/notes"}, Title: "Notes"},
        {Rel: "tags", Target: hyper.Target{Href: "/contacts/1/tags"}, Title: "Tags"},
    },
    Actions: []hyper.Action{
        {
            Name:     "Update Contact",
            Rel:      "update",
            Method:   "PUT",
            Target:   hyper.Target{Href: "/contacts/1"},
            Consumes: []string{"application/json"},
            Fields: []hyper.Field{
                {Name: "name", Type: "text", Label: "Name", Value: "Ada Lovelace", Required: true},
                {Name: "email", Type: "email", Label: "Email", Value: "ada@example.com", Required: true},
            },
        },
        {
            Name:   "Delete Contact",
            Rel:    "delete",
            Method: "DELETE",
            Target: hyper.Target{Href: "/contacts/1"},
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
      "consumes": ["application/json"],
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

### 6.2 Following Nested Links

When the user runs `cli contacts 1 notes`, the CLI follows the `notes` link from the contact `Representation`:

1. The client sends `GET /contacts/1/notes` with `Accept: application/json`
2. The server returns a notes list `Representation` with its own `Embedded` items, `Actions`, and `Links`

#### Notes List (Server Side)

```go
notesList := hyper.Representation{
    Kind: "note-list",
    Self: &hyper.Target{Href: "/contacts/1/notes"},
    Links: []hyper.Link{
        {Rel: "contact", Target: hyper.Target{Href: "/contacts/1"}, Title: "Ada Lovelace"},
    },
    Actions: []hyper.Action{
        {
            Name:     "Add Note",
            Rel:      "create",
            Method:   "POST",
            Target:   hyper.Target{Href: "/contacts/1/notes"},
            Consumes: []string{"application/json"},
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
                Self: &hyper.Target{Href: "/contacts/1/notes/3"},
                State: hyper.Object{
                    "id":      hyper.Scalar{V: 3},
                    "title":   hyper.Scalar{V: "Meeting notes"},
                    "created": hyper.Scalar{V: "2025-11-20"},
                },
                Links: []hyper.Link{
                    {Rel: "self", Target: hyper.Target{Href: "/contacts/1/notes/3"}, Title: "Meeting notes"},
                },
            },
            {
                Kind: "note",
                Self: &hyper.Target{Href: "/contacts/1/notes/7"},
                State: hyper.Object{
                    "id":      hyper.Scalar{V: 7},
                    "title":   hyper.Scalar{V: "Follow-up tasks"},
                    "created": hyper.Scalar{V: "2025-12-03"},
                },
                Links: []hyper.Link{
                    {Rel: "self", Target: hyper.Target{Href: "/contacts/1/notes/7"}, Title: "Follow-up tasks"},
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
      "consumes": ["application/json"],
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

### 6.3 Deeply Nested Navigation

When the user runs `cli contacts 1 notes 3`, the CLI resolves the `Self` target of the embedded note and fetches `/contacts/1/notes/3`:

#### Single Note (Server Side)

```go
note := hyper.Representation{
    Kind: "note",
    Self: &hyper.Target{Href: "/contacts/1/notes/3"},
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
        {Rel: "notes", Target: hyper.Target{Href: "/contacts/1/notes"}, Title: "All Notes"},
        {Rel: "contact", Target: hyper.Target{Href: "/contacts/1"}, Title: "Ada Lovelace"},
    },
    Actions: []hyper.Action{
        {
            Name:     "Edit Note",
            Rel:      "update",
            Method:   "PUT",
            Target:   hyper.Target{Href: "/contacts/1/notes/3"},
            Consumes: []string{"application/json"},
            Fields: []hyper.Field{
                {Name: "title", Type: "text", Label: "Title", Value: "Meeting notes", Required: true},
                {Name: "body", Type: "text", Label: "Body", Value: "Discussed the *analytical engine* project timeline.", Required: true},
            },
        },
        {
            Name:   "Delete Note",
            Rel:    "delete",
            Method: "DELETE",
            Target: hyper.Target{Href: "/contacts/1/notes/3"},
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
      "consumes": ["application/json"],
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

### 6.4 Command Tree Construction Algorithm

The CLI uses a single recursive algorithm for every navigation step:

1. **Fetch** the current `Representation` from the server
2. **For each `Link`**, register a navigable subcommand named after `Link.Rel`. When the user invokes it, follow `Link.Target.Href` and recurse from step 1
3. **For each `Action`**, register an executable command named after `Action.Rel`. Generate flags from `Action.Fields` — `Field.Name` becomes the flag name, `Field.Required` determines whether it is mandatory, `Field.Value` provides the default, and `Field.Type` drives validation
4. **For `Embedded` items** with `Self` targets, register `show <id>` subcommands. When the user invokes one, fetch the `Self.Href` and recurse from step 1

In pseudocode:

```go
func buildCommands(rep hyper.Representation) *CommandGroup {
    group := &CommandGroup{Kind: rep.Kind, Self: rep.Self}

    // Links become navigable subcommands
    for _, link := range rep.Links {
        group.AddSubcommand(link.Rel, link.Title, func() {
            next := fetch(link.Target.Href)
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
                    next := fetch(item.Self.Href)
                    buildCommands(next)  // recurse
                })
            }
        }
    }

    return group
}
```

The critical insight: the CLI never hard-codes nesting depth. Whether the user navigates to `cli contacts`, `cli contacts 1 notes`, or `cli contacts 1 notes 3 attachments`, each step is the same algorithm applied to whatever `Representation` the server returns. The server controls the shape of the resource hierarchy through the `Links` it includes in each response.

### 6.5 Breadcrumb / Path Display

The interactive REPL prompt reflects nesting depth using the `Representation.Kind` and `Self.Href` at each level. As the user navigates deeper, the prompt updates to show the current position in the resource hierarchy:

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
- `Self.Href` provides the path (e.g., `/contacts/1/notes`)
- `back` pops the navigation stack and returns to the previous `Representation`

### 6.6 CLI Output for Nested Resources

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

The output structure is identical to the top-level contacts list (Section 4.4). The CLI uses the same rendering logic regardless of nesting depth — the `Representation.Kind` selects the formatter, `Embedded["items"]` provides the table rows, `Actions` list the available mutations, and `Links` offer navigation back to the parent resource.

## 7. Executing Actions

### 7.1 Create

The `create` `Action` is discovered on the contacts list `Representation` (§7.2). Its `Fields` map to CLI flags:

```
$ cli contacts create --name "Alan Turing" --email "alan@example.com"
```

The CLI:

1. Finds the `Action` with `Rel: "create"` on the contacts list `Representation`
2. Maps `--name` and `--email` to `Field` values
3. Validates that required `Fields` (`name`, `email`) are present
4. Submits a JSON body (from `Action.Consumes: ["application/json"]`) to the `Action.Target`:

```
POST /contacts
Content-Type: application/json

{"name": "Alan Turing", "email": "alan@example.com"}
```

5. Parses the response `Representation` and displays the created resource:

```
Created contact #3
  Name:  Alan Turing
  Email: alan@example.com
```

### 7.2 Update

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
Content-Type: application/json

{"name": "Ada Lovelace", "email": "ada.lovelace@example.com", "phone": "+1-555-0100"}
```

6. Displays the updated resource from the response.

### 7.3 Delete

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

## 8. Search as a GET Action with Fields

The root `Representation` exposes a `Search` `Action` with `Method: "GET"` and a `Field` for the query parameter:

```
$ cli search --q "Ada"
```

The CLI:

1. Finds the `Action` with `Rel: "search"` on the root `Representation`
2. Since `Method` is `GET`, maps `Fields` to query parameters instead of a request body
3. Sends `GET /search?q=Ada` with `Accept: application/json`

### 8.1 Search Results (Server Side)

```go
results := hyper.Representation{
    Kind: "search-results",
    Self: &hyper.Target{Href: "/search?q=Ada"},
    State: hyper.Object{
        "query": hyper.Scalar{V: "Ada"},
    },
    Embedded: map[string][]hyper.Representation{
        "items": {
            {
                Kind: "contact",
                Self: &hyper.Target{Href: "/contacts/1"},
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

### 8.2 Search Results (JSON Wire Format)

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

### 8.3 CLI Output

```
$ cli search --q "Ada"

Search results for "Ada"
┌────┬────────────────┬───────────────────┐
│ ID │ Name           │ Email             │
├────┼────────────────┼───────────────────┤
│  1 │ Ada Lovelace   │ ada@example.com   │
└────┴────────────────┴───────────────────┘
```

## 9. Interactive Mode

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

## 10. Output Formatting

The CLI supports multiple output modes, driven by the `Representation` and user flags:

### 10.1 Default Formatting

- **Collections** (`Embedded` with `"items"` slot): rendered as tables, with columns inferred from the `State` keys of the first embedded `Representation`
- **Single resources**: rendered as key-value pairs
- **`Representation.Kind`** can select specialized formatters — a kind of `"contact"` might use a contact-specific layout, while an unrecognized kind falls back to generic formatting

### 10.2 `--json` Flag

Outputs the raw JSON wire format (§13.3) as received from the server:

```
$ cli contacts --json
{
  "kind": "contact-list",
  "self": {"href": "/contacts"},
  ...
}
```

### 10.3 `--markdown` Flag

Requests `Accept: text/markdown` from the server, which triggers the Markdown codec (§12):

```
$ cli contacts show 1 --markdown

# Ada Lovelace

- **Email:** ada@example.com
- **Phone:** +1-555-0100
- **Bio:** Wrote the *first* algorithm intended for a machine.
```

### 10.4 Kind-Based Formatting

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

## 11. Error Handling

### 11.1 HTTP Errors

When the server returns an error status, the CLI displays the status code and any error `Representation` body:

```
$ cli contacts show 999

Error 404: Not Found
  Contact not found.
```

### 11.2 Validation Errors

When the server returns `422 Unprocessable Entity`, it includes a `Representation` whose `Action.Fields` carry `Field.Error` values (§7.3):

```go
validationRep := hyper.Representation{
    Kind: "contact-form",
    Self: &hyper.Target{Href: "/contacts"},
    Actions: []hyper.Action{
        {
            Name:     "Create Contact",
            Rel:      "create",
            Method:   "POST",
            Target:   hyper.Target{Href: "/contacts"},
            Consumes: []string{"application/json"},
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
      "consumes": ["application/json"],
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

### 11.3 Network Errors

When the server is unreachable, the CLI reports the connection failure and suggests retrying:

```
$ cli contacts

Error: could not connect to http://localhost:8080/contacts
  Connection refused. Is the server running?
```

## 12. Authentication

A hypermedia-driven CLI discovers authentication affordances the same way it discovers everything else: through `Links` and `Actions` on the root `Representation`. The server controls what is available based on auth state — an unauthenticated root representation exposes a `login` action, while an authenticated one exposes a `logout` action and additional protected links.

### 12.1 Unauthenticated Root Representation

When the CLI connects without stored credentials, the server returns a root `Representation` with limited affordances:

```go
root := hyper.Representation{
    Kind: "root",
    Self: &hyper.Target{Href: "/"},
    Links: []hyper.Link{
        // No "contacts" link — requires auth
    },
    Actions: []hyper.Action{
        {
            Name:   "Login",
            Rel:    "login",
            Method: "POST",
            Target: hyper.Target{Href: "/auth/login"},
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

### 12.2 Login Flow

The user authenticates with `cli login --username ada --password secret`:

1. The CLI finds the `login` `Action` on the root `Representation` by matching `Action.Rel == "login"`
2. `Field.Type: "password"` triggers masked input if the `--password` flag is omitted — the CLI prompts interactively without echoing characters
3. The CLI submits credentials to the action's `Target.Href` (`/auth/login`) using the method (`POST`) and content type specified in `Action.Consumes` (`application/vnd.api+json`)
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
        {Rel: "root", Target: hyper.Target{Href: "/"}, Title: "Home"},
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

### 12.3 Authenticated Root Representation

After login, the CLI re-fetches the root with `Authorization: Bearer eyJhbGci...`. The server now returns a richer `Representation` with protected links and a `logout` action:

```go
root := hyper.Representation{
    Kind: "root",
    Self: &hyper.Target{Href: "/"},
    State: hyper.Object{
        "user": hyper.Scalar{V: "ada"},
    },
    Links: []hyper.Link{
        {Rel: "contacts", Target: hyper.Target{Href: "/contacts"}, Title: "Contacts"},
        {Rel: "profile", Target: hyper.Target{Href: "/profile"}, Title: "My Profile"},
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
        {
            Name:   "Logout",
            Rel:    "logout",
            Method: "POST",
            Target: hyper.Target{Href: "/auth/logout"},
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

### 12.4 Logout Flow

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

### 12.5 Token Refresh and Expiry

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
            Target: hyper.Target{Href: "/auth/login"},
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
        {Rel: "root", Target: hyper.Target{Href: "/"}, Title: "Home"},
    },
    Actions: []hyper.Action{
        {
            Name:   "Refresh Token",
            Rel:    "refresh",
            Method: "POST",
            Target: hyper.Target{Href: "/auth/refresh"},
        },
    },
}
```

When the CLI detects that `expires_at` is approaching, it submits the `refresh` action automatically. The server returns a new auth-token `Representation` with an updated token and expiry. The CLI stores the new token and continues without interrupting the user.

### 12.6 Credential Storage

The CLI manages credentials locally with the following conventions:

- Credentials are stored per base URL in `~/.config/cli/credentials.json`, allowing the user to be authenticated against multiple servers simultaneously
- `Field.Type: "password"` (§7.3.1) signals the CLI to prompt interactively with masked input when the value is not provided as a flag — this prevents passwords from appearing in shell history
- The `--token` flag allows passing a bearer token directly for scripting and CI/CD use cases, bypassing the interactive login flow: `cli --token eyJhbGci... contacts`
- Stored tokens are included in all subsequent requests as `Authorization: Bearer <token>` headers

### 12.7 Interactive Mode Auth

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

The REPL re-fetches the root representation after login and rebuilds the command tree. The same happens after logout — the command tree collapses back to just `login`. This is the same discovery mechanism described in [Section 2](#2-discovery-flow) and [Section 9](#9-interactive-mode), applied to auth state transitions.

## 13. Representations and Types Exercised

This use case exercises the following `hyper` types:

| Type | Role in This Use Case |
|---|---|
| `Representation` | Every server response — root, list, single contact, nested note list, single note, search results, errors |
| `Link` | Navigation and subcommand discovery (`contacts`, `root`, `self`, `notes`, `tags`, `contact`). Recursive `Link` following drives nested subcommands — the CLI follows `Links` at each level to build an arbitrarily deep command tree |
| `Action` | All mutations (`create`, `update`, `delete`) and parameterized queries (`search`). Actions on nested resources (e.g., "Add Note", "Edit Note") work identically to top-level actions |
| `Field` | Flag generation, validation, completion candidates, default values |
| `Option` | Enum completion candidates (not shown in contacts but supported for `select` fields) |
| `Target` with `Href` | Resolved URLs from JSON codec (§13.3.6) |
| `Embedded` representations | Collection items in list and search results (§6.1). `Embedded` representations within nested resources carry their own full hypermedia controls — `Links`, `Actions`, and `Self` targets — enabling recursive navigation at every level |
| `Node` (`Object`) | Contact and note state as key-value pairs |
| `Value` (`Scalar`, `RichText`) | Primitive fields, bio content, and note body content |
| JSON `RepresentationCodec` | Primary codec for all CLI communication (§13) |
| `Action.Hints` | CLI-specific keys: `confirm`, `destructive`, `hidden` (§15.6) |
| `Action.Consumes` | Drives the submission content type for login (`application/vnd.api+json`) |
| `Field.Type: "password"` | Masked interactive input — the CLI prompts without echoing characters when this field type is encountered |
| `Meta` | Could carry pagination info, total counts (gap identified below) |

## 14. Spec Feedback

After playing through the full contacts CLI scenario, the following gaps and questions emerge:

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
