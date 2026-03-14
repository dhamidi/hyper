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

- **`task-list.vue`** — Full page layout with task list and create form (maps to `Kind: "task-list"`)
- **`task.vue`** — Single-row task with inline toggle and delete actions (maps to `Kind: "task"`)
- **`task-form.vue`** — Task creation form with field-driven inputs and validation error display

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

For htmx requests (`HX-Request: true`), delete returns an updated `task-list` fragment and swaps `#task-list-root` so list-level state stays consistent.

## Architecture

- **In-memory store**: Tasks are stored in a `sync.Mutex`-protected slice. Three sample tasks are seeded on startup.
- **Hypermedia representations**: Each task and the task list are modeled as `hyper.Representation` values with links, actions, and embedded items — used by both HTML and JSON codecs.
- **htmlcCodec**: A custom `RepresentationCodec` that maps `Representation.Kind` to a Vue SFC component name and uses `representationToScope` to bridge the representation into template data.
- **representationToScope**: Converts structured `hyper.Representation` fields into flat `map[string]any` scopes with resolved URLs, flattened state, and pre-filtered htmx attributes.
- **RenderMode**: The `HX-Request` header drives fragment vs. document rendering — htmx requests get bare component markup, full page loads get a complete HTML document.
- **htmx integration**: Toggle and delete actions include htmx hint attributes (`hx-post`, `hx-delete`, `hx-target`, `hx-swap`) rendered via `v-bind` in templates. Delete swaps `#task-list-root` with a refreshed list fragment.
- **Compact task rows**: Each task is rendered as a dense single row with title/status on the left and action buttons on the right.
- **Method override**: DELETE actions are rendered as POST forms with a hidden `_method=DELETE` field. The `methodoverride.Wrap` middleware translates these back to DELETE requests.
- **Typewriter styling**: A custom `style.css` provides a monospace, cream-background aesthetic inspired by typewritten pages.

## Testing

```bash
go test -v ./...
```
