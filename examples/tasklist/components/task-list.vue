<template>
  <page-layout v-if="renderDocument" :tailwindHref="tailwindHref" :taskCount="taskCount" :actions="actions" :embedded="embedded" :noTasks="noTasks" :rootHxSwapOob="rootHxSwapOob"></page-layout>
  <task-list-content v-else :taskCount="taskCount" :actions="actions" :embedded="embedded" :noTasks="noTasks" :rootHxSwapOob="rootHxSwapOob"></task-list-content>
</template>
