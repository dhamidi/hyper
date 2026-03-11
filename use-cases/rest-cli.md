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

1. The client sends `GET /` with `Accept: application/json`
2. The server returns a root `Representation` encoded per the JSON wire format (§13.3)
3. The client parses top-level `Links` and `Actions` to build a command tree

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

## 6. Executing Actions

### 6.1 Create

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

### 6.2 Update

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

### 6.3 Delete

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

## 7. Search as a GET Action with Fields

The root `Representation` exposes a `Search` `Action` with `Method: "GET"` and a `Field` for the query parameter:

```
$ cli search --q "Ada"
```

The CLI:

1. Finds the `Action` with `Rel: "search"` on the root `Representation`
2. Since `Method` is `GET`, maps `Fields` to query parameters instead of a request body
3. Sends `GET /search?q=Ada` with `Accept: application/json`

### 7.1 Search Results (Server Side)

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

### 7.2 Search Results (JSON Wire Format)

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

### 7.3 CLI Output

```
$ cli search --q "Ada"

Search results for "Ada"
┌────┬────────────────┬───────────────────┐
│ ID │ Name           │ Email             │
├────┼────────────────┼───────────────────┤
│  1 │ Ada Lovelace   │ ada@example.com   │
└────┴────────────────┴───────────────────┘
```

## 8. Interactive Mode

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
  back      Return to previous view

contact(/contacts/1)> back
contact-list>
```

Key behaviors:

- The prompt reflects the current `Representation.Kind` and path
- `help` is generated dynamically from the current `Representation`'s `Links` and `Actions`
- Tab completion is populated from `Field.Options` and discovered `Link.Rel` / `Action.Rel` values
- After each response, the CLI re-parses the new `Representation` and updates available commands
- `Action.Hints["hidden"]` actions are omitted from `help` output but remain invocable

## 9. Output Formatting

The CLI supports multiple output modes, driven by the `Representation` and user flags:

### 9.1 Default Formatting

- **Collections** (`Embedded` with `"items"` slot): rendered as tables, with columns inferred from the `State` keys of the first embedded `Representation`
- **Single resources**: rendered as key-value pairs
- **`Representation.Kind`** can select specialized formatters — a kind of `"contact"` might use a contact-specific layout, while an unrecognized kind falls back to generic formatting

### 9.2 `--json` Flag

Outputs the raw JSON wire format (§13.3) as received from the server:

```
$ cli contacts --json
{
  "kind": "contact-list",
  "self": {"href": "/contacts"},
  ...
}
```

### 9.3 `--markdown` Flag

Requests `Accept: text/markdown` from the server, which triggers the Markdown codec (§12):

```
$ cli contacts show 1 --markdown

# Ada Lovelace

- **Email:** ada@example.com
- **Phone:** +1-555-0100
- **Bio:** Wrote the *first* algorithm intended for a machine.
```

### 9.4 Kind-Based Formatting

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

## 10. Error Handling

### 10.1 HTTP Errors

When the server returns an error status, the CLI displays the status code and any error `Representation` body:

```
$ cli contacts show 999

Error 404: Not Found
  Contact not found.
```

### 10.2 Validation Errors

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

### 10.3 Network Errors

When the server is unreachable, the CLI reports the connection failure and suggests retrying:

```
$ cli contacts

Error: could not connect to http://localhost:8080/contacts
  Connection refused. Is the server running?
```

## 11. Representations and Types Exercised

This use case exercises the following `hyper` types:

| Type | Role in This Use Case |
|---|---|
| `Representation` | Every server response — root, list, single contact, search results, errors |
| `Link` | Navigation and subcommand discovery (`contacts`, `root`, `self`) |
| `Action` | All mutations (`create`, `update`, `delete`) and parameterized queries (`search`) |
| `Field` | Flag generation, validation, completion candidates, default values |
| `Option` | Enum completion candidates (not shown in contacts but supported for `select` fields) |
| `Target` with `Href` | Resolved URLs from JSON codec (§13.3.6) |
| `Embedded` representations | Collection items in list and search results (§6.1) |
| `Node` (`Object`) | Contact state as key-value pairs |
| `Value` (`Scalar`, `RichText`) | Primitive fields and bio content |
| JSON `RepresentationCodec` | Primary codec for all CLI communication (§13) |
| `Action.Hints` | CLI-specific keys: `confirm`, `destructive`, `hidden` (§15.6) |
| `Meta` | Could carry pagination info, total counts (gap identified below) |

## 12. Spec Feedback

After playing through the full contacts CLI scenario, the following gaps and questions emerge:

- **Action discoverability on collections vs. items.** The contacts list `Representation` carries a `create` `Action`, and each embedded contact carries `update`/`delete` `Actions`. The spec does not formally distinguish "collection-level" actions from "item-level" actions. A CLI needs to know whether an `Action` applies to the collection itself or to individual items within `Embedded`. Consider clarifying in §6.1 or §7.2 that `Actions` on the outer `Representation` are collection-level, while `Actions` on `Embedded` representations are item-level — or introduce a convention for this.

- **Pagination.** A contacts collection with thousands of items needs `next`/`prev` navigation. The spec has no pagination model. The CLI would need `Links` with rels like `next`, `prev`, `first`, `last` on the collection `Representation`. Should the spec define standard link rels for pagination, or recommend a convention? Without this, every API invents its own pagination discovery.

- **Collection metadata.** Total count, page size, current page — these are essential for CLI output like `"Showing 1-20 of 142 contacts"`. Should these live in `Meta` on the collection `Representation`? The spec does not recommend any standard `Meta` keys. A convention like `meta.total`, `meta.page`, `meta.pageSize` would help clients render pagination info without parsing `Links`.

- **Field type `search`.** The recommended field type vocabulary (§7.3.1) does not include `search`. The root `Search` action uses `Field.Type: "text"` for the query parameter, but a `search` type would let CLI clients offer search-specific behavior (e.g., history, fuzzy matching). Consider adding `search` to the vocabulary table.

- **Action grouping / categorization.** A CLI with many actions (contacts CRUD + search + import + export + merge) needs grouping for readable `--help` output. `Action.Hints` could support a `group` or `category` key (e.g., `"group": "management"`) so the CLI can organize commands into sections. Should this be a recommended hint key?

- **Machine-readable error representations.** The spec does not define how error responses should be structured. This use case assumes the server returns a `Representation` with `Field.Error` values on a 422, but what about 404, 500, or other errors? Should there be a conventional error `Representation.Kind` (e.g., `"error"`) with standard `State` keys like `"message"`, `"code"`, `"details"`? Without this, every server invents its own error format.

- **Field value type coercion.** `Field.Value` is typed as `any` in Go, but the JSON wire format does not specify how to distinguish between `"42"` (string) and `42` (number). A CLI needs to know whether `--id` expects a number or string. `Field.Type` helps (`number` vs `text`), but the spec could clarify that clients SHOULD use `Field.Type` to coerce `Field.Value` from JSON.

- **Action ordering.** The spec does not define whether `Actions` ordering is significant. A CLI displaying actions in `--help` would benefit from knowing that the server's ordering is intentional (e.g., primary action first). Consider stating that `Actions` ordering SHOULD be preserved by codecs and MAY be treated as significant by clients.
