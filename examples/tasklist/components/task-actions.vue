<template>
  <div class="task-actions">
    <form class="task-action-form" method="POST" :action="toggle.href" v-bind="toggle.hxAttrs">
      <button type="submit">toggle</button>
    </form>
    <form class="task-action-form" method="POST" :action="delete.href" v-bind="delete.hxAttrs">
      <input type="hidden" name="_method" value="DELETE">
      <button type="submit" class="destructive">delete</button>
    </form>
  </div>
</template>
