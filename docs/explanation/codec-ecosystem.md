# The Codec Ecosystem

Hyper separates the representation model from its wire format through codecs.
This design allows the same API handlers to serve multiple formats.

## Built-in codecs

The `hyper` package provides a native JSON codec:

```go
import "github.com/dhamidi/hyper"

repCodec  := hyper.JSONCodec()            // encodes Representation as JSON
jsonSub   := hyper.JSONSubmissionCodec() // decodes and encodes JSON request bodies
formSub   := hyper.FormSubmissionCodec() // decodes and encodes form-urlencoded request bodies
```

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
