# Task List Example

A task list web application demonstrating the `hyper` library's built-in HTML codec with a typewriter-inspired visual design.

## Running

```bash
cd examples/tasklist
go run .
```

The server starts on [http://localhost:8080](http://localhost:8080).

## How it works

The app uses `hyper.HTMLCodec()` for HTML rendering and `hyper.JSONCodec()` for JSON API responses. The same endpoints serve both formats via content negotiation based on the `Accept` header.

### Content negotiation

Request HTML (for browsers):

```bash
curl -s http://localhost:8080/ -H 'Accept: text/html'
```

Request JSON (for API clients):

```bash
curl -s http://localhost:8080/ -H 'Accept: application/json' | jq .
```

### Creating tasks

```bash
curl -s -X POST http://localhost:8080/tasks \
  -d 'title=Buy+milk&status=pending' \
  -H 'Accept: application/json' | jq .
```

### Toggling task status

```bash
curl -s -X POST http://localhost:8080/tasks/1/toggle \
  -H 'Accept: application/json' | jq .
```

### Deleting tasks

```bash
curl -s -X DELETE http://localhost:8080/tasks/1 \
  -H 'Accept: application/json' | jq .
```

## Architecture

- **In-memory store**: Tasks are stored in a `sync.Mutex`-protected slice. Three sample tasks are seeded on startup.
- **Hypermedia representations**: Each task and the task list are modeled as `hyper.Representation` values with links, actions, and embedded items.
- **htmx integration**: Toggle and delete actions include htmx hint attributes (`hx-post`, `hx-delete`, `hx-target`, `hx-swap`) so the HTML interface updates without full page reloads.
- **Method override**: DELETE actions are rendered as POST forms with a hidden `_method=DELETE` field. The `methodoverride.Wrap` middleware translates these back to DELETE requests.
- **Typewriter styling**: A custom `style.css` provides a monospace, cream-background aesthetic inspired by typewritten pages.

## Testing

```bash
go test -v ./...
```
