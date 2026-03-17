# Proposal 002: Pagination and Collection Conventions

- **Status:** Draft
- **Date:** 2026-03-17
- **Author:** hyper contributors

## 1. Problem Statement

Every use-case exploration that displays collections has independently invented
its own pagination and collection structure. The contacts-hypermedia use case
added `next`/`prev` links with `current_page` and `page_size` meta. The blog
platform added `total_count`, `status_counts`, and conditional pagination links.
The CLI server and REST CLI use cases identified pagination as an unresolved gap
but did not implement it. The media library uses `total_count` but no
pagination links.

Without a standard convention:

- Generic clients (CLIs, navigators, AI agents) cannot navigate paginated
  collections because there is no consistent mechanism for discovering
  next/previous pages.
- Each server author re-invents pagination metadata keys, leading to
  incompatibilities (`"total"` vs `"total_count"`, `"page"` vs
  `"current_page"`).
- Empty collections are handled inconsistently — some use `"items": []`,
  others omit the key entirely.
- The `Embedded` map provides the mechanism for nesting collection items, but
  the spec does not define a conventional key name or structure.

## 2. Background

### 2.1 Collection Patterns Found in Use Cases

The following table summarizes collection and pagination patterns discovered
across use-case explorations:

| Use Case | Embedded Key | Pagination Links | Pagination Meta | Empty Collection |
|---|---|---|---|---|
| contacts-hypermedia | `"items"` | `next`, `prev` | `current_page`, `page_size` | `"items": []` |
| cli-server | `"items"` | — | — | `"items": []` |
| blog-platform (posts) | `"items"` | `next`, `prev` | `total_count`, `page_size`, `current_page`, `status_counts` | — |
| blog-platform (comments) | `"items"` | `next`, `prev` | `total_count`, `page_size`, `current_page`, `status_counts` | — |
| blog-platform (media) | `"items"` | — | `total_count` | — |
| rest-cli | `"items"` | — | — | — |

**Observations:**

1. **All use cases converge on `"items"` as the embedded key.** No use case
   uses a different key name for the primary collection slot.

2. **Pagination links are implemented only in contacts-hypermedia and
   blog-platform.** Both use IANA-registered `next` and `prev` rels.
   cli-server and rest-cli identify pagination as an unresolved gap but do
   not implement it.

3. **Pagination metadata varies by application need.** `current_page` and
   `page_size` are common across use cases that implement pagination.
   `total_count` appears when the total is known. `status_counts` is
   application-specific metadata in blog-platform.

4. **Empty collection handling is demonstrated by contacts-hypermedia and
   cli-server.** Both use `"items": []` — an explicit empty array in the
   `Embedded` map. The cli-server use case raises this as an open question
   (empty array vs. omitted key vs. null).

### 2.2 Existing Pagination Code in contacts-hypermedia

The contacts-hypermedia use case demonstrates the core pagination pattern
using `Links` for navigation and `Meta` for page metadata:

```go
// Add pagination links using IANA-registered rels (§5.3) and RouteRef.Query (§8.1)
if page > 1 {
    rep.Links = append(rep.Links, hyper.Link{
        Rel: "prev",
        Target: hyper.Target{Route: &hyper.RouteRef{
            Name:  "contacts.list",
            Query: url.Values{"page": {strconv.Itoa(page - 1)}},
        }},
        Title: "Previous Page",
    })
}
if len(contacts) == 100 {
    rep.Links = append(rep.Links, hyper.Link{
        Rel: "next",
        Target: hyper.Target{Route: &hyper.RouteRef{
            Name:  "contacts.list",
            Query: url.Values{"page": {strconv.Itoa(page + 1)}},
        }},
        Title: "Next Page",
    })
}

// Add pagination metadata (§4.1)
rep.Meta = map[string]any{
    "current_page": page,
    "page_size":    100,
}
```

### 2.3 The `Representation` Type

The existing `Representation` type (hyper.go:13-22) already provides all the
fields needed for paginated collections:

```go
type Representation struct {
    Kind     string                      // Application-defined semantic label
    Self     *Target                     // Canonical target URL
    State    Node                        // Primary application state
    Links    []Link                      // Navigational controls
    Actions  []Action                    // Available state transitions
    Embedded map[string][]Representation // Named nested representations
    Meta     map[string]any              // Application metadata
    Hints    map[string]any              // Codec/UI rendering directives
}
```

