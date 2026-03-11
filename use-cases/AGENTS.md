# Use-Case Explorations for `hyper`

This directory contains use-case documents that explore applications of the `hyper` library conceptually. The library does not exist yet — these documents exercise the spec's design by playing through realistic scenarios end-to-end.

## Purpose

Use-case explorations stress-test the spec before implementation begins. Each document should reveal whether the core types (`Representation`, `Link`, `Action`, `Field`, `Target`, `Resolver`, codecs, etc.) are sufficient for a real scenario, and surface any gaps, ambiguities, or needed changes.

## How to Write a Use-Case Document

### 1. Pick a Concrete Scenario

Choose a realistic application scenario (e.g. "multi-step checkout flow", "admin dashboard with inline editing", "CLI client navigating an API"). Define the actors, resources, and interactions involved.

### 2. Play Through the Scenario End-to-End

Walk through every meaningful interaction step. For each step, identify:

- Which `hyper` types are exercised: `Representation`, `Node` (`Object`, `Collection`), `Value` (`Scalar`, `RichText`), `Link`, `Action`, `Field`, `Option`, `Target`, `RouteRef`, `Embedded`, `Meta`, `Hints`
- How hypermedia controls drive the interaction (links for navigation, actions for state transitions)
- How embedded representations are used for fragments, sub-resources, or inline editors
- Which codecs are involved (`RepresentationCodec`, `SubmissionCodec`) and what media types they handle
- How `Target` and `Resolver` handle URL resolution

### 3. Show Concrete Go Code Snippets

For each key interaction, show Go code that builds the `hyper.Representation` value. Use the spec's type definitions directly:

```go
rep := hyper.Representation{
    Kind: "order",
    Self: hyper.Path("orders", "99").Ptr(),
    State: hyper.Object{
        "status": hyper.Scalar{V: "pending"},
        "total":  hyper.Scalar{V: 49.99},
    },
    Links: []hyper.Link{
        {Rel: "customer", Target: hyper.Path("customers", "7"), Title: "Customer"},
    },
    Actions: []hyper.Action{
        {
            Name:   "Confirm",
            Rel:    "confirm",
            Method: "POST",
            Target: hyper.Path("orders", "99", "confirm"),
        },
    },
}
```

### 4. Show the JSON Wire Format

For key representations, include the expected JSON output so readers can see how the scenario looks on the wire:

```json
{
  "kind": "order",
  "self": {"href": "/orders/99"},
  "state": {
    "status": "pending",
    "total": 49.99
  },
  "links": [
    {"rel": "customer", "href": "/customers/7", "title": "Customer"}
  ],
  "actions": [
    {"name": "Confirm", "rel": "confirm", "method": "POST", "href": "/orders/99/confirm"}
  ]
}
```

### 5. Identify Gaps and Ambiguities

As you work through the scenario, note anything that feels awkward, unclear, or unsupported:

- Types that are missing or insufficient for the scenario
- Interactions that require workarounds or conventions not covered by the spec
- Ambiguities in how existing types should be used
- Cases where `Hints` or `Meta` are overloaded to fill a gap that might deserve first-class support
- Edge cases around `Embedded`, `RenderMode` (document vs. fragment), or codec behavior

### 6. End with a "Spec Feedback" Section

Every use-case document MUST end with a **Spec Feedback** section that lists:

- Potential spec updates or additions discovered during the exercise
- Clarifications needed for existing spec language
- New types, fields, or interfaces that might be warranted
- Open questions that the scenario raises but does not resolve

Format each item as a concise, actionable bullet point.

## Conventions

- Use the spec's terminology and type names exactly (e.g. `Representation`, not "response object")
- Reference spec section numbers when relevant (e.g. "per §6.1, `Embedded` maps slot names to representation arrays")
- Keep documents focused — one scenario per file
- Name files descriptively: `checkout-flow.md`, `cli-api-navigation.md`, `inline-editing-htmx.md`
- Write in present tense, as if walking a reader through the scenario
- Include both the "happy path" and at least one error/edge case
