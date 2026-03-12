// Package jsonapi provides codecs that map between hyper.Representation and the
// JSON:API (https://jsonapi.org/) wire format.
//
// This is the recommended codec package for public-facing REST APIs. JSON:API
// enjoys broad ecosystem support (Ember Data, JSONAPI::Resources, Axios
// serializers, etc.) and provides a consistent, well-documented wire format
// that external consumers already understand. Use this package when your API
// serves browser clients, mobile apps, or third-party integrations.
//
// For hyper-to-hyper communication, CLI --json output, debugging, or scenarios
// that require full round-trip fidelity of every hyper feature, use the built-in
// native JSON codec (spec §14.3) instead.
//
// # Quick Start
//
// Register both codecs with a single call:
//
//	repCodec, subCodec := jsonapi.DefaultCodecs()
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