- `Embedded["items"]` holds the collection members.
- `Links` with `rel: "next"`, `"prev"`, `"first"`, `"last"` provide
  page navigation.
- `Meta` carries pagination metadata (`total_count`, `current_page`, etc.).

### 2.4 JSON:API Pagination Rels

The `jsonapi/` codec already recognizes pagination link relations and
partitions them to document-level links (jsonapi/mapping.go:218-224):

```go
var paginationRels = map[string]bool{
    "first": true,
    "last":  true,
    "prev":  true,
    "next":  true,
}
```

This confirms that `first`, `last`, `prev`, and `next` are the expected
pagination rels, consistent with IANA link relation registry and RFC 8288.

## 3. Proposal

### 3.1 Embedded Key: `"items"`

Collection representations SHOULD use `"items"` as the `Embedded` map key
for the primary collection of members:

```go
rep := hyper.Representation{
    Kind: "contact-list",
    Embedded: map[string][]Representation{
        "items": contactRepresentations,
    },
}
```

The key `"items"` is chosen because all existing use cases already converge
on this name. Using a consistent key allows generic clients to discover
collection members without application-specific knowledge.

### 3.2 Pagination Links

Paginated collections SHOULD use IANA-registered link relations for page
navigation:

| Rel | Meaning | When to Include |
|---|---|---|
| `next` | Next page of results | When more pages follow the current page |
| `prev` | Previous page of results | When the current page is not the first page |
| `first` | First page of results | Optionally, when not on the first page |
| `last` | Last page of results | Optionally, when the total page count is known |

The absence of a `next` link indicates the current page is the last page.
The absence of a `prev` link indicates the current page is the first page.

```go
if page > 1 {
    rep.Links = append(rep.Links, hyper.Link{
        Rel:   "prev",
        Target: listTarget.WithQuery(url.Values{"page": {strconv.Itoa(page - 1)}}),
        Title: "Previous Page",
    })
}
if page*pageSize < totalCount {
    rep.Links = append(rep.Links, hyper.Link{
        Rel:   "next",
        Target: listTarget.WithQuery(url.Values{"page": {strconv.Itoa(page + 1)}}),
        Title: "Next Page",
    })
}
```

### 3.3 Pagination Metadata

Collection representations MAY include pagination metadata in `Meta`.
The following keys are RECOMMENDED when applicable:

| Key | Type | Description |
|---|---|---|
| `total_count` | integer | Total number of items across all pages |
| `page_size` | integer | Number of items per page |
| `current_page` | integer | Current page number (1-indexed) |
| `page_count` | integer | Total number of pages |

Applications MAY include additional domain-specific metadata alongside
the standard keys. For example, the blog platform includes `status_counts`
to display filter badge counts:

```go
rep.Meta = map[string]any{
    "total_count":   totalCount,
    "current_page":  page,
    "page_size":     pageSize,
    "status_counts": statusCounts,  // application-specific
}
```

All metadata keys are optional. A collection with no pagination metadata
is valid — it simply means the client cannot display page indicators.

### 3.4 Empty Collections

Empty collections SHOULD include an explicit empty array in the `Embedded`
map rather than omitting the key:

```go
rep := hyper.Representation{
    Kind: "contact-list",
    Embedded: map[string][]Representation{
        "items": {},  // explicit empty collection
    },
}
```

**Rationale:** An explicit `"items": []` in the wire format signals to
clients that the collection exists but has no members, as opposed to the
key being absent (which could mean the response is not a collection or the
items were not requested).

Empty collections MAY still include `Actions` (e.g., a "create" action)
and `Links` (e.g., navigation back to a parent resource). An empty
collection is not an error — it is a valid state with available transitions.

### 3.5 Wire Format

A paginated collection renders as follows in JSON:

```json
{
  "kind": "contact-list",
  "self": {"href": "/contacts?page=2"},
  "meta": {
    "current_page": 2,
    "page_size": 100,
    "total_count": 250
  },
  "links": [
    {"rel": "prev", "href": "/contacts?page=1", "title": "Previous Page"},
    {"rel": "next", "href": "/contacts?page=3", "title": "Next Page"}
  ],
  "embedded": {
    "items": [
      {
        "kind": "contact",
        "self": {"href": "/contacts/42"},
        "state": {"name": "Alice", "email": "alice@example.com"}
      }
    ]
  }
}
```

An empty collection:

