# Proposal 003: Mutation Response Conventions

- **Status:** Draft
- **Date:** 2026-03-17
- **Author:** hyper contributors

## 1. Problem Statement

Every use-case exploration has independently decided what to return after
state-changing operations (POST, PUT, DELETE). The CLI server returns created
resources with 201. The contacts app uses `http.Redirect` after creates and
deletes. The blog platform returns updated representations with new action sets
after state transitions and embeds success notifications in `Meta`. The REST
CLI assumes it will receive the created or updated resource for display.

Without conventions:

- Generic clients (CLIs, navigators, AI agents) cannot predict what a mutation
  response will contain — they must guess whether to display a resource, follow
  a redirect, or show a confirmation message.
- Server authors make inconsistent choices about status codes, response bodies,
  and redirect behavior, leading to interoperability problems.
- The `Client.Submit` and `Navigator.SubmitAction` methods decode the response
  into a `Representation` and update the navigator's current position, but
  there is no guidance on what that representation should look like for
  different mutation types.

## 2. Background

### 2.1 Mutation Patterns Found in Use Cases

The following table summarizes mutation response patterns discovered across
use-case explorations:

| Use Case | Operation | Status | Response Body | Navigation |
|---|---|---|---|---|
| cli-server (create) | POST | 201 | Created resource | `Self` link |
| cli-server (update) | PUT | 200 | Updated resource | `Self` link |
| cli-server (delete) | DELETE | 200 | `kind: "deleted"`, message | `rel: "contacts"` link |
| contacts-hypermedia (create) | POST | 303 | — (redirect) | `Location` header |
| contacts-hypermedia (delete) | DELETE | 303 | — (redirect) | `Location` header |
| blog-platform (trash) | POST | 200 | Updated post with new actions | Actions reflect trashed state |
| blog-platform (restore) | POST | 200 | Updated post with draft actions | Actions reflect draft state |
| blog-platform (permanent delete) | DELETE | 200 | Post list + notification | `Meta["notification"]` |
| rest-cli (create) | POST | 201 | Created resource | Displayed to user |
| rest-cli (update) | PUT | 200 | Updated resource | Displayed to user |
| rest-cli (delete) | DELETE | 200 | Confirmation message | Displayed to user |
| agent-streaming (async) | POST | 202 | Pending representation | `Meta["poll-interval"]` |

### 2.2 Redirect-After-POST Pattern

The contacts-hypermedia use case uses `http.Redirect` with 303 See Other after
mutations. This is the standard Post/Redirect/Get (PRG) pattern for HTML
browser clients. When `hx-boost="true"` is active, htmx follows 303 redirects
transparently and swaps the response body.

The hyper `Renderer` does not model redirects — and correctly so, since
redirects are not representations. However, this means `Client.Submit` and
`Navigator.SubmitAction` will follow the redirect and decode whatever the
redirect target returns. The client sees the final representation, not the
redirect itself.

### 2.3 Navigator Behavior After Mutations

`Navigator.SubmitAction` (navigator.go:87) pushes the current position onto
the history stack and updates the current position to the mutation response.
This means the response representation becomes the navigator's new "location".
For this to work well, mutation responses should be meaningful, self-describing
representations — not empty bodies or opaque status codes.

### 2.4 Async Mutation Pattern

The agent-streaming use case demonstrates async mutations: a POST returns 202
Accepted with a pending representation containing `Meta["poll-interval"]`.
The client polls until the operation completes. For streaming clients, the
same POST returns SSE events with progressive updates.

## 3. Proposal

### 3.1 Create (POST) — 201 Created

Successful resource creation SHOULD return HTTP 201 Created with a full
representation of the created resource. The `Self` link provides the canonical
URL of the new resource.

