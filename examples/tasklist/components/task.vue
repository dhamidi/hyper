<template>
  <article data-kind="task">
    <div class="task-row">
      <div class="task-meta">
        <strong class="task-title">{{ title }}</strong>
        <span class="task-status">{{ status }}</span>
      </div>
      <div class="task-actions">
        <form class="task-action-form" method="POST" :action="actions.toggle.href" v-bind="actions.toggle.hxAttrs">
          <button type="submit">toggle</button>
        </form>
        <form class="task-action-form" method="POST" :action="actions.delete.href" v-bind="actions.delete.hxAttrs">
          <input type="hidden" name="_method" value="DELETE">
          <button type="submit" class="destructive">delete</button>
        </form>
      </div>
    </div>
  </article>
</template>
