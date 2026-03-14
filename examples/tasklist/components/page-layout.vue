<template>
  <main id="task-list-root" class="tasklist-page">
    <meta name="htmx-config" content='{"responseHandling":[{"code":"204","swap":false},{"code":"[23]..","swap":true},{"code":"422","swap":true},{"code":"[45]..","swap":false},{"code":"...","swap":true}]}' />
    <link rel="stylesheet" href="/style.css">
    <script src="https://unpkg.com/htmx.org@2.0.4"></script>
    <task-list-content :taskCount="taskCount" :actions="actions" :embedded="embedded" :noTasks="noTasks" :rootHxAttrs="rootHxAttrs"></task-list-content>
  </main>
</template>