```go
created := hyper.Representation{
    Kind: "contact",
    Self: hyper.Route("contact", contactID),
    State: hyper.Object{
        "id":    hyper.Scalar{V: contactID},
        "name":  hyper.Scalar{V: "Alan Turing"},
        "email": hyper.Scalar{V: "alan@example.com"},
    },
    Links: []hyper.Link{
        {Rel: "collection", Target: hyper.Route("contacts"), Title: "All Contacts"},
    },
    Actions: []hyper.Action{
        {Name: "edit-contact", Rel: "edit", Method: "PUT", Target: hyper.Route("contact", contactID), Fields: editFields},
        {Name: "delete-contact", Rel: "delete", Method: "DELETE", Target: hyper.Route("contact", contactID)},
    },
}
renderer.Respond(w, r, http.StatusCreated, created)
```

The HTTP `Location` header SHOULD also be set to the created resource's URL,
per HTTP semantics (RFC 9110 §15.3.2). The `Renderer` or server handler is
responsible for setting this header.

**Rationale:** Returning the full resource avoids a follow-up GET. The `Self`
link lets clients navigate to the created resource. Actions on the response
tell the client what it can do next.

### 3.2 Update (PUT/PATCH) — 200 OK

Successful updates SHOULD return HTTP 200 OK with a full representation of the
updated resource. The representation reflects the resource's new state,
including any server-computed fields.

```go
updated := hyper.Representation{
    Kind: "contact",
    Self: hyper.Route("contact", contactID),
    State: hyper.Object{
        "id":    hyper.Scalar{V: contactID},
        "name":  hyper.Scalar{V: "Alan M. Turing"},
        "email": hyper.Scalar{V: "alan@example.com"},
    },
    Actions: []hyper.Action{
        {Name: "edit-contact", Rel: "edit", Method: "PUT", Target: hyper.Route("contact", contactID), Fields: editFields},
        {Name: "delete-contact", Rel: "delete", Method: "DELETE", Target: hyper.Route("contact", contactID)},
    },
}
renderer.Respond(w, r, http.StatusOK, updated)
```

**Rationale:** The client already has the resource; the response confirms
what changed and provides the updated action set.

### 3.3 Delete (DELETE) — 200 OK with Navigation

Successful deletion SHOULD return HTTP 200 OK with a representation that
confirms the deletion and provides navigation links. The representation
SHOULD use `Kind: "deleted"` to signal that the resource no longer exists.

```go
deleted := hyper.Representation{
    Kind: "deleted",
    State: hyper.Object{
        "message": hyper.Scalar{V: "Contact deleted"},
    },
    Links: []hyper.Link{
        {Rel: "collection", Target: hyper.Route("contacts"), Title: "All Contacts"},
    },
}
renderer.Respond(w, r, http.StatusOK, deleted)
```

Servers MAY alternatively return HTTP 204 No Content with no body, but this
is discouraged because:

- `Navigator.SubmitAction` updates the current position to the response;
  an empty representation leaves the navigator in a dead-end state.
- CLI and agent clients cannot display a confirmation or offer navigation.
- The `"deleted"` representation with links lets clients continue browsing.

For bulk operations or cases where the parent collection should be shown,
servers MAY return the parent collection representation instead:

```go
rep := contactListRepresentation(remainingContacts)
rep.Meta = map[string]any{
    "notification": map[string]any{
        "type":    "success",
        "message": "Contact deleted",
    },
}
renderer.Respond(w, r, http.StatusOK, rep)
```

### 3.4 State Transitions — 200 OK with Updated Resource

When a mutation changes a resource's state (e.g., draft → published,
active → trashed), the server SHOULD return HTTP 200 OK with the resource
in its new state. The response MUST include the updated action set that
reflects the new state — this is how clients discover what transitions are
now available.

```go
// After trashing a post
post := hyper.Representation{
    Kind: "post",
    Self: hyper.Route("post", postID),
    State: hyper.Object{
        "id":     hyper.Scalar{V: postID},
        "title":  hyper.Scalar{V: "My Post"},
        "status": hyper.Scalar{V: "trashed"},
    },
    Actions: []hyper.Action{
        // Only actions valid in "trashed" state
        {Name: "restore-post", Rel: "restore", Method: "POST", Target: hyper.Route("restore-post", postID)},
        {Name: "delete-post", Rel: "delete", Method: "DELETE", Target: hyper.Route("post", postID),
            Hints: map[string]any{"confirm": "Permanently delete this post?", "destructive": true}},
    },
}
renderer.Respond(w, r, http.StatusOK, post)
```

