# hyper — Spec Readiness Report

**Date:** 2026-03-14
**Spec:** `spec.md` (Draft)
**Module:** `github.com/dhamidi/hyper`

## Summary

10 deficiencies identified across the implementation and specification.
The library implements the core model (§6), hypermedia controls (§7),
target resolution (§8), the JSON codec (§14), the JSON:API codec, the
Renderer (§10), and the Client (§11) including Navigator, streaming,
and credential support. The gaps below range from correctness bugs to
SHOULD-level compliance items.

---

## Deficiencies

### Correctness Bugs (fix first)

#### D8 — JSON wire format decoder does not decode Field.Options (§14.3.4)

**Status: Resolved**

**File:** `client.go`, `decodeField` function

**Spec requirement (§14.3.4):** Fields SHALL be encoded as JSON objects
including an `"options"` array of `{"value", "label", "selected"}` objects.
Round-tripping requires the decoder to reconstruct `Field.Options` from this
wire format.

**Resolution:** Added an `"options"` clause to `decodeField` that iterates
the JSON array and constructs `[]Option` values, mirroring the encode path
in `encodeFields`. Field options are now correctly round-tripped through
encode/decode.

---

#### D3 — Client.resolveTarget ignores Target.Query (§8.1) — Resolved

**File:** `client.go`, `resolveTarget` method

**Spec requirement (§8.1):** `Query` MAY contain query parameters to be
appended to the resolved URL. Resolvers SHALL append `Query` to the URL
when non-nil.

**Resolution:** `resolveTarget` now merges `Target.Query` and
`Target.Route.Query` into the resolved URL's query string before returning.
Pagination links constructed with
`hyper.Path("contacts").WithQuery(url.Values{"page": {"3"}})` now correctly
resolve to `/contacts?page=3`.

---

### Architectural Gaps

#### D4 — Client.Submit hardcodes JSON encoding, bypassing registered SubmissionCodecs (§11.4.2)

**Status: Resolved**

**File:** `client.go`, `Submit` method; `hyper.go`, `SubmissionCodec` interface

**Spec requirement (§11.4.2):** Submit SHALL select a `SubmissionCodec`
from `c.SubmissionCodecs` based on `action.Consumes` and encode `values`
using the selected codec.

**Resolution:** Added an `Encode(values map[string]any) (io.Reader, error)`
method to the `SubmissionCodec` interface. `JSONSubmissionCodec` and
`jsonapi.SubmissionCodec` both implement the new method. `Client.Submit`
now calls `selectSubmissionCodec` to find the matching codec by media type
and uses its `Encode` method instead of hardcoding `json.NewEncoder`.
Custom submission codecs are now fully functional on the client side.

---

#### D6 — No FormSubmissionCodec for application/x-www-form-urlencoded (§11.8)

**Status: Resolved**

**File:** `form_codec.go`, `client.go` `NewClient` function

**Spec requirement (§11.8):** `NewClient` defaults SHALL include
`SubmissionCodecs: [JSONSubmissionCodec, FormSubmissionCodec]`.

**Resolution:** Implemented `FormSubmissionCodec` in `form_codec.go` that
decodes `application/x-www-form-urlencoded` bodies into `*map[string]any`
targets using `url.ParseQuery`, and encodes `map[string]any` values as
form-urlencoded bodies using `url.Values`. Registered as a default in
`NewClient` alongside `JSONSubmissionCodec`.

---

### Missing Codecs (SHOULD-level compliance)

#### D1 — No HTML RepresentationCodec (§12)

**Spec requirement (§12):** HTML SHALL be treated as a first-class target
format. An HTML codec SHOULD render `Link` as `<a>`, actions with fields
as `<form>`, etc.

**Current behavior:** No HTML codec exists. The `Renderer` can only serve
JSON (native or JSON:API) and `text/event-stream`.

**Impact:** Applications that need server-rendered HTML must build their
own codec or use `html/template` outside the `Renderer` pipeline, losing
content negotiation benefits.

**Remediation:** Implement an HTML `RepresentationCodec` (likely backed by
`html/template`) that renders representations as semantic HTML. This is a
SHOULD-level requirement; applications can work around it, but it is a
stated design goal (§3.4).

---

#### D2 — No Markdown RepresentationCodec (§13)

**Spec requirement (§13):** Markdown SHOULD be treated as a read-oriented
alternate representation.

**Current behavior:** No Markdown codec exists.

**Impact:** The `RespondAs(w, r, 200, "text/markdown", rep)` pattern shown
in §15.4 will fail with no matching codec.

**Remediation:** Implement a Markdown `RepresentationCodec` that renders
state as Markdown prose, links as `[text](url)`, and actions as descriptive
blocks. This is a SHOULD-level requirement.

