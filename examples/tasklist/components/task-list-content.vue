<template>
  <div id="task-list-content" :hx-swap-oob="rootHxSwapOob" class="space-y-10">
    <header class="space-y-3">
      <h1 class="text-5xl tracking-[0.2em] uppercase border-b-2 border-zinc-800 pb-2">Task List</h1>
      <p class="text-zinc-500 uppercase tracking-[0.12em] text-sm">{{ taskCount }} total</p>
    </header>

    <section aria-labelledby="create-task-heading" class="space-y-4">
      <h2 id="create-task-heading" class="text-3xl tracking-[0.16em] uppercase border-b-2 border-zinc-800 pb-1">Add Task</h2>
      <task-form v-bind="actions.create"></task-form>
    </section>

    <section aria-labelledby="tasks-heading" class="space-y-4">
      <h2 id="tasks-heading" class="text-3xl tracking-[0.16em] uppercase border-b-2 border-zinc-800 pb-1">Tasks</h2>
      <ul class="divide-y divide-zinc-400/60">
        <li v-for="task in embedded.items" class="py-3">
          <task-row v-bind="task"></task-row>
        </li>
      </ul>
      <p v-if="noTasks" class="text-zinc-500">No tasks yet.</p>
    </section>
  </div>
</template>
