<template>
  <article data-kind="task">
    <dl>
      <dt>title</dt><dd>{{ title }}</dd>
      <dt>status</dt><dd>{{ status }}</dd>
    </dl>
    <form method="POST" :action="toggleAction" v-bind="toggleHints">
      <button type="submit">toggle</button>
    </form>
    <form method="POST" :action="deleteAction" class="destructive" v-bind="deleteHints">
      <input type="hidden" name="_method" value="DELETE">
      <button type="submit" class="destructive">delete</button>
    </form>
  </article>
</template>