---

### Minor Alignment

#### D5 — Default Accept header is "application/json" vs spec's "application/vnd.api+json"

**Status: Resolved**

**File:** `client.go`, `NewClient` function (line ~116)

**Spec requirement (§11.8, §11.4.1):** The default `Accept` header SHALL
be `"application/vnd.api+json"`.

**Resolution:** Changed the default `Accept` value in `NewClient` from
`"application/json"` to `"application/vnd.api+json"`. Updated tests to
match the new default.

---

#### D7 — Route-only targets silently produce empty href when no Resolver configured

**Status: Resolved**

**File:** `json_codec.go`, `resolveTarget` function (line ~96)

**Spec requirement (§8.2.1):** A resolver SHALL fail when neither URL nor
Route form is resolvable.

**Resolution:** `resolveTarget` now returns an error when a Route-only target
is encountered and no `Resolver` is configured, instead of silently returning
an empty string. This ensures malformed hrefs are caught at encode time.

---

#### D10 — Spec §7.3 Field definition doesn't include Accept/MaxSize/Multiple (implementation is ahead)

**File:** `hyper.go`, `Field` struct (line ~80) vs `spec.md` §7.3

**Spec definition (§7.3):**
```go
type Field struct {
    Name, Type, Label, Help, Error string
    Value    any
    Required bool
    ReadOnly bool
    Options  []Option
}
```

**Implementation:**
```go
type Field struct {
    // ...all spec fields, plus:
    Accept   string  // Accepted MIME types (file fields)
    MaxSize  int64   // Maximum file size in bytes
    Multiple bool    // Whether field accepts multiple files
}
```

**Impact:** The implementation is ahead of the spec. The extra fields are
useful for file-upload fields but are not documented in the specification.
The JSON encoder and decoder both handle these fields correctly.

**Remediation:** Update §7.3 in `spec.md` to document `Accept`, `MaxSize`,
and `Multiple` fields, or remove them from the implementation if they are
not yet approved.

---

#### D9 — Spec §18.3 example has nil-map append bug

**File:** `spec.md`, §18.3 example (line ~2918)

**Code in spec:**
```go
rep := Representation{}
// ...
if ep, ok := v.(EmbeddedProvider); ok {
    for slot, reps := range ep.HyperEmbedded() {
        rep.Embedded[slot] = append(rep.Embedded[slot], reps...)
    }
}
```

**Bug:** `rep.Embedded` is `nil` (zero value of `map[string][]Representation`).
The `append` call reads from a nil map (safe), but the assignment
`rep.Embedded[slot] = ...` panics at runtime because you cannot assign to
a nil map.

**Impact:** Spec example is incorrect. Anyone implementing the
`buildRepresentation` helper from §18.3 will hit a runtime panic.

**Remediation:** Add `rep.Embedded = make(map[string][]Representation)`
before the loop, or initialize it lazily inside the `if` block.

---

## Compliance Matrix

| ID  | Section | Severity       | Status         | Summary                                           |
|-----|---------|----------------|----------------|---------------------------------------------------|
| D8  | §14.3.4 | Bug            | Resolved        | Decoder now decodes Field.Options                 |
| D3  | §8.1    | Bug            | Resolved        | Client now merges Target.Query into resolved URL  |
| D4  | §11.4.2 | Architectural  | Resolved        | Submit now uses SubmissionCodec.Encode              |
| D6  | §11.8   | Architectural  | Not implemented | No FormSubmissionCodec                            |
| D1  | §12     | SHOULD         | Not implemented | No HTML codec                                     |
| D2  | §13     | SHOULD         | Not implemented | No Markdown codec                                 |
| D5  | §11.8   | Minor          | Resolved        | Default Accept header now application/vnd.api+json |
| D7  | §8.2.1  | Minor          | Resolved        | Error on unresolved Route targets without Resolver |
| D10 | §7.3    | Minor (spec)   | Impl ahead      | Field has undocumented Accept/MaxSize/Multiple     |
| D9  | §18.3   | Minor (spec)   | Spec bug        | Nil-map panic in example code                     |

## Recommended Priority

1. **D8, D3** — Correctness bugs. Fix these first to ensure round-trip
   fidelity and correct client behavior.
2. **D4, D6** — Architectural gaps. These block the codec abstraction from
   working end-to-end for non-JSON content types.
3. **D5, D7** — Minor alignment issues. Quick fixes that improve spec
   conformance.
4. **D9, D10** — Spec updates. These require editing `spec.md` rather than
   Go code.
5. **D1, D2** — SHOULD-level codecs. Significant implementation effort but
   not blocking for JSON-only deployments.
