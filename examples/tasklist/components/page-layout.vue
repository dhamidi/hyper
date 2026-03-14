<template>
  <main id="task-list-root" class="tasklist-page">
    <link rel="stylesheet" href="/style.css">
    <script src="https://unpkg.com/htmx.org@2.0.4"></script>
    <task-list-content :taskCount="taskCount" :actions="actions" :embedded="embedded" :noTasks="noTasks" :rootHxAttrs="rootHxAttrs"></task-list-content>
  </main>
</template>
