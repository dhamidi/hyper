// Package jsonapi provides codecs that map between hyper.Representation and the
// JSON:API (https://jsonapi.org/) wire format.
//
// This package implements [hyper.RepresentationCodec] and [hyper.SubmissionCodec]
// to allow hyper-based applications to interoperate with JSON:API-speaking clients
// such as Ember Data and JSONAPI::Resources.
//
// # Mapping Overview
//
// The following table summarises how hyper concepts map to JSON:API:
//
//   - Representation.Kind   → data.type
//   - Representation.Self   → data.id (extracted from URL) + data.links.self
//   - Representation.State  → data.attributes
//   - Representation.Links  → data.links and/or data.relationships.*.links
//   - Representation.Embedded → included array + data.relationships
//   - Representation.Meta   → meta (top-level or resource-level)
//   - Representation.Actions → meta.actions (extension; JSON:API has no action concept)
//   - Representation.Hints  → omitted (no JSON:API equivalent)
//
// # Relationship Detection
//
// A Link is treated as a JSON:API relationship (rather than a plain link) when its
// Rel matches a key in Representation.Embedded. All other links appear in
// data.links.
//
// # ID Extraction
//
// The resource ID is determined by the following precedence:
//  1. State["id"] if present (removed from attributes to avoid duplication)
//  2. Last path segment of the resolved Self URL
//  3. Empty string if neither is available
//
// # Limitations
//
//   - Actions and Fields have no JSON:API equivalent and are placed in
//     meta.actions as a non-standard extension.
//   - Hints are omitted entirely.
//   - Round-trip fidelity is not guaranteed for all hyper features.
package jsonapi