```json
{
  "kind": "contact-list",
  "self": {"href": "/contacts"},
  "embedded": {
    "items": []
  },
  "actions": [
    {
      "name": "create-contact",
      "method": "POST",
      "href": "/contacts",
      "fields": [
        {"name": "name", "type": "text", "required": true},
        {"name": "email", "type": "email", "required": true}
      ]
    }
  ]
}
```

### 3.6 JSON:API Mapping

The `jsonapi/` codec maps pagination conventions as follows:

- `Embedded["items"]` members become the `data` array in the JSON:API
  document.
- Links with rels `first`, `last`, `prev`, `next` are promoted to
  document-level `links` (per the existing `paginationRels` map in
  jsonapi/mapping.go).
- `Meta` keys are mapped to the document-level `meta` object.

No changes to the `jsonapi/` codec are needed — the existing partitioning
logic already handles these conventions correctly.

## 4. Examples

### 4.1 Paginated Collection (Page 2 of N)

```go
rep := hyper.Representation{
    Kind: "contact-list",
    Self: hyper.Route("contacts.list").WithQuery(url.Values{
        "page": {"2"},
    }).Ptr(),
    Links: []hyper.Link{
        {Rel: "first", Target: hyper.Route("contacts.list"), Title: "First Page"},
        {Rel: "prev", Target: hyper.Route("contacts.list").WithQuery(url.Values{
            "page": {"1"},
        }), Title: "Previous Page"},
        {Rel: "next", Target: hyper.Route("contacts.list").WithQuery(url.Values{
            "page": {"3"},
        }), Title: "Next Page"},
        {Rel: "last", Target: hyper.Route("contacts.list").WithQuery(url.Values{
            "page": {"5"},
        }), Title: "Last Page"},
    },
    Embedded: map[string][]hyper.Representation{
        "items": items,
    },
    Meta: map[string]any{
        "total_count":  500,
        "current_page": 2,
        "page_size":    100,
        "page_count":   5,
    },
}
```

Wire format:

```json
{
  "kind": "contact-list",
  "self": {"href": "/contacts?page=2"},
  "meta": {
    "total_count": 500,
    "current_page": 2,
    "page_size": 100,
    "page_count": 5
  },
  "links": [
    {"rel": "first", "href": "/contacts", "title": "First Page"},
    {"rel": "prev", "href": "/contacts?page=1", "title": "Previous Page"},
    {"rel": "next", "href": "/contacts?page=3", "title": "Next Page"},
    {"rel": "last", "href": "/contacts?page=5", "title": "Last Page"}
  ],
  "embedded": {
    "items": [
      {"kind": "contact", "self": {"href": "/contacts/101"}, "state": {"name": "Alice"}}
    ]
  }
}
```

### 4.2 Empty Collection

```go
rep := hyper.Representation{
    Kind: "contact-list",
    Self: hyper.Route("contacts.list").Ptr(),
    Embedded: map[string][]hyper.Representation{
        "items": {},
    },
    Actions: []hyper.Action{
        {
            Name:   "create-contact",
            Method: "POST",
            Target: hyper.Route("contacts.create"),
            Fields: []hyper.Field{
                {Name: "name", Type: "text", Required: true},
                {Name: "email", Type: "email", Required: true},
            },
        },
    },
}
```

Wire format:

```json
{
  "kind": "contact-list",
  "self": {"href": "/contacts"},
  "embedded": {
    "items": []
  },
  "actions": [
    {
      "name": "create-contact",
      "method": "POST",
      "href": "/contacts",
      "fields": [
        {"name": "name", "type": "text", "required": true},
        {"name": "email", "type": "email", "required": true}
      ]
    }
  ]
}
```

### 4.3 First Page (No prev/first Links)

```go
rep := hyper.Representation{
    Kind: "contact-list",
    Self: hyper.Route("contacts.list").Ptr(),
    Links: []hyper.Link{
        {Rel: "next", Target: hyper.Route("contacts.list").WithQuery(url.Values{
            "page": {"2"},
        }), Title: "Next Page"},
    },
    Embedded: map[string][]hyper.Representation{
        "items": items,
    },
    Meta: map[string]any{
        "current_page": 1,
        "page_size":    100,
    },
}
```

Wire format:

