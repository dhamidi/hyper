# The Codec Ecosystem

Hyper separates the representation model from its wire format through codecs.
This design allows the same API handlers to serve multiple formats.

## Built-in codecs

The `hyper` package provides several built-in codecs:

```go
import "github.com/dhamidi/hyper"

jsonCodec := hyper.JSONCodec()            // encodes Representation as JSON
htmlCodec := hyper.HTMLCodec()            // encodes Representation as semantic HTML
jsonSub   := hyper.JSONSubmissionCodec()  // decodes and encodes JSON request bodies
formSub   := hyper.FormSubmissionCodec()  // decodes and encodes form-urlencoded request bodies
```

## HTML support

The HTML codec renders representations as semantic HTML:

- **Links** become `<a>` tags inside a `<nav>` element
- **Actions** become `<form>` tags with `<input>`, `<select>`, and `<textarea>` fields
- **Object state** renders as a `<dl>` (definition list)
- **Collection state** renders as an `<ol>` (ordered list)
- **Embedded representations** render as nested `<article>` elements inside `<section>`

The codec supports both `RenderDocument` (full HTML page with `<!DOCTYPE>`)
and `RenderFragment` (just the `<article>` element) modes. All output is
HTML-escaped to prevent XSS.

## JSON:API support

The `jsonapi` subpackage provides codecs for the JSON:API media type
(`application/vnd.api+json`):

```go
import "github.com/dhamidi/hyper/jsonapi"

repCodec, subCodec := jsonapi.DefaultCodecs()
```

These codecs translate between hyper's `Representation` model and the
JSON:API document structure, including resource objects, relationships,
and error documents.

## How codecs are selected

On the **server side**, the `Renderer` performs content negotiation using the
request's `Accept` header to pick a codec whose `MediaTypes()` match.

On the **client side**, the `Client` sends an `Accept` header and decodes
the response using the first codec that matches the response `Content-Type`.
For submissions, `Client.Submit` selects a `SubmissionCodec` whose
`MediaTypes()` match `action.Consumes` and calls its `Encode` method to
serialize the request body in the correct format.

## Writing your own codec

See [How to Add a Codec](../how-to/add-codec.md) for a step-by-step guide.
