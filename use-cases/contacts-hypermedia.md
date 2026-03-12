# Use Case: Contacts App with hyper, dispatch, htmlc, and htmx

This document explores building the Contacts app from *Hypermedia Systems* by Carson Gross et al. using `hyper`, `dispatch`, `htmlc`, and htmx on the frontend. The reference implementation is Python/Flask (`github.com/bigskysoftware/contact-app`). The central question: can `hyper`'s representation model — designed to be media-type-neutral — faithfully reproduce the full range of htmx interaction patterns from the book, including active search, inline validation, bulk operations, lazy loading, and background archive jobs?

## 1. Overview

The Contacts app is the canonical example from *Hypermedia Systems*. It starts as a simple server-rendered CRUD application and progressively enhances each feature with htmx. The book demonstrates that a hypermedia-driven architecture — where the server returns HTML fragments and the browser swaps them into the DOM — can handle sophisticated interactions without client-side JavaScript frameworks.

This use case walks through every feature in the book, building each one with:

- **`hyper`** — representation model with `Representation`, `Action`, `Field`, `Link`, `Hints`, `Embedded`, `Meta` (this repo's spec)
- **`dispatch`** — router with named routes and reverse URL generation via `RouteRef` (§8.1)
- **`htmlc`** — server-side Go template engine using Vue.js SFC (`.vue`) syntax (§15.5)
- **htmx** — frontend library for HTML-over-the-wire interactions

Key properties:

- **HTML-first** — the primary output is server-rendered HTML; JSON is available via content negotiation
- **htmx attributes flow through `Action.Hints`** — the spec's open `Hints` map (§11.4) carries `hx-target`, `hx-swap`, `hx-trigger`, and other htmx attributes
- **`Representation.Kind` maps to `htmlc` component names** — `"contact-list"` renders via `contact-list.vue`
- **Named routes via `dispatch`** — `RouteRef` targets resolve to URLs through `DispatchResolver` (§15.1)
- **Fragment vs. document rendering** — `RenderMode` (§9.4) controls whether the server returns a full page or an HTML fragment for htmx partial requests

## 2. Application Setup

### 2.1 Dispatch Router with Named Routes

The `dispatch` router defines all routes with names for reverse URL generation:

```go
router := dispatch.NewRouter()
router.Get("contacts.list",       "/contacts")
router.Get("contacts.new",        "/contacts/new")
router.Post("contacts.create",    "/contacts/new")
router.Get("contacts.show",       "/contacts/{id}")
router.Get("contacts.edit",       "/contacts/{id}/edit")
router.Post("contacts.update",    "/contacts/{id}/edit")
router.Delete("contacts.delete",  "/contacts/{id}")
router.Get("contacts.email",      "/contacts/{id}/email")
router.Delete("contacts.bulk",    "/contacts/")
router.Get("contacts.count",      "/contacts/count")
router.Post("archive.start",      "/contacts/archive")
router.Get("archive.status",      "/contacts/archive")
router.Get("archive.file",        "/contacts/archive/file")
router.Delete("archive.reset",    "/contacts/archive")
```

### 2.2 DispatchResolver

The `DispatchResolver` (per §15.1) bridges `hyper.Target` route references to resolved URLs:

```go
resolver := DispatchResolver{Router: router}
```

All `Target` values in this document use `RouteRef` for named routes. The resolver converts them to concrete URLs at render time.

### 2.3 htmlc Engine

```go
engine, err := htmlc.New(
    htmlc.WithDirectory("components/"),
    htmlc.WithLayout("layout"),
)
```

Each `Representation.Kind` maps to a `.vue` component file. The `representationToScope` function (§15.5) converts a `hyper.Representation` into the `map[string]any` scope that `htmlc` templates consume.

### 2.4 Renderer with Codecs

```go
renderer := hyper.NewRenderer(
    hyper.WithCodec("text/html", htmlcCodec),
    hyper.WithCodec("application/vnd.api+json", jsonCodec),
)
renderer.Resolver = resolver
```

The `htmlcCodec` wraps the `htmlc.Engine` and uses `Representation.Kind` to select the component. It also checks `RenderMode` (§9.4) to determine whether to render a full document (with layout) or a fragment.

### 2.5 Detecting htmx Partial Requests

The book's Flask app checks the `HX-Trigger` header to decide whether to return a full page or a partial fragment. In `hyper`, this maps to `RenderMode`:

```go
func renderMode(r *http.Request) hyper.RenderMode {
    if r.Header.Get("HX-Request") == "true" {
        return hyper.RenderFragment
    }
    return hyper.RenderDocument
}
```

The htmlc codec uses this mode to decide whether to call `eng.RenderPage` (full document with layout) or `eng.RenderComponent` (fragment only).

## 3. Domain Layer

### 3.1 Contact Type

```go
type Contact struct {
    ID    int
    First string
    Last  string
    Phone string
    Email string
}

type ContactInput struct {
    First string `json:"first"`
    Last  string `json:"last"`
    Phone string `json:"phone"`
    Email string `json:"email"`
}

type ValidationErrors map[string]string

func validateContact(input ContactInput, existingID int) ValidationErrors {
    errs := ValidationErrors{}
    if input.First == "" {
        errs["first"] = "First name is required"
    }
    if input.Last == "" {
        errs["last"] = "Last name is required"
    }
    if input.Email == "" {
        errs["email"] = "Email is required"
    } else if !isValidEmail(input.Email) {
        errs["email"] = "Invalid email address"
    } else if emailTaken(input.Email, existingID) {
        errs["email"] = "Email must be unique"
    }
    return errs
}
```

### 3.2 Shared Field Definitions

Following the shared field pattern from `cli-server.md` §3.2, contact fields are defined once and reused across create, edit, and validation error representations:

```go
var contactFields = []hyper.Field{
    {Name: "first", Type: "text", Label: "First Name", Required: true},
    {Name: "last", Type: "text", Label: "Last Name", Required: true},
    {Name: "phone", Type: "tel", Label: "Phone"},
    {Name: "email", Type: "email", Label: "Email", Required: true},
}
```

### 3.3 Representation Helper Functions

```go
func contactTarget(id int) hyper.Target {
    return hyper.Target{Route: &hyper.RouteRef{
        Name:   "contacts.show",
        Params: map[string]string{"id": strconv.Itoa(id)},
    }}
}

func contactState(c Contact) hyper.Node {
    return hyper.StateFrom(
        "id", c.ID,
        "first", c.First,
        "last", c.Last,
        "phone", c.Phone,
        "email", c.Email,
    )
}

func contactRowRepresentation(c Contact) hyper.Representation {
    return hyper.Representation{
        Kind:  "contact-row",
        Self:  contactTarget(c.ID).Ptr(),
        State: contactState(c),
        Links: []hyper.Link{
            {Rel: "self", Target: contactTarget(c.ID), Title: c.First + " " + c.Last},
            {Rel: "edit", Target: hyper.Target{Route: &hyper.RouteRef{
                Name: "contacts.edit", Params: map[string]string{"id": strconv.Itoa(c.ID)},
            }}, Title: "Edit"},
        },
        Actions: []hyper.Action{
            {
                Name:   "Delete",
                Rel:    "delete",
                Method: "DELETE",
                Target: contactTarget(c.ID),
                Hints: map[string]any{
                    "hx-confirm":  "Are you sure you want to delete this contact?",
                    "hx-target":   "closest tr",
                    "hx-swap":     "outerHTML swap:1s",
                    "confirm":     "Are you sure you want to delete this contact?",
                    "destructive": true,
                },
            },
        },
    }
}
```

The `Hints` map carries both htmx-specific attributes (`hx-confirm`, `hx-target`, `hx-swap`) and generic hints (`confirm`, `destructive`) per §15.6. HTML/htmx codecs use the `hx-*` keys; CLI clients use the generic keys. This dual-hinting pattern is a pragmatic convention — the spec's open `Hints` map (§11.4) supports it without modification.

## 4. Contact List and Search

### 4.1 Contact List Representation

The contacts list page is the app's primary view. It displays contacts in a table with pagination and supports active search via htmx.

```go
func contactListRepresentation(contacts []Contact, page int, q string) hyper.Representation {
    items := make([]hyper.Representation, len(contacts))
    for i, c := range contacts {
        items[i] = contactRowRepresentation(c)
    }

    listTarget := hyper.Target{Route: &hyper.RouteRef{Name: "contacts.list"}}

    rep := hyper.Representation{
        Kind: "contact-list",
        Self: listTarget.Ptr(),
        State: hyper.StateFrom(
            "page", page,
            "query", q,
        ),
        Links: []hyper.Link{
            {Rel: "create", Target: hyper.Target{Route: &hyper.RouteRef{
                Name: "contacts.new",
            }}, Title: "Add Contact"},
        },
        Actions: []hyper.Action{
            {
                Name:   "Search",
                Rel:    "search",
                Method: "GET",
                Target: listTarget,
                Fields: []hyper.Field{
                    {Name: "q", Type: "text", Label: "Search", Value: q},
                },
                Hints: map[string]any{
                    "hx-get":      "",
                    "hx-trigger":  "search, keyup delay:200ms changed",
                    "hx-target":   "tbody",
                    "hx-push-url": "true",
                    "hx-select":   "tbody > tr",
                    "hx-indicator": "#spinner",
                },
            },
            {
                Name:   "Bulk Delete",
                Rel:    "bulk-delete",
                Method: "DELETE",
                Target: hyper.Target{Route: &hyper.RouteRef{Name: "contacts.bulk"}},
                Fields: []hyper.Field{
                    {
                        Name:    "selected_contact_ids",
                        Type:    "checkbox-group",
                        Label:   "Selected Contacts",
                        Options: contactOptions(contacts),
                    },
                },
                Hints: map[string]any{
                    "hx-confirm":  "Are you sure you want to delete these contacts?",
                    "destructive": true,
                },
            },
        },
        Embedded: map[string][]hyper.Representation{
            "items": items,
        },
    }

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

    return rep
}
```

Note on the search action: the `hx-get` hint value is empty because the target URL is resolved from `Action.Target` at render time. The htmlc template constructs the `hx-get` attribute from the resolved target. The `hx-trigger` hint combines the `search` event (triggered by htmx's `hx-trigger` on the search type) with `keyup delay:200ms changed` for active search as the user types.

### 4.2 Partial vs. Full Response

The Flask app checks the `HX-Trigger` header to decide whether to return the full page or just the table rows. In the `hyper` model, this maps to `RenderMode`:

```go
func handleContactList(w http.ResponseWriter, r *http.Request) {
    q := r.URL.Query().Get("q")
    page, _ := strconv.Atoi(r.URL.Query().Get("page"))
    if page < 1 {
        page = 1
    }

    contacts := store.Search(q, page, 100)
    rep := contactListRepresentation(contacts, page, q)

    mode := renderMode(r)
    renderer.RespondWithMode(w, r, http.StatusOK, rep, mode)
}
```

When the request comes from the htmx search input (`HX-Request: true`), the renderer uses `RenderFragment` mode. The htmlc codec then renders only the `contact-list` component body (the table rows) without the surrounding layout. When it is a full page load (browser navigation), `RenderDocument` mode wraps the component in the layout.

The spec provides `Renderer.RespondWithMode` (§10.1) for exactly this purpose — the handler passes the desired `RenderMode` and the codec receives it via `EncodeOptions.Mode`.

### 4.3 JSON Wire Format (Full Page)

```json
{
  "kind": "contact-list",
  "self": {"href": "/contacts"},
  "state": {
    "page": 1,
    "query": ""
  },
  "meta": {
    "current_page": 1,
    "page_size": 100
  },
  "links": [
    {"rel": "create", "href": "/contacts/new", "title": "Add Contact"},
    {"rel": "next", "href": "/contacts?page=2", "title": "Next Page"}
  ],
  "actions": [
    {
      "name": "Search",
      "rel": "search",
      "method": "GET",
      "href": "/contacts",
      "fields": [
        {"name": "q", "type": "text", "label": "Search"}
      ],
      "hints": {
        "hx-get": "",
        "hx-trigger": "search, keyup delay:200ms changed",
        "hx-target": "tbody",
        "hx-push-url": "true",
        "hx-select": "tbody > tr",
        "hx-indicator": "#spinner"
      }
    },
    {
      "name": "Bulk Delete",
      "rel": "bulk-delete",
      "method": "DELETE",
      "href": "/contacts/",
      "fields": [
        {"name": "selected_contact_ids", "type": "checkbox-group", "label": "Selected Contacts", "options": ["..."]}
      ],
      "hints": {
        "hx-confirm": "Are you sure you want to delete these contacts?",
        "destructive": true
      }
    }
  ],
  "embedded": {
    "items": [
      {
        "kind": "contact-row",
        "self": {"href": "/contacts/1"},
        "state": {"id": 1, "first": "Ada", "last": "Lovelace", "phone": "555-0100", "email": "ada@example.com"},
        "links": [
          {"rel": "self", "href": "/contacts/1", "title": "Ada Lovelace"},
          {"rel": "edit", "href": "/contacts/1/edit", "title": "Edit"}
        ],
        "actions": [
          {
            "name": "Delete",
            "rel": "delete",
            "method": "DELETE",
            "href": "/contacts/1",
            "hints": {
              "hx-confirm": "Are you sure you want to delete this contact?",
              "hx-target": "closest tr",
              "hx-swap": "outerHTML swap:1s",
              "confirm": "Are you sure you want to delete this contact?",
              "destructive": true
            }
          }
        ]
      }
    ]
  }
}
```

### 4.4 htmlc Templates

#### `contact-list.vue`

```vue
<!-- components/contact-list.vue -->
<template>
  <form>
    <label for="search">Search</label>
    <input id="search" type="text" name="q"
      :value="query"
      hx-get="/contacts"
      hx-trigger="search, keyup delay:200ms changed"
      hx-target="tbody"
      hx-push-url="true"
      hx-select="tbody > tr"
      hx-indicator="#spinner" />
    <img id="spinner" class="htmx-indicator" src="/static/spinner.gif" />
  </form>

  <form method="DELETE" action="/contacts/">
    <table>
      <thead>
        <tr>
          <th><input type="checkbox" id="select-all" /></th>
          <th>First</th>
          <th>Last</th>
          <th>Phone</th>
          <th>Email</th>
          <th></th>
        </tr>
      </thead>
      <tbody>
        <slot name="items"></slot>
      </tbody>
    </table>
    <button type="submit"
      hx-confirm="Are you sure you want to delete these contacts?"
      hx-delete="/contacts/"
      hx-target="body">
      Delete Selected
    </button>
  </form>

  <p>
    <a :href="createHref">Add Contact</a>
  </p>
</template>
```

#### `contact-row.vue`

```vue
<!-- components/contact-row.vue -->
<template>
  <tr>
    <td><input type="checkbox" name="selected_contact_ids" :value="id" /></td>
    <td>{{ first }}</td>
    <td>{{ last }}</td>
    <td>{{ phone }}</td>
    <td>{{ email }}</td>
    <td>
      <a :href="editHref">Edit</a>
      <a :href="selfHref">View</a>
      <button
        hx-delete=""
        hx-confirm="Are you sure you want to delete this contact?"
        hx-target="closest tr"
        hx-swap="outerHTML swap:1s">
        Delete
      </button>
    </td>
  </tr>
</template>
```

**Hard-coded vs. data-driven htmx attributes:** The htmlc templates above hard-code htmx attributes rather than reading them from `Action.Hints`. This is often the pragmatic choice for app-specific templates — the template author knows the layout and can hard-code attributes for clarity.

However, the updated `representationToScope` (§16.5) now surfaces actions and their hints in the template scope. Templates can use `v-bind` with the `hxAttrs` map to spread `hx-*` attributes from action hints onto elements. The codec resolves `Action.Target` through the `Resolver` and injects the result as `hx-{method}` into `hxAttrs`.

Here is the data-driven alternative for `contact-row.vue`:

```vue
<!-- components/contact-row.vue (data-driven variant) -->
<template>
  <tr>
    <td><input type="checkbox" name="selected_contact_ids" :value="id" /></td>
    <td>{{ first }}</td>
    <td>{{ last }}</td>
    <td>{{ phone }}</td>
    <td>{{ email }}</td>
    <td>
      <a :href="editHref">Edit</a>
      <a :href="selfHref">View</a>
      <button v-bind="actions.delete.hxAttrs">
        {{ actions.delete.name }}
      </button>
    </td>
  </tr>
</template>
```

With the delete action's hints resolved by the codec, `v-bind="actions.delete.hxAttrs"` produces:

```html
<button hx-delete="/contacts/42" hx-confirm="Are you sure you want to delete this contact?" hx-target="closest tr" hx-swap="outerHTML swap:1s">
  Delete
</button>
```

Both patterns — hard-coded and data-driven — are valid and may coexist. Data-driven hints are most valuable when: (1) the same representation serves multiple codecs (HTML + JSON), (2) a generic codec iterates over hints, or (3) attributes vary per record (e.g., confirmation messages). App-specific templates MAY still hard-code htmx attributes for clarity.

## 5. View Contact

### 5.1 Representation

```go
func contactDetailRepresentation(c Contact) hyper.Representation {
    return hyper.Representation{
        Kind:  "contact-detail",
        Self:  contactTarget(c.ID).Ptr(),
        State: contactState(c),
        Links: []hyper.Link{
            {Rel: "list", Target: hyper.Target{Route: &hyper.RouteRef{
                Name: "contacts.list",
            }}, Title: "Back"},
            {Rel: "edit", Target: hyper.Target{Route: &hyper.RouteRef{
                Name: "contacts.edit", Params: map[string]string{"id": strconv.Itoa(c.ID)},
            }}, Title: "Edit"},
        },
        Actions: []hyper.Action{
            {
                Name:   "Delete",
                Rel:    "delete",
                Method: "DELETE",
                Target: contactTarget(c.ID),
                Hints: map[string]any{
                    "hx-confirm":  "Are you sure you want to delete this contact?",
                    "hx-push-url": "true",
                    "hx-target":   "body",
                    "confirm":     fmt.Sprintf("Are you sure you want to delete %s %s?", c.First, c.Last),
                    "destructive": true,
                },
            },
        },
    }
}
```

### 5.2 Handler

```go
func handleContactDetail(w http.ResponseWriter, r *http.Request) {
    id := extractID(r)
    c, err := store.Get(id)
    if err != nil {
        renderError(w, r, err)
        return
    }
    renderer.Respond(w, r, http.StatusOK, contactDetailRepresentation(c))
}
```

### 5.3 htmlc Template

```vue
<!-- components/contact-detail.vue -->
<template>
  <h1>{{ first }} {{ last }}</h1>
  <div>
    <p>Phone: {{ phone }}</p>
    <p>Email: {{ email }}</p>
  </div>
  <a :href="editHref">Edit</a>
  <button
    hx-delete=""
    hx-confirm="Are you sure you want to delete this contact?"
    hx-push-url="true"
    hx-target="body">
    Delete Contact
  </button>
  <a :href="listHref">Back</a>
</template>
```

Data-driven variant using `v-bind` with action hints:

```vue
<!-- components/contact-detail.vue (data-driven variant) -->
<template>
  <h1>{{ first }} {{ last }}</h1>
  <div>
    <p>Phone: {{ phone }}</p>
    <p>Email: {{ email }}</p>
  </div>
  <a :href="editHref">Edit</a>
  <button v-bind="actions.delete.hxAttrs">
    {{ actions.delete.name }}
  </button>
  <a :href="listHref">Back</a>
</template>
```

### 5.4 Delete from Detail Page

When the user clicks "Delete Contact" on the detail page, htmx sends `DELETE /contacts/{id}`. The server must differentiate between an inline delete (from a table row, where the response replaces just the row) and a page-level delete (from the detail page, where the response redirects to the list).

The Flask app checks the `HX-Trigger` header. In `hyper`, the handler inspects the request header:

```go
func handleDeleteContact(w http.ResponseWriter, r *http.Request) {
    id := extractID(r)
    if err := store.Delete(id); err != nil {
        renderError(w, r, err)
        return
    }

    trigger := r.Header.Get("HX-Trigger")
    if trigger == "" || !isHTMXRequest(r) {
        // Full page delete (button click) — redirect to list
        listURL, _ := resolver.ResolveTarget(r.Context(), hyper.Target{
            Route: &hyper.RouteRef{Name: "contacts.list"},
        })
        http.Redirect(w, r, listURL.String(), http.StatusSeeOther)
        return
    }

    // Inline delete (table row) — return empty string, row fades via CSS
    w.WriteHeader(http.StatusOK)
    w.Write([]byte(""))
}
```

**Spec observation:** This handler does not return a `hyper.Representation` for either case — the redirect uses standard HTTP semantics, and the inline delete returns an empty body. The `hyper` model does not need to represent "nothing" or "redirect" as a `Representation`. This is correct: not every HTTP response is a representation. The spec's `Renderer.Respond` is for responses that carry representational state.

## 6. Create Contact

### 6.1 New Contact Form Representation

```go
func newContactFormRepresentation(input ContactInput, errs ValidationErrors) hyper.Representation {
    fields := contactFields
    if len(errs) > 0 {
        fields = hyper.WithErrors(contactFields, map[string]any{
            "first": input.First, "last": input.Last,
            "phone": input.Phone, "email": input.Email,
        }, errs)
    }

    return hyper.Representation{
        Kind: "contact-form",
        Self: hyper.Target{Route: &hyper.RouteRef{Name: "contacts.new"}}.Ptr(),
        Links: []hyper.Link{
            {Rel: "list", Target: hyper.Target{Route: &hyper.RouteRef{
                Name: "contacts.list",
            }}, Title: "Back"},
        },
        Actions: []hyper.Action{
            {
                Name:     "Save",
                Rel:      "create",
                Method:   "POST",
                Target:   hyper.Target{Route: &hyper.RouteRef{Name: "contacts.create"}},
                Consumes: []string{"application/x-www-form-urlencoded"},
                Fields:   fields,
            },
        },
    }
}
```

### 6.2 Handler

```go
func handleNewContact(w http.ResponseWriter, r *http.Request) {
    switch r.Method {
    case http.MethodGet:
        rep := newContactFormRepresentation(ContactInput{}, nil)
        renderer.Respond(w, r, http.StatusOK, rep)

    case http.MethodPost:
        var input ContactInput
        if err := formCodec.Decode(r.Context(), r.Body, &input, hyper.DecodeOptions{
            Request: r,
        }); err != nil {
            http.Error(w, "bad request", http.StatusBadRequest)
            return
        }

        errs := validateContact(input, 0)
        if len(errs) > 0 {
            rep := newContactFormRepresentation(input, errs)
            renderer.Respond(w, r, http.StatusUnprocessableEntity, rep)
            return
        }

        c, err := store.Create(input)
        if err != nil {
            renderError(w, r, err)
            return
        }

        showURL, _ := resolver.ResolveTarget(r.Context(), contactTarget(c.ID))
        http.Redirect(w, r, showURL.String(), http.StatusSeeOther)
    }
}
```

On success, the server redirects to the detail page (303 See Other). With `hx-boost="true"` on the body element, htmx follows the redirect and swaps the response into the body — the same behavior as the Flask app.

### 6.3 Validation Error (JSON Wire Format)

```json
{
  "kind": "contact-form",
  "self": {"href": "/contacts/new"},
  "links": [
    {"rel": "list", "href": "/contacts", "title": "Back"}
  ],
  "actions": [
    {
      "name": "Save",
      "rel": "create",
      "method": "POST",
      "href": "/contacts/new",
      "consumes": ["application/x-www-form-urlencoded"],
      "fields": [
        {"name": "first", "type": "text", "label": "First Name", "value": "Ada", "required": true},
        {"name": "last", "type": "text", "label": "Last Name", "value": "", "required": true, "error": "Last name is required"},
        {"name": "phone", "type": "tel", "label": "Phone", "value": ""},
        {"name": "email", "type": "email", "label": "Email", "value": "taken@example.com", "required": true, "error": "Email must be unique"}
      ]
    }
  ]
}
```

### 6.4 htmlc Template

```vue
<!-- components/contact-form.vue -->
<template>
  <form method="POST" :action="formAction">
    <fieldset>
      <legend>Contact Values</legend>

      <p>
        <label for="first">First Name</label>
        <input id="first" type="text" name="first" :value="firstValue"
          :required="firstRequired" />
        <span class="error" v-if="firstError">{{ firstError }}</span>
      </p>

      <p>
        <label for="last">Last Name</label>
        <input id="last" type="text" name="last" :value="lastValue"
          :required="lastRequired" />
        <span class="error" v-if="lastError">{{ lastError }}</span>
      </p>

      <p>
        <label for="email">Email</label>
        <input id="email" type="email" name="email" :value="emailValue"
          :required="emailRequired"
          hx-get=""
          hx-target="next .error"
          hx-trigger="change, keyup delay:200ms changed" />
        <span class="error" v-if="emailError">{{ emailError }}</span>
      </p>

      <p>
        <label for="phone">Phone</label>
        <input id="phone" type="tel" name="phone" :value="phoneValue" />
        <span class="error" v-if="phoneError">{{ phoneError }}</span>
      </p>

      <button type="submit">Save</button>
    </fieldset>
  </form>
  <a :href="listHref">Back</a>
</template>
```

## 7. Edit Contact

### 7.1 Edit Form Representation

The edit form reuses `contactFields` with `WithValues` to pre-populate current values:

```go
func editContactFormRepresentation(c Contact, input ContactInput, errs ValidationErrors) hyper.Representation {
    var fields []hyper.Field
    if len(errs) > 0 {
        fields = hyper.WithErrors(contactFields, map[string]any{
            "first": input.First, "last": input.Last,
            "phone": input.Phone, "email": input.Email,
        }, errs)
    } else {
        fields = hyper.WithValues(contactFields, map[string]any{
            "first": c.First, "last": c.Last,
            "phone": c.Phone, "email": c.Email,
        })
    }

    editTarget := hyper.Target{Route: &hyper.RouteRef{
        Name: "contacts.update", Params: map[string]string{"id": strconv.Itoa(c.ID)},
    }}

    return hyper.Representation{
        Kind: "contact-form",
        Self: hyper.Target{Route: &hyper.RouteRef{
            Name: "contacts.edit", Params: map[string]string{"id": strconv.Itoa(c.ID)},
        }}.Ptr(),
        State: hyper.StateFrom("id", c.ID),
        Links: []hyper.Link{
            {Rel: "detail", Target: contactTarget(c.ID), Title: "Back"},
        },
        Actions: []hyper.Action{
            {
                Name:     "Save",
                Rel:      "update",
                Method:   "POST",
                Target:   editTarget,
                Consumes: []string{"application/x-www-form-urlencoded"},
                Fields:   fields,
            },
        },
    }
}
```

Note that `Kind: "contact-form"` is the same as the create form. The same `contact-form.vue` template renders both — the only difference is which fields carry pre-populated `Value`s and where the form posts. This is the shared field definition pattern from `cli-server.md` §3.2 in action: one field definition, one template, multiple contexts.

### 7.2 Handler

```go
func handleEditContact(w http.ResponseWriter, r *http.Request) {
    id := extractID(r)
    c, err := store.Get(id)
    if err != nil {
        renderError(w, r, err)
        return
    }

    switch r.Method {
    case http.MethodGet:
        rep := editContactFormRepresentation(c, ContactInput{}, nil)
        renderer.Respond(w, r, http.StatusOK, rep)

    case http.MethodPost:
        var input ContactInput
        if err := formCodec.Decode(r.Context(), r.Body, &input, hyper.DecodeOptions{
            Request: r,
        }); err != nil {
            http.Error(w, "bad request", http.StatusBadRequest)
            return
        }

        errs := validateContact(input, id)
        if len(errs) > 0 {
            rep := editContactFormRepresentation(c, input, errs)
            renderer.Respond(w, r, http.StatusUnprocessableEntity, rep)
            return
        }

        updated, err := store.Update(id, input)
        if err != nil {
            renderError(w, r, err)
            return
        }

        showURL, _ := resolver.ResolveTarget(r.Context(), contactTarget(updated.ID))
        http.Redirect(w, r, showURL.String(), http.StatusSeeOther)
    }
}
```

## 8. Delete Contact

The delete handler (shown in §5.4) differentiates between two contexts:

1. **Inline delete from table row** — htmx sends `DELETE /contacts/{id}` from a button inside a `<tr>`. The `hx-target="closest tr"` and `hx-swap="outerHTML swap:1s"` hints tell htmx to replace the entire row. The server returns an empty body, and the row fades out via the CSS class `tr.htmx-swapping { opacity: 0; transition: opacity 1s ease-out; }`.

2. **Page-level delete from detail view** — htmx sends `DELETE /contacts/{id}` from the detail page button. The server redirects to the list page with `303 See Other`.

The server distinguishes these by checking whether the request is an htmx request. In the Flask app, this is done by checking the `HX-Trigger` header (which contains the ID of the element that triggered the request). The `hyper` handler follows the same approach.

**Spec observation:** The delete response is not a `hyper.Representation` in either case. This is a place where the spec's model does not need to cover every HTTP response. Not every server response carries representational state — redirects and empty bodies are valid HTTP and do not need to be modeled as representations.

## 9. Inline Email Validation

### 9.1 The Pattern

The book adds inline email validation to the contact form. As the user types an email, htmx sends `GET /contacts/{id}/email?email=value` and replaces the error span next to the input.

The htmx attributes on the email input:
- `hx-get="/contacts/{id}/email"` — the validation endpoint
- `hx-target="next .error"` — replace the next sibling with class `.error`
- `hx-trigger="change, keyup delay:200ms changed"` — fire on change and debounced keyup

### 9.2 The Response

The server returns a raw HTML fragment — just a string of text or an error message:

```go
func handleEmailValidation(w http.ResponseWriter, r *http.Request) {
    id := extractID(r)
    email := r.URL.Query().Get("email")

    if email == "" {
        w.Write([]byte(""))
        return
    }
    if !isValidEmail(email) {
        w.Write([]byte("Invalid email address"))
        return
    }
    if emailTaken(email, id) {
        w.Write([]byte("Email must be unique"))
        return
    }
    w.Write([]byte(""))
}
```

### 9.3 Mapping to hyper

This is the first feature that does **not** map cleanly to `hyper.Representation`. The response is a trivial HTML fragment — a single string, not structured state with links and actions. Wrapping it in a `Representation` would be purely ceremonial:

```go
// Possible but over-engineered:
rep := hyper.Representation{
    Kind:  "email-validation",
    State: hyper.StateFrom("error", "Email must be unique"),
}
```

This representation has no `Self`, no `Links`, no `Actions`, no `Embedded`. It exists only to carry a single string. The `Representation` model adds no value here — the response is better served as a raw string.

**Note on non-representation responses:** The spec explicitly addresses this in §10.3 — handlers MAY write responses directly to `http.ResponseWriter` without using `Renderer.Respond` for trivial fragments, empty responses, binary content, and redirects. This is the correct boundary: the `hyper` model is for structured representations, and not every HTTP response is one.

## 10. Pagination

The book demonstrates three pagination patterns: traditional pagination, click-to-load, and infinite scroll.

### 10.1 Traditional Pagination

The basic pattern uses `?page=N` query parameters with 100 contacts per page. The contact list representation (§4.1) includes `State` with the current page and a `next` link when more pages exist.

### 10.2 Click-to-Load

The click-to-load pattern replaces the "Next Page" link with a button that loads additional rows into the existing table:

```go
func loadMoreRepresentation(page int) hyper.Representation {
    return hyper.Representation{
        Kind: "load-more",
        Actions: []hyper.Action{
            {
                Name:   "Load More",
                Rel:    "load-more",
                Method: "GET",
                Target: hyper.Target{Route: &hyper.RouteRef{
                    Name:  "contacts.list",
                    Query: url.Values{"page": {strconv.Itoa(page + 1)}},
                }},
                Hints: map[string]any{
                    "hx-target": "closest tr",
                    "hx-swap":   "outerHTML",
                    "hx-select": "tbody > tr",
                },
            },
        },
        State: hyper.StateFrom("page", page+1),
    }
}
```

The "Load More" button lives in a `<tr>` at the bottom of the table. When clicked, htmx sends `GET /contacts?page=2` and replaces the `<tr>` containing the button with the new rows (selected via `hx-select="tbody > tr"`). This is the htmx pattern from the book — the response is the full contacts page, but `hx-select` extracts only the `<tr>` elements from the `<tbody>`.

The template:

```vue
<!-- components/load-more.vue -->
<template>
  <tr>
    <td colspan="6">
      <button
        hx-get="/contacts?page={{ nextPage }}"
        hx-target="closest tr"
        hx-swap="outerHTML"
        hx-select="tbody > tr">
        Load More
      </button>
    </td>
  </tr>
</template>
```

The server embeds the load-more representation as the last item:

```go
if len(contacts) == 100 {
    items = append(items, loadMoreRepresentation(page))
}
```

### 10.3 Infinite Scroll

The infinite scroll variant is identical to click-to-load except the trigger changes from a click to `revealed` — the element triggers a load when it becomes visible in the viewport:

```go
Hints: map[string]any{
    "hx-target":  "closest tr",
    "hx-swap":    "outerHTML",
    "hx-select":  "tbody > tr",
    "hx-trigger": "revealed",
},
```

The template:

```vue
<!-- components/scroll-sentinel.vue -->
<template>
  <tr>
    <td colspan="6">
      <span
        hx-get="/contacts?page={{ nextPage }}"
        hx-target="closest tr"
        hx-swap="outerHTML"
        hx-select="tbody > tr"
        hx-trigger="revealed">
        Loading...
      </span>
    </td>
  </tr>
</template>
```

**Observation:** The click-to-load and infinite scroll patterns use the same underlying mechanism — only the trigger differs. In the `hyper` model, this is captured entirely in `Action.Hints`. The representation structure is identical; the behavioral difference lives in a single hint key-value pair. This validates the design of `Hints` as an open map — htmx's trigger vocabulary is expressive, and `Hints` passes it through without needing to model each trigger type.

## 11. Lazy Loading

### 11.1 The Pattern

The book demonstrates lazy loading a contact count. The count is expensive to compute, so the page loads immediately with a placeholder and then fetches the count asynchronously:

```html
<span hx-get="/contacts/count" hx-trigger="load">
  <img class="htmx-indicator" src="/static/spinner.gif" />
</span>
```

When the element loads, htmx sends `GET /contacts/count` and replaces the `<span>` with the response.

### 11.2 Count Representation

```go
func handleContactCount(w http.ResponseWriter, r *http.Request) {
    count := store.Count()

    rep := hyper.Representation{
        Kind:  "contact-count",
        State: hyper.StateFrom("count", count),
    }

    renderer.Respond(w, r, http.StatusOK, rep)
}
```

The template:

```vue
<!-- components/contact-count.vue -->
<template>
  <span>({{ count }} total Contacts)</span>
</template>
```

This is a borderline case for `hyper.Representation`. Like the email validation endpoint (§9), the response is a trivial fragment. However, the count representation does carry structured state (`count`) and could plausibly carry links (e.g., a link to the contacts list). Using a `Representation` here is not over-engineered — it is a minimal but valid use of the model.

### 11.3 Integration in Contact List Template

The contact list template includes the lazy-loaded count:

```vue
<!-- In contact-list.vue -->
<p>
  <span hx-get="/contacts/count" hx-trigger="load">
    <img class="htmx-indicator" src="/static/spinner.gif" />
  </span>
</p>
```

This is pure template-side htmx — the parent representation does not need to know about the lazy-loaded count. The template author adds the `hx-get` and `hx-trigger` attributes directly. This is an example where htmx attributes on the template side are independent of the `hyper` model — the count endpoint is a separate resource, not an embedded representation.

**Design choice:** Should the contact list representation include a `Link` to the count endpoint? It could:

```go
Links: []hyper.Link{
    {Rel: "count", Target: hyper.Target{Route: &hyper.RouteRef{
        Name: "contacts.count",
    }}, Title: "Contact Count"},
}
```

This would make the count endpoint discoverable via the representation. For a pure HTML/htmx app, this is unnecessary — the template knows the URL. For machine clients (CLI, mobile), a `count` link on the list representation would be useful. This is a judgment call that depends on whether the API serves non-HTML clients.

## 12. Archive / Download

### 12.1 Overview

The archive feature is the most complex interaction in the book. It demonstrates an asynchronous operation with a three-state UI:

1. **Waiting** — no archive in progress; show a "Download Contact Archive" button
2. **Running** — archive generation in progress; show a progress bar that polls for updates
3. **Complete** — archive ready; show a download link and a "Clear" button

The server manages an `Archiver` object that tracks the state of the background job. htmx polls for progress updates using `hx-trigger="load delay:500ms"`.

### 12.2 Archive Resource Representation

The archive is modeled as a resource with state — this maps directly to `hyper`'s async action conventions (§7.2). The archive has a status (`waiting`, `running`, `complete`) and a progress percentage.

```go
type Archiver struct {
    Status   string  // "waiting", "running", "complete"
    Progress float64 // 0.0 to 1.0
}

func archiveRepresentation(a *Archiver) hyper.Representation {
    rep := hyper.Representation{
        Kind: "archive-ui",
        Self: hyper.Target{Route: &hyper.RouteRef{Name: "archive.status"}}.Ptr(),
        State: hyper.StateFrom(
            "status", a.Status,
            "progress", a.Progress,
        ),
        Meta: map[string]any{
            "id": "archive-progress",
        },
    }

    switch a.Status {
    case "waiting":
        rep.Actions = []hyper.Action{
            {
                Name:   "Download Contact Archive",
                Rel:    "start-archive",
                Method: "POST",
                Target: hyper.Target{Route: &hyper.RouteRef{Name: "archive.start"}},
                Hints: map[string]any{
                    "async": true,
                },
            },
        }
    case "running":
        rep.Meta["poll-interval"] = 1
        rep.Hints = map[string]any{
            "hx-trigger": "load delay:500ms",
            "hx-target":  "#archive-progress",
            "hx-swap":    "outerHTML",
        }
        rep.Actions = []hyper.Action{
            {
                Name:   "Cancel",
                Rel:    "cancel-archive",
                Method: "DELETE",
                Target: hyper.Target{Route: &hyper.RouteRef{Name: "archive.reset"}},
            },
        }
    case "complete":
        rep.Links = []hyper.Link{
            {
                Rel:    "download",
                Target: hyper.Target{Route: &hyper.RouteRef{Name: "archive.file"}},
                Title:  "Download Archive",
                Type:   "application/octet-stream",
            },
        }
        rep.Actions = []hyper.Action{
            {
                Name:   "Clear",
                Rel:    "clear-archive",
                Method: "DELETE",
                Target: hyper.Target{Route: &hyper.RouteRef{Name: "archive.reset"}},
            },
        }
    }

    return rep
}
```

### 12.3 Handlers

```go
func handleArchiveStart(w http.ResponseWriter, r *http.Request) {
    archiver.Start() // launches background goroutine
    rep := archiveRepresentation(archiver)
    renderer.Respond(w, r, http.StatusOK, rep)
}

func handleArchiveStatus(w http.ResponseWriter, r *http.Request) {
    rep := archiveRepresentation(archiver)
    renderer.Respond(w, r, http.StatusOK, rep)
}

func handleArchiveFile(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/octet-stream")
    w.Header().Set("Content-Disposition", "attachment; filename=contacts.json")
    archiver.WriteTo(w)
}

func handleArchiveReset(w http.ResponseWriter, r *http.Request) {
    archiver.Reset()
    rep := archiveRepresentation(archiver)
    renderer.Respond(w, r, http.StatusOK, rep)
}
```

### 12.4 htmlc Template

```vue
<!-- components/archive-ui.vue -->
<template>
  <div id="archive-progress">
    <div v-if="status === 'waiting'">
      <button
        hx-post="/contacts/archive"
        hx-target="#archive-progress"
        hx-swap="outerHTML">
        Download Contact Archive
      </button>
    </div>

    <div v-if="status === 'running'"
      hx-get="/contacts/archive"
      hx-trigger="load delay:500ms"
      hx-target="#archive-progress"
      hx-swap="outerHTML">
      <progress :value="progress" max="1"></progress>
      <button
        hx-delete="/contacts/archive"
        hx-target="#archive-progress"
        hx-swap="outerHTML">
        Cancel
      </button>
    </div>

    <div v-if="status === 'complete'">
      <a :href="downloadHref" hx-boost="false">Download Archive</a>
      <button
        hx-delete="/contacts/archive"
        hx-target="#archive-progress"
        hx-swap="outerHTML">
        Clear
      </button>
    </div>
  </div>
</template>
```

Key observations:

- **`id="archive-progress"`** — the stable ID ensures smooth CSS transitions as htmx swaps the element. This is carried in `Meta` on the representation. The template uses it as the container ID.
- **`hx-trigger="load delay:500ms"`** — on the "running" state div, this tells htmx to re-fetch the archive status after a 500ms delay every time the element loads. This creates the polling loop. These htmx attributes are carried in `Representation.Hints` (not `Meta`).
- **`hx-boost="false"`** — on the download link, this opts out of htmx's boost behavior so the browser handles the file download natively.
- **Three-state representation** — the representation's `State.status` drives which actions and links are available. This aligns with the spec's async action conventions (§7.2): the `status` state key and dynamic `Links` (download link appears only when complete).

### 12.5 How This Maps to hyper's Async Conventions

The archive feature maps well to the async action pattern from §7.2:

| Spec Convention | Archive Implementation |
|---|---|
| `Action.Hints["async"]: true` | Set on the "start archive" action |
| `State["status"]` with recommended values | Uses `"waiting"`, `"running"`, `"complete"` (spec recommends `"pending"`, `"processing"`, `"complete"`, `"failed"`) |
| `Meta["poll-interval"]` | Set to `1` during running state |
| Dynamic `Links` based on status | `download` link appears only when complete |
| `Action` with `rel: "retry"` for failures | Could add a retry action if archive generation fails |

The status values diverge slightly from the spec's recommendations (`waiting` vs `pending`, `running` vs `processing`). This is fine — the spec says "APIs MAY use additional status values beyond this recommended set." But standardizing on the recommended values would improve interoperability with generic clients.

### 12.6 Polling via htmx vs. Meta

The htmx polling mechanism (`hx-trigger="load delay:500ms"`) is independent of the spec's `Meta["poll-interval"]` convention. The htmx attributes are for the HTML/htmx client; the `Meta["poll-interval"]` is for machine clients (CLI, mobile) that poll by re-fetching the resource.

The spec now provides `Representation.Hints` (§4.1) for exactly this separation. The htmx rendering directives (`hx-trigger`, `hx-target`, `hx-swap`) live in `Hints`, while codec-agnostic metadata (`poll-interval`) remains in `Meta`. This parallels the existing `Action.Hints` pattern and keeps `Meta` focused on application-specific metadata.

## 13. Spec Feedback

This section lists gaps, ambiguities, and suggestions discovered while working through the Contacts app.

### 13.1 RenderMode Not Exposed by Renderer.Respond

**Resolved.** The spec now provides `Renderer.RespondWithMode(w, r, status, rep, mode)` (§10.1). This is the most explicit option — handlers call `RespondWithMode` with `RenderFragment` for htmx partial requests and `RenderDocument` for full page loads. The mode is passed through to the codec via `EncodeOptions.Mode` (§9.4). See §4.2 in this document for the updated usage.

### 13.2 Trivial Fragment Responses

**Resolved.** The spec now explicitly addresses this in §10.3 (Non-Representation Responses). It acknowledges that handlers MAY write responses directly to `http.ResponseWriter` without using `Renderer.Respond` for trivial fragment responses, empty responses, binary content, and redirects. This is the correct boundary — `Renderer` is for structured `Representation` values, and not every HTTP response is one.

### 13.3 Meta as Rendering Directive Container

**Resolved.** The spec now includes `Representation.Hints` (§4.1) — a `map[string]any` field on `Representation` that parallels `Action.Hints`. Rendering directives (`hx-trigger`, `hx-target`, `hx-swap`) now belong in `Hints`, while `Meta` is scoped to application-specific metadata (e.g., `poll-interval`, pagination totals). The archive feature code in §12.2 has been updated to use `Hints` for htmx attributes.

### 13.4 Redirect Responses

**Observation.** Several handlers respond with `http.Redirect` rather than `Renderer.Respond` — create-then-redirect, delete-then-redirect. The spec's `Renderer` does not model redirects. This is correct (redirects are not representations), but the interaction between `hx-boost` and server-side redirects is worth noting. When `hx-boost="true"` is active, htmx follows 303 redirects and swaps the response body — the server does not need to do anything special. The `hyper` model handles this correctly by not modeling it at all.

### 13.5 Bulk Operations

**Resolved.** The spec now defines `checkbox-group` and `multiselect` field types in §7.3.1. These types indicate that the field accepts zero or more values, with multiple `Option` entries having `Selected: true` simultaneously. The bulk delete action in §4.1 now uses `checkbox-group` for the `selected_contact_ids` field. Codecs render `checkbox-group` as a group of checkboxes and `multiselect` as a `<select multiple>` element.

### 13.6 Action.Hints vs. Template-Authored Attributes

**Resolved.** The updated `representationToScope` (§16.5) now surfaces actions, links, and hints in the template scope. Each action's `hx-*` hints are extracted into an `hxAttrs` map that templates can spread onto elements using `v-bind="actions.{rel}.hxAttrs"`. The codec resolves `Action.Target` and injects the resolved URL as `hx-{method}` into `hxAttrs` before rendering.

Both patterns coexist: app-specific templates MAY hard-code htmx attributes for clarity, and MAY use data-driven `v-bind` spreading when attributes vary per record or when the same representation serves multiple codecs. See §4.4 and §5.3 in this document for data-driven template variants alongside the hard-coded originals.

### 13.7 Pagination Standardization

**Resolved.** The spec now defines recommended pagination conventions in two areas:

1. **Pagination `Meta` keys** (§4.1): `total_count`, `page_size`, `page_count`, `current_page` — all optional, supporting both offset-based and cursor-based pagination.
2. **Pagination link relations** (§5.3): IANA-registered `next`, `prev`, `first`, `last` rels per RFC 8288. The absence of a `next` link indicates the last page.

The contact list representation in §4.1 now uses these conventions: `Meta` carries `current_page` and `page_size`, and links use `next`/`prev` rels with `RouteRef.Query` for page parameters.

### 13.8 Stable Element IDs for htmx Swap Continuity

**Observation.** The archive feature relies on a stable `id="archive-progress"` attribute for smooth CSS transitions during htmx swaps. This ID is carried in `Meta["id"]` in the representation. There is no spec convention for element IDs. For htmx applications, stable IDs are critical for swap targeting and CSS transition continuity.

**Suggestion.** Consider a recommended `Meta["element-id"]` key or a top-level `ID` field on `Representation` for codecs that render to HTML. This would formalize a common need without cluttering the core model.

### 13.9 HX-Trigger Response Header

**Gap.** Several htmx patterns in the book use the `HX-Trigger` response header to trigger client-side events after a swap. For example, after a successful contact creation, the server could send `HX-Trigger: contacts-updated` to trigger a refresh of the contact list on other parts of the page. The `hyper` model has no way to express response headers — `Meta` could carry them, but it is not clear how a codec would extract `Meta` keys and set HTTP headers.

**Suggestion.** This may be out of scope for the representation model. HTTP response headers are a transport concern, not a representation concern. Handlers can set headers directly before calling `Renderer.Respond`. But the pattern is common enough in htmx applications that it is worth documenting as a recommended practice.

### 13.10 Form Wrapping for Bulk Operations

**Observation.** The bulk delete feature wraps the entire contacts table in a `<form>` element so that checkboxes in individual rows contribute their values to the form submission. In the `hyper` model, the `Action` for bulk delete lives on the contact list representation, and the checkboxes live on the embedded contact row representations. The relationship between a parent representation's action and the form fields contributed by embedded representations is implicit — there is no `hyper` mechanism to express "this action collects fields from embedded items."

This is acceptable for template-based rendering (the template author knows to wrap the table in a form), but it is a gap for generic codecs that would need to infer the form boundary.

### 13.11 RouteRef Target with Query Parameters

**Resolved.** The `RouteRef` struct (§8.1) now includes a `Query url.Values` field. When both `Params` (path parameters) and `Query` (query parameters) are present, the resolver first resolves the path using `Params`, then appends `Query` as the URL query string. Pagination links in §4.1 and the load-more action in §10.2 now use `RouteRef.Query` to express `?page=N` without manual URL construction.

### 13.12 Summary of Gaps

| # | Gap | Severity | Section | Status |
|---|---|---|---|---|
| 1 | `Renderer.Respond` does not accept `RenderMode` | High — blocks basic htmx usage | §13.1 | **Resolved** — `RespondWithMode` added (§10.1) |
| 2 | No acknowledgment of non-representation responses | Low — documentation | §13.2 | **Resolved** — §10.3 documents non-representation responses |
| 3 | `Meta` overloaded for rendering directives | Medium — design clarity | §13.3 | **Resolved** — `Representation.Hints` added (§4.1) |
| 4 | Bulk operations / multi-value fields | Medium — missing field type | §13.5 | **Resolved** — `checkbox-group` and `multiselect` types added (§7.3.1) |
| 5 | Pagination convention | Medium — interoperability | §13.7 | **Resolved** — pagination `Meta` keys and IANA link rels defined (§4.1, §5.3) |
| 6 | `RouteRef` lacks query parameter support | Medium — common need | §13.11 | **Resolved** — `RouteRef.Query` added (§8.1) |
| 7 | `Action.Hints` not surfaced to templates | Medium — data-driven hints | §13.6 | **Resolved** — `representationToScope` surfaces actions/hints; `v-bind` spreads `hxAttrs` (§16.5) |
| 8 | No `HX-Trigger` response header convention | Low — transport concern | §13.9 | Open |
| 9 | Form boundary for embedded item fields | Low — template concern | §13.10 | Open |
