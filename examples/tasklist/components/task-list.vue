<template>
  <div id="task-list-root">
    <link v-if="renderDocument" rel="stylesheet" href="/style.css">
    <script v-if="renderDocument" src="https://unpkg.com/htmx.org@2.0.4"></script>
    <h1>Task List</h1>
    <task-form v-bind="actions.create"></task-form>
    <section>
      <h2>Tasks</h2>
      <task v-for="task in embedded.items" v-bind="task"></task>
      <p v-if="noTasks">No tasks yet.</p>
    </section>
  </div>
</template>
