# The Codec Ecosystem

Hyper separates the representation model from its wire format through codecs.
This design allows the same API handlers to serve multiple formats.

## Built-in codecs

The `hyper` package provides several built-in codecs:

```go
import "github.com/dhamidi/hyper"

jsonCodec := hyper.JSONCodec()            // encodes Representation as JSON
htmlCodec := hyper.HTMLCodec()            // encodes Representation as semantic HTML
mdCodec   := hyper.MarkdownCodec()       // encodes Representation as Markdown
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

## Markdown support

The Markdown codec renders representations as read-oriented Markdown prose
(§13.1). Since Markdown has no native form controls, actions are degraded to
descriptive prose and UI-specific hints are omitted.

- **Kind** becomes a top-level `# heading`
- **Object state** renders as a bulleted list with bold keys (e.g., `- **name:** Ada`)
- **Collection state** renders as a numbered list
- **Links** become `[text](url) (rel: rel)` Markdown links under a `## Links` heading
- **Actions** render as `### Name (METHOD /path)` headings with fields listed as
  `- name (type, required): "value"`
- **Embedded representations** render as subsections with `##` slot headings and
  `###` kind headings
- **RichText** with `text/markdown` media type passes through directly; other
  media types render the source in a fenced code block with the media type as
  language hint
- **Meta** renders as a `## Meta` section with key-value items
- **Hints** are omitted (UI-specific, not relevant to Markdown)

The codec supports both `RenderDocument` (includes the `Kind` heading) and
`RenderFragment` (omits the heading, renders state/links/actions only) modes.

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
