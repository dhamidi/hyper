# API Reference

This document covers the primary public API of the `hyper` package.

## Core Types

Import the package:

```go
import "github.com/dhamidi/hyper"
```

### Representation

The central transferable value in hyper:

```go
rep := hyper.Representation{
    Kind:  "contact",
    Self:  hyper.Path("contacts", "1").Ptr(),
    State: hyper.StateFrom("name", "Ada", "email", "ada@example.com"),
    Links: []hyper.Link{
        hyper.NewLink("self", hyper.Path("contacts", "1")),
    },
    Actions: []hyper.Action{
        hyper.NewAction("Update", "PUT", hyper.Path("contacts", "1")),
    },
}
```

### Target Constructors

Targets designate where a link or action points:

```go
hyper.Path("contacts", "42")          // /contacts/42
hyper.Pathf("/contacts/%d", 99)       // /contacts/99
hyper.ParseTarget("/api/v2")          // parsed URL
hyper.MustParseTarget("https://example.com/api")
hyper.Route("contacts.show", "id", "42")  // named route
```

### Client

Create a client with `NewClient`:

```go
import "github.com/dhamidi/hyper"

client, err := hyper.NewClient("http://localhost:8080",
    hyper.WithAccept("application/json"),
)
```

Available options: `WithTransport`, `WithCredentials`, `WithStaticCredential`,
`WithCodec`, `WithSubmissionCodec`, `WithAccept`.

### Navigator

Stateful browsing session:

```go
nav, err := client.Navigate(ctx, hyper.Path("contacts"))
nav.Kind()                         // current representation kind
nav.Links()                        // available links
nav.Actions()                      // available actions
nav.Follow(ctx, "self")            // follow link by rel
nav.Submit(ctx, "create", values)  // submit action by rel
nav.Back()                         // go back in history
nav.Refresh(ctx)                   // re-fetch current
```

### Convenience Helpers

```go
hyper.NewLink("next", hyper.Path("contacts", "page", "2"))
hyper.NewAction("Delete", "DELETE", hyper.Path("contacts", "1"))
hyper.NewField("username", "text")
hyper.Markdown("**bold**")
hyper.PlainText("hello")
hyper.StateFrom("key", "value")
hyper.WithValues(fields, values)
hyper.WithErrors(fields, values, errors)
```

### Finding Utilities

```go
link, ok := hyper.FindLink(rep, "self")
action, ok := hyper.FindAction(rep, "create")
embedded := hyper.FindEmbedded(rep, "contacts")
defaults := hyper.ActionValues(action)
```

### Codecs

```go
hyper.JSONCodec()            // RepresentationCodec for application/json
hyper.HTMLCodec()            // RepresentationCodec for text/html
hyper.JSONSubmissionCodec()  // SubmissionCodec for application/json
hyper.FormSubmissionCodec()  // SubmissionCodec for application/x-www-form-urlencoded
```

`SubmissionCodec` supports both directions: `Decode` for server-side request
body parsing, and `Encode` for client-side request body serialization.
`Client.Submit` selects a codec based on `action.Consumes` and calls `Encode`
to serialize values in the matching format.

For JSON:API format, see the `jsonapi` subpackage.
