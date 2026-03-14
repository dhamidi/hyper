<template>
  <article data-kind="task" class="flex items-center justify-between gap-3">
    <div class="flex min-w-0 items-baseline gap-3">
      <strong class="truncate text-2xl font-semibold">{{ title }}</strong>
      <span class="text-xs uppercase tracking-[0.12em] text-zinc-500">{{ status }}</span>
    </div>
    <task-actions v-bind="actions"></task-actions>
  </article>
</template>