**Rationale:** The action set is the authoritative signal for available
transitions. A trashed post loses "publish" and "unpublish" actions and gains
"restore" and "permanent delete". The client does not need to know the state
machine — it discovers valid transitions from the actions present.

### 3.5 Validation Failure — 422 with Field Errors

When a mutation fails validation, the server SHOULD return HTTP 422
Unprocessable Entity with the action's representation, preserving submitted
values and populating `Field.Error` for each invalid field. This convention
is defined in Proposal 001 §3.3 and is affirmed here.

```go
fields := hyper.WithErrors(action.Fields, submittedValues, map[string]string{
    "email": "Invalid email address",
    "name":  "Name is required",
})
rep := hyper.Representation{
    Kind: "contact-form",
    Actions: []hyper.Action{
        {Name: "create-contact", Method: "POST", Target: hyper.Route("contacts"), Fields: fields},
    },
}
renderer.Respond(w, r, http.StatusUnprocessableEntity, rep)
```

The representation uses the same `Kind` as the original form so that clients
can re-render it in place with inline error annotations.

### 3.6 Redirect-After-POST — 303 See Other

When serving HTML browser clients, servers MAY use HTTP 303 See Other with
a `Location` header instead of returning a representation body. This is the
standard Post/Redirect/Get (PRG) pattern.

```go
http.Redirect(w, r, "/contacts", http.StatusSeeOther)
```

This is appropriate when:

- The client is a browser or htmx-enhanced page.
- The desired outcome is to display a different resource (e.g., the collection
  after creating an item).

This is NOT appropriate when:

- The client is a CLI or programmatic client that expects to receive the
  mutation result directly.
- The server uses `Renderer.Respond`, which produces a representation body.

Servers that need to support both browser and API clients SHOULD use content
negotiation: return 303 for `Accept: text/html` and a representation body
(201, 200, etc.) for `Accept: application/json`.

**Note:** `Client.Submit` follows redirects via Go's `http.Client`. The
client will see the final representation returned by the redirect target,
not the 303 itself. This is transparent and requires no special handling.

### 3.7 Async Mutations — 202 Accepted

When a mutation cannot complete synchronously, the server SHOULD return HTTP
202 Accepted with a representation describing the pending operation.

```go
pending := hyper.Representation{
    Kind: "message",
    Self: hyper.Route("message", messageID),
    State: hyper.Object{
        "id":     hyper.Scalar{V: messageID},
        "status": hyper.Scalar{V: "generating"},
        "content": hyper.Scalar{V: ""},
    },
    Meta: map[string]any{
        "poll-interval": "2s",
    },
}
renderer.Respond(w, r, http.StatusAccepted, pending)
```

The `Meta["poll-interval"]` value tells polling clients how frequently to
re-fetch the resource. When the operation completes, the resource transitions
to its final state and no longer includes `poll-interval`.

For clients that request `Accept: text/event-stream`, the server MAY stream
progressive updates as SSE events instead of returning a polling
representation. Each event contains a full representation with accumulated
state. The final event contains the completed resource with all actions.
See the agent-streaming use case for the full pattern.

### 3.8 Success Notifications

When a mutation response returns a collection or a different resource (rather
than the mutated resource itself), servers MAY include a success notification
in `Meta["notification"]`:

```go
rep.Meta = map[string]any{
    "notification": map[string]any{
        "type":    "success",
        "message": fmt.Sprintf("%q has been permanently deleted.", post.Title),
    },
}
```

This is particularly useful for delete operations that return the parent
collection, or state transitions that redirect to a list view. HTML clients
can render the notification as a toast or banner. CLI clients can print the
message before displaying the resource.

