# Task List Example

A task list web application demonstrating the `hyper` library with a custom `RepresentationCodec` backed by `htmlc` Vue SFC templates. Both HTML and JSON responses are driven through `hyper.Renderer` with content negotiation — handlers build a single `hyper.Representation` that serves both formats.

## Running

```bash
cd examples/tasklist
go run .
```

The server starts on [http://localhost:8082](http://localhost:8082).

## How it works

The app registers an `htmlcCodec` as a `hyper.RepresentationCodec` alongside `hyper.JSONCodec()` in the renderer. When a request arrives, `hyper.Renderer` negotiates the response format based on the `Accept` header and encodes the representation using the matching codec.

A `representationToScope` bridge function converts `hyper.Representation` values into flat `map[string]any` scopes for the htmlc templates:

- **State fields** are promoted to top-level scope keys (e.g., `title`, `status`)
- **Actions** are keyed by name (e.g., `actions.create`, `actions.toggle`) with resolved `href`, `fields`, and `hxAttrs`
- **Embedded representations** are recursively converted (e.g., `embedded.items`)
- **Links** are keyed by rel (e.g., `links.self`)

### Vue SFC templates

HTML views are defined as Vue SFC templates in the `components/` directory:

- **`page-layout.vue`** — Layout shell for page-wide concerns (style/script includes)
- **`task-list-content.vue`** — Swappable content region (`#task-list-content`)
- **`task-list.vue`** — Router component choosing document vs fragment composition
- **`task-row.vue`** and **`task-actions.vue`** — Semantic task row and action controls
- **`task-form.vue`** — Task creation form with validation error display

### Content negotiation

Request HTML (for browsers):

```bash
curl -s http://localhost:8082/ -H 'Accept: text/html'
```

Request JSON (for API clients):

```bash
curl -s http://localhost:8082/ -H 'Accept: application/json' | jq .
```

### htmx fragment responses

When the `HX-Request: true` header is present, the codec renders fragments (no DOCTYPE wrapper) suitable for htmx swaps:

```bash
curl -s http://localhost:8082/ -H 'Accept: text/html' -H 'HX-Request: true'
```

### Creating tasks

```bash
curl -s -X POST http://localhost:8082/tasks \
  -d 'title=Buy+milk' \
  -H 'Accept: application/json' | jq .
```

New tasks default to `pending`; create does not expose a status field.

Validation rule: title must be longer than 3 characters.

### Toggling task status

```bash
curl -s -X POST http://localhost:8082/tasks/1/toggle \
  -H 'Accept: application/json' | jq .
```

### Deleting tasks

```bash
curl -i -X DELETE http://localhost:8082/tasks/1 \
  -H 'Accept: application/json'
```

For htmx requests (`HX-Request: true`), delete returns an updated `task-list` fragment and swaps `#task-list-content` so list-level state stays consistent.

## Interaction patterns

The UI uses a small set of consistent HTMX interaction contracts:

1. Document load:
   - `GET /` returns full document composition through `page-layout`.
   - Layout owns style/script tags and `htmx-config`.

2. Create (HTMX, no refresh):
   - Create form uses `hx-post="/tasks"` and `hx-swap="none"`.
   - Server returns a fragment with `hx-swap-oob="outerHTML:#task-list-content"` on the content root.
   - Browser stays on the same URL; task list and count update via OOB swap.

3. Create validation (422):
   - If title length is `<= 3`, server returns `422`.
   - Response includes OOB form replacement (`hx-swap-oob="outerHTML:#task-create-form"`) with an inline error line under the title input.
   - `htmx-config` allows swapping on `422`.

4. Toggle:
   - Row action posts to `/tasks/{id}/toggle`.
   - Response swaps only the affected row (`hx-target="closest li"`, `hx-swap="outerHTML"`).

5. Delete:
   - Row action submits delete to `/tasks/{id}`.
   - Response swaps the whole content region (`#task-list-content`) to keep aggregate state (`taskCount`, empty-state message) correct.

## Architecture

- **In-memory store**: Tasks are stored in a `sync.Mutex`-protected slice. Three sample tasks are seeded on startup.
- **Hypermedia representations**: Each task and the task list are modeled as `hyper.Representation` values with links, actions, and embedded items — used by both HTML and JSON codecs.
- **htmlcCodec**: A custom `RepresentationCodec` that maps `Representation.Kind` to a Vue SFC component name and uses `representationToScope` to bridge the representation into template data.
- **representationToScope**: Converts structured `hyper.Representation` fields into flat `map[string]any` scopes with resolved URLs, flattened state, and pre-filtered htmx attributes.
- **RenderMode**: The `HX-Request` header drives fragment vs. document rendering — htmx requests get bare component markup, full page loads get a complete HTML document.
- **htmx integration**: Actions include htmx hint attributes (`hx-post`, `hx-delete`, `hx-target`, `hx-swap`) rendered via `v-bind` in templates. Create uses OOB content swaps and form OOB rerender on validation errors; delete swaps `#task-list-content`.
- **Compact task rows**: Each task is rendered as a dense single row with title/status on the left and action buttons on the right.
- **Method override**: DELETE actions are rendered as POST forms with a hidden `_method=DELETE` field. The `methodoverride.Wrap` middleware translates these back to DELETE requests.
- **Typewriter styling**: A custom `style.css` provides a monospace, cream-background aesthetic inspired by typewritten pages.

## Testing

```bash
go test -v ./...
```
