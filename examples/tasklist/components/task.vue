<template>
  <article data-kind="task">
    <dl>
      <dt>title</dt><dd>{{ title }}</dd>
      <dt>status</dt><dd>{{ status }}</dd>
    </dl>
    <form method="POST" :action="actions.toggle.href" v-bind="actions.toggle.hxAttrs">
      <button type="submit">toggle</button>
    </form>
    <form method="POST" :action="actions.delete.href" class="destructive" v-bind="actions.delete.hxAttrs">
      <input type="hidden" name="_method" value="DELETE">
      <button type="submit" class="destructive">delete</button>
    </form>
  </article>
</template>