## 4. Examples

### 4.1 Create — 201 Created

```
HTTP/1.1 201 Created
Content-Type: application/json
Location: /contacts/3
```

```json
{
  "kind": "contact",
  "self": "/contacts/3",
  "state": {
    "id": 3,
    "name": "Alan Turing",
    "email": "alan@example.com"
  },
  "links": [
    {"rel": "collection", "href": "/contacts", "title": "All Contacts"}
  ],
  "actions": [
    {
      "name": "edit-contact",
      "rel": "edit",
      "method": "PUT",
      "href": "/contacts/3",
      "fields": [
        {"name": "name", "type": "text", "required": true, "value": "Alan Turing"},
        {"name": "email", "type": "email", "required": true, "value": "alan@example.com"}
      ]
    },
    {
      "name": "delete-contact",
      "rel": "delete",
      "method": "DELETE",
      "href": "/contacts/3"
    }
  ]
}
```

### 4.2 Update — 200 OK

```
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
  "kind": "contact",
  "self": "/contacts/3",
  "state": {
    "id": 3,
    "name": "Alan M. Turing",
    "email": "alan@example.com"
  },
  "actions": [
    {
      "name": "edit-contact",
      "rel": "edit",
      "method": "PUT",
      "href": "/contacts/3",
      "fields": [
        {"name": "name", "type": "text", "required": true, "value": "Alan M. Turing"},
        {"name": "email", "type": "email", "required": true, "value": "alan@example.com"}
      ]
    },
    {
      "name": "delete-contact",
      "rel": "delete",
      "method": "DELETE",
      "href": "/contacts/3"
    }
  ]
}
```

### 4.3 Delete — 200 OK with Navigation

```
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
  "kind": "deleted",
  "state": {
    "message": "Contact deleted"
  },
  "links": [
    {"rel": "collection", "href": "/contacts", "title": "All Contacts"}
  ]
}
```

### 4.4 State Transition — 200 OK with Updated Actions

```
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
  "kind": "post",
  "self": "/admin/posts/42",
  "state": {
    "id": 42,
    "title": "My Post",
    "status": "trashed",
    "trashedAt": "2026-03-17T10:30:00Z"
  },
  "actions": [
    {
      "name": "restore-post",
      "rel": "restore",
      "method": "POST",
      "href": "/admin/posts/42/restore"
    },
    {
      "name": "delete-post",
      "rel": "delete",
      "method": "DELETE",
      "href": "/admin/posts/42",
      "hints": {"confirm": "Permanently delete this post?", "destructive": true}
    }
  ]
}
```

