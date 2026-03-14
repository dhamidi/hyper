<template>
  <div>
    <h1>Task List</h1>
    <task-form v-bind="createAction"></task-form>
    <section>
      <h2>Tasks</h2>
      <task-item v-for="task in tasks" v-bind="task"></task-item>
      <p v-if="noTasks">No tasks yet.</p>
    </section>
  </div>
</template>
