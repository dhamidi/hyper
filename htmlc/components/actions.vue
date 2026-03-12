<!-- actions.vue renders all non-hidden actions for a representation.
     It consumes the actionList array from representationToScope.

     Usage:
       <actions :action-list="actionList" />

     Useful for:
       - Admin interfaces that auto-generate UI from the data model
       - Generic resource browsers that render any representation
       - Prototyping: get a working UI by defining only the representation
-->
<template>
  <div class="actions" v-if="actionList.length > 0">
    <template v-for="a in actionList" :key="a.rel">
      <action v-bind="a" v-if="!a.hints || !a.hints.hidden" />
    </template>
  </div>
</template>
