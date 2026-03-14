<template>
  <main id="task-list-root" class="tasklist-page">
    <link v-if="renderDocument" rel="stylesheet" href="/style.css">
    <script v-if="renderDocument" src="https://unpkg.com/htmx.org@2.0.4"></script>
    <header class="page-header">
      <h1>Task List</h1>
      <p>{{ taskCount }} total</p>
    </header>

    <section class="task-create" aria-labelledby="create-task-heading">
      <h2 id="create-task-heading">Add Task</h2>
      <task-form v-bind="actions.create"></task-form>
    </section>

    <section class="task-section" aria-labelledby="tasks-heading">
      <div class="tasks-header">
        <h2 id="tasks-heading">Tasks</h2>
      </div>
      <ul class="task-items">
        <li v-for="task in embedded.items" class="task-item">
          <task-row v-bind="task"></task-row>
        </li>
      </ul>
      <p v-if="noTasks" class="task-empty">No tasks yet.</p>
    </section>
  </main>
</template>
