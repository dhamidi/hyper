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

**File:** `client.go`, `decodeField` function (line ~644)

**Spec requirement (§14.3.4):** Fields SHALL be encoded as JSON objects
including an `"options"` array of `{"value", "label", "selected"}` objects.
Round-tripping requires the decoder to reconstruct `Field.Options` from this
wire format.

**Current behavior:** `decodeField` decodes `name`, `type`, `value`,
`required`, `readOnly`, `label`, `help`, `error`, `accept`, `maxSize`, and
`multiple` — but never reads the `"options"` key. Any `Field.Options` present
in the JSON response are silently dropped.

**Impact:** Clients that fetch representations with select/checkbox-group
fields will see empty `Options` slices, breaking UI rendering and form
pre-population.

**Remediation:** Add an `"options"` clause to `decodeField` that iterates
the JSON array and constructs `[]Option` values (mirroring the encode path
in `encodeFields`).

---

#### D3 — Client.resolveTarget ignores Target.Query (§8.1)

**File:** `client.go`, `resolveTarget` method (line ~346)

**Spec requirement (§8.1):** `Query` MAY contain query parameters to be
appended to the resolved URL. Resolvers SHALL append `Query` to the URL
when non-nil.

**Current behavior:** `Client.resolveTarget` checks only `Target.URL`,
resolves it against `BaseURL`, and returns. It never inspects `Target.Query`
(or `Target.Route.Query`). Query parameters set via `WithQuery()` are lost
in all client operations (`Fetch`, `Submit`, `Follow`, `FetchStream`).

**Impact:** Pagination links constructed with
`hyper.Path("contacts").WithQuery(url.Values{"page": {"3"}})` will fetch
`/contacts` instead of `/contacts?page=3`.

**Remediation:** After resolving the base URL, merge `Target.Query` (and
`Target.Route.Query` if applicable) into the resolved URL's query string
before returning.

---

### Architectural Gaps

#### D4 — Client.Submit hardcodes JSON encoding, bypassing registered SubmissionCodecs (§11.4.2)

**File:** `client.go`, `Submit` method (line ~200)

**Spec requirement (§11.4.2):** Submit SHALL select a `SubmissionCodec`
from `c.SubmissionCodecs` based on `action.Consumes` and encode `values`
using the selected codec.

**Current behavior:** `Submit` calls `selectSubmissionMediaType` to choose
the Content-Type string, but then unconditionally uses `json.NewEncoder`
to serialize the body. The registered `SubmissionCodec.Decode` method is
never called (and the codec interface is for decoding, not encoding — there
is no symmetric `Encode` on `SubmissionCodec`). This means:

- Non-JSON submission types (e.g., `application/x-www-form-urlencoded`)
  will receive a JSON body with the wrong Content-Type header.
- Custom SubmissionCodecs are effectively dead code on the client side.

**Impact:** Actions that declare `Consumes: ["application/x-www-form-urlencoded"]`
will send malformed requests.

**Remediation:** Either (a) add an `Encode` method to the `SubmissionCodec`
interface for client-side encoding, or (b) introduce a separate
`SubmissionEncoder` interface that `Client.Submit` can use to encode values
in the format matching `action.Consumes`.

---

#### D6 — No FormSubmissionCodec for application/x-www-form-urlencoded (§11.8)

**File:** `client.go`, `NewClient` function (line ~111)

**Spec requirement (§11.8):** `NewClient` defaults SHALL include
`SubmissionCodecs: [JSONSubmissionCodec, FormSubmissionCodec]`.

**Current behavior:** `NewClient` registers only `JSONSubmissionCodec()`.
No `FormSubmissionCodec` type exists anywhere in the codebase. The spec's
`§15.3` example shows a `FormCodec.Decode` call for form-encoded bodies,
implying a form codec is expected.

**Impact:** Server handlers cannot decode `application/x-www-form-urlencoded`
submissions using the codec abstraction; they must fall back to manual
`r.ParseForm()` calls, undermining the codec architecture.

**Remediation:** Implement a `FormSubmissionCodec` that wraps
`net/http.Request.ParseForm` and populates `*map[string]any` targets.
Register it as a default in `NewClient`.

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

**File:** `client.go`, `NewClient` function (line ~116)

**Spec requirement (§11.8, §11.4.1):** The default `Accept` header SHALL
be `"application/vnd.api+json"`.

**Current behavior:** `NewClient` sets `Accept: "application/json"`.

**Impact:** Servers that content-negotiate based on `Accept` will serve
native JSON instead of JSON:API format. This is a minor mismatch that
affects interoperability with JSON:API servers.

**Remediation:** Change the default `Accept` value in `NewClient` to
`"application/vnd.api+json"`.

---

#### D7 — Route-only targets silently produce empty href when no Resolver configured

**File:** `json_codec.go`, `resolveTarget` function (line ~95)

**Spec requirement (§8.2.1):** A resolver SHALL fail when neither URL nor
Route form is resolvable.

**Current behavior:** When `Resolver` is nil and `Target.URL` is nil (i.e.,
the target only has a `Route`), `resolveTarget` returns `""` with no error.
This silently produces empty `"href": ""` in the JSON output.

**Impact:** Representations that use `Route`-based targets without
configuring a `Resolver` on the `Renderer` will emit malformed JSON with
empty hrefs. Clients will not be able to follow these links.

**Remediation:** Return an error when a `Route`-only target is encountered
and no `Resolver` is configured, rather than silently emitting an empty
string.

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
| D8  | §14.3.4 | Bug            | Not implemented | Decoder drops Field.Options                       |
| D3  | §8.1    | Bug            | Not implemented | Client ignores Target.Query                       |
| D4  | §11.4.2 | Architectural  | Not implemented | Submit hardcodes JSON, ignores SubmissionCodecs    |
| D6  | §11.8   | Architectural  | Not implemented | No FormSubmissionCodec                            |
| D1  | §12     | SHOULD         | Not implemented | No HTML codec                                     |
| D2  | §13     | SHOULD         | Not implemented | No Markdown codec                                 |
| D5  | §11.8   | Minor          | Misaligned      | Default Accept header mismatch                    |
| D7  | §8.2.1  | Minor          | Silent failure  | Empty href for unresolved Route targets            |
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