```json
{
  "kind": "contact-list",
  "self": {"href": "/contacts"},
  "meta": {
    "current_page": 1,
    "page_size": 100
  },
  "links": [
    {"rel": "next", "href": "/contacts?page=2", "title": "Next Page"}
  ],
  "embedded": {
    "items": [
      {"kind": "contact", "self": {"href": "/contacts/1"}, "state": {"name": "Bob"}}
    ]
  }
}
```

### 4.4 Last Page (No next/last Links)

```go
rep := hyper.Representation{
    Kind: "contact-list",
    Self: hyper.Route("contacts.list").WithQuery(url.Values{
        "page": {"5"},
    }).Ptr(),
    Links: []hyper.Link{
        {Rel: "first", Target: hyper.Route("contacts.list"), Title: "First Page"},
        {Rel: "prev", Target: hyper.Route("contacts.list").WithQuery(url.Values{
            "page": {"4"},
        }), Title: "Previous Page"},
    },
    Embedded: map[string][]hyper.Representation{
        "items": items,
    },
    Meta: map[string]any{
        "total_count":  500,
        "current_page": 5,
        "page_size":    100,
        "page_count":   5,
    },
}
```

Wire format:

```json
{
  "kind": "contact-list",
  "self": {"href": "/contacts?page=5"},
  "meta": {
    "total_count": 500,
    "current_page": 5,
    "page_size": 100,
    "page_count": 5
  },
  "links": [
    {"rel": "first", "href": "/contacts", "title": "First Page"},
    {"rel": "prev", "href": "/contacts?page=4", "title": "Previous Page"}
  ],
  "embedded": {
    "items": [
      {"kind": "contact", "self": {"href": "/contacts/401"}, "state": {"name": "Eve"}}
    ]
  }
}
```

## 5. Alternatives Considered

### 5.1 Use a Dedicated Collection Type

Define a `Collection` type separate from `Representation` with built-in
pagination fields.

**Pros:**
- Type-safe pagination fields at compile time.
- No need for map-based metadata.

**Cons:**
- Doubles the API surface — renderers, codecs, and clients all need to
  handle two top-level types.
- The existing `Representation` type already handles collections well via
  `Embedded`, `Links`, and `Meta`.
- Every use case successfully used `Representation` for collections without
  needing a separate type.

**Decision:** Collections are representations. The `Embedded["items"]` key
and pagination links/meta distinguish them, not the Go type.

### 5.2 Use `State` for Pagination Metadata

Put `current_page`, `page_size`, etc. in `State` rather than `Meta`.

**Pros:**
- One fewer field to learn — everything is in `State`.

**Cons:**
- `State` is the primary application data (the "resource" content). Mixing
  pagination metadata into `State` conflates transport concerns with domain
  data.
- `Meta` exists precisely for this purpose — metadata about the
  representation that is not part of the resource itself.

**Decision:** Pagination metadata belongs in `Meta`.

### 5.3 Allow Any Embedded Key Name

Do not standardize on `"items"` — let applications choose their own key
(e.g., `"contacts"`, `"posts"`).

**Pros:**
- More descriptive key names.

**Cons:**
- Generic clients cannot discover collection members without knowing the
  application's key name.
- All six use cases already converge on `"items"` — standardizing this
  reflects existing practice.

**Decision:** `"items"` is the RECOMMENDED key. Applications MAY use
additional embedded keys for secondary collections (e.g., sidebar data)
but the primary collection SHOULD use `"items"`.

## 6. Open Questions

1. **Should `page_count` be a RECOMMENDED key?** It is derivable from
   `total_count` and `page_size`, but including it saves clients from
   doing the math. Is the convenience worth the redundancy?

2. **Cursor-based pagination.** The current conventions assume offset-based
   pagination (`page=N`). Should the spec also define conventions for
   cursor-based pagination (e.g., `after=<cursor>`)? The `Meta` and `Links`
   model supports both, but naming conventions for cursor keys are not
   defined.

3. **Should `total_count` be mandatory for collections?** Some data sources
   cannot efficiently compute a total count. The current proposal makes all
   metadata optional, but should collections that *can* provide a total be
   RECOMMENDED to do so?

4. **Filtered collections and metadata scope.** The blog platform's
   `status_counts` in `Meta` is filter-related metadata, not pagination
   metadata. Should the spec distinguish between pagination meta and
   application meta within the `Meta` map, or is a flat namespace
   sufficient?

5. **Server-driven vs. client-driven page size.** Should the spec define a
   mechanism for clients to request a specific page size (e.g., a
   `page_size` query parameter), or is this entirely application-defined?