### 4.5 Validation Failure — 422

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
        {"name": "name", "type": "text", "required": true, "value": "", "error": "Name is required"},
        {"name": "email", "type": "email", "required": true, "value": "not-an-email", "error": "Invalid email address"},
        {"name": "phone", "type": "tel", "value": "+1234567890"}
      ]
    }
  ]
}
```

### 4.6 Async Mutation — 202 Accepted

```
HTTP/1.1 202 Accepted
Content-Type: application/json
```

```json
{
  "kind": "message",
  "self": "/agents/a1/conversations/c42/messages/m7",
  "state": {
    "id": "m7",
    "role": "assistant",
    "status": "generating",
    "content": ""
  },
  "meta": {
    "poll-interval": "2s"
  }
}
```

### 4.7 Delete with Collection Response and Notification

```
HTTP/1.1 200 OK
Content-Type: application/json
```

```json
{
  "kind": "post-list",
  "state": {
    "posts": [
      {"id": 43, "title": "Another Post", "status": "trashed"}
    ],
    "total": 1
  },
  "meta": {
    "notification": {
      "type": "success",
      "message": "\"My Post\" has been permanently deleted."
    }
  },
  "actions": [
    {
      "name": "filter-posts",
      "rel": "filter",
      "method": "GET",
      "href": "/admin/posts",
      "fields": [
        {"name": "status", "type": "select", "options": [
          {"value": "draft", "label": "Draft"},
          {"value": "published", "label": "Published"},
          {"value": "trashed", "label": "Trashed"}
        ]}
      ]
    }
  ]
}
```

## 5. Alternatives Considered

### 5.1 Always Redirect After Mutations

Always use 303 See Other after every mutation, redirecting to the affected
resource (for create/update) or the parent collection (for delete).

**Pros:**
- Simple, uniform pattern. Every mutation is followed by a GET.
- Natural fit for browser-based clients (PRG pattern).
- `Navigator` follows redirects transparently.

**Cons:**
- Doubles the number of round-trips: one for the mutation, one for the GET.
- CLI clients that display the result immediately must wait for the redirect
  and second response.
- 201 Created semantics are lost — the client cannot distinguish "just
  created" from "already existed" without additional signaling.
- Async mutations (202) and streaming responses do not fit this pattern.

**Decision:** Redirects are appropriate for HTML/browser clients but not as
a universal convention. Servers SHOULD return representation bodies for API
clients and MAY use redirects for HTML clients via content negotiation.

### 5.2 Always Return 204 No Content on Delete

Return an empty body for all delete operations.

**Pros:**
- Minimal response. Clearly signals "nothing to return."
- Common convention in REST APIs.

**Cons:**
- `Navigator.SubmitAction` sets the current position to the response. An
  empty representation leaves the navigator stranded with no links or actions.
- CLI clients cannot display a confirmation message or offer navigation.
- Inconsistent with the hypermedia principle that responses should be
  self-describing and navigable.

**Decision:** Return a `kind: "deleted"` representation with navigation links.
Servers MAY use 204 only when the client is known to not need a response body
(e.g., fire-and-forget background operations).

### 5.3 Encode Mutation Type in Representation.Hints

Add a `Hints["mutation"]` field to signal what kind of mutation produced this
response (e.g., `"created"`, `"updated"`, `"deleted"`).

**Pros:**
- Clients can handle responses generically based on mutation type.
- Works even when `Kind` is a domain-specific label.

**Cons:**
- Duplicates information already available from the HTTP status code (201 =
  created, 200 = updated, etc.) and `Kind` (e.g., `"deleted"`).
- Adds complexity without clear benefit — no use case needed this.

**Decision:** Rely on HTTP status codes and `Kind` to signal mutation outcomes.
Do not add `Hints["mutation"]`.

## 6. Open Questions

1. **Should `Representation.Hints` signal the mutation type?** The HTTP status
   code (201, 200, 204) already distinguishes mutation types. Adding a
   `Hints["mutation"]` field would be redundant but might help clients that
   process representations outside of the HTTP context (e.g., SSE events,
   message queues). Is this worth standardizing?

2. **How do htmx partial responses interact with mutation conventions?** When
   using `hx-target` and `hx-swap`, htmx expects HTML fragments rather than
   full page representations. Should mutation responses support partial
   representations for htmx, or should htmx clients always use full
   representations with client-side extraction?

3. **Should delete responses carry `Self`?** A `kind: "deleted"` representation
   refers to a resource that no longer exists. Setting `Self` to the deleted
   resource's URL could be misleading (a GET to that URL would return 404).
   Should `Self` be omitted, or should it point to the parent collection?

4. **Should `Meta["notification"]` be standardized?** The blog-platform use
   case introduced `Meta["notification"]` with `type` and `message` fields
   for toast-style messages. Should this be a standard convention for all
   mutation responses, or should it remain application-specific?

5. **How should batch mutations respond?** If a client submits a batch
   operation that creates, updates, or deletes multiple resources, what should
   the response look like? A collection representation? An array of individual
   results? This proposal does not address batch mutations.

6. **Should 201 responses always set the `Location` header?** HTTP semantics
   (RFC 9110 §15.3.2) say 201 responses SHOULD include a `Location` header
   pointing to the created resource. The `Self` link serves the same purpose
   within the representation. Should hyper's `Renderer` automatically set
   `Location` from `Self` on 201 responses?
