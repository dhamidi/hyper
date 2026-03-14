<template>
  <article data-kind="task" class="task-row">
    <div class="task-meta">
      <strong class="task-title">{{ title }}</strong>
      <span class="task-status">{{ status }}</span>
    </div>
    <task-actions v-bind="actions"></task-actions>
  </article>
</template>
