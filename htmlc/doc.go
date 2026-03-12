// Package htmlc provides helpers for rendering hyper.Representation values
// as HTML using a Vue.js SFC-style template engine.
//
// The package includes three reusable Vue component templates in the
// components/ directory:
//
//   - action.vue: Renders a hyper.Action as a link, button, or form based
//     on the action's method and fields. Supports slot-based customization.
//   - field.vue: Renders a hyper.Field as the appropriate HTML input element
//     (text, select, checkbox, textarea, etc.).
//   - actions.vue: Enumerates all non-hidden actions from a representation's
//     actionList, rendering each via the action component.
//
// RepresentationToScope converts representations into template scopes that
// these components consume directly. Each action scope includes computed
// properties (hasFields, isGet, formMethod) that drive the action component's
// element selection logic.
package htmlc
