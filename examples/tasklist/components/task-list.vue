<template>
  <page-layout v-if="renderDocument" :taskCount="taskCount" :actions="actions" :embedded="embedded" :noTasks="noTasks" :rootHxAttrs="rootHxAttrs"></page-layout>
  <task-list-content v-else :taskCount="taskCount" :actions="actions" :embedded="embedded" :noTasks="noTasks" :rootHxAttrs="rootHxAttrs"></task-list-content>
</template>
