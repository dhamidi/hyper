<!-- action.vue renders a hyper.Action as the appropriate HTML element.
     It receives an action scope (from representationToScope) via v-bind.

     Element mapping:
       - GET with no fields  → <a> link
       - non-GET with no fields → <button> with hx-* attributes
       - has fields → <form> with nested <field> components

     Computed properties:
       - isGet: method === "GET"
       - hasFields: fields && fields.length > 0
       - formMethod: "GET" stays "GET", everything else becomes "POST"
       - hxAttrs: hx-* attribute map from Action.Hints
       - formHxAttrs: for forms, hx-* attributes go on the <form> element

     Slots:
       - default: custom button/link content (overrides action.name)
       - fields: custom field layout (overrides auto-generated fields)
       - submit: custom submit button content
-->
<template>
  <!-- GET actions with no fields: render as link -->
  <a
    v-if="!hasFields && isGet"
    v-bind="hxAttrs"
    :href="href">
    <slot>{{ name }}</slot>
  </a>

  <!-- Non-GET actions with no fields: render as button -->
  <button
    v-if="!hasFields && !isGet"
    v-bind="hxAttrs"
    :class="{ destructive: hints && hints.destructive }"
    type="button">
    <slot>{{ name }}</slot>
  </button>

  <!-- Actions with fields: render as form -->
  <form
    v-if="hasFields"
    :method="formMethod"
    :action="href"
    v-bind="formHxAttrs">
    <slot name="fields">
      <field v-for="f in fields" :key="f.name" v-bind="f" />
    </slot>
    <button type="submit">
      <slot name="submit">{{ name }}</slot>
    </button>
  </form>
</template>
