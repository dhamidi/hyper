# hyper

`hyper` is a Go library for building hypermedia-driven APIs. The module path is `github.com/dhamidi/hyper`.

## Driver programs

Driver programs are small `package main` programs that exercise the library's public API to verify it works as documented.

- They live under `internal/` (e.g., `internal/tryme/main.go`) so they are not installable by external consumers.
- They are **not** test files — they are standalone programs that start an `httptest.Server`, build representations, and print results to stdout.
- To run a driver program: `go run ./internal/tryme/`
- To add a new driver program: create a new directory under `internal/` (e.g., `internal/mydriver/main.go`) with `package main` and a `func main()`.

## Testing

Run the full test suite:

```
go test ./...
```

## Use-case documents

See `use-cases/AGENTS.md` for conventions on writing use-case exploration documents.

## Subdirectories

- `htmlc/` — HTML component library
- `jsonapi/` — JSON:API codec
- `docs/` — Documentation (explanation, how-to, reference, tutorials)
- `use-cases/` — Use-case exploration documents
- `internal/` — Internal driver programs (not for external use)
