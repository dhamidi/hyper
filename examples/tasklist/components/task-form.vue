<template>
  <form method="POST" action="/tasks">
    <h2>create</h2>
    <label>Title
      <input type="text" name="title" :value="titleValue" required>
      <em v-if="titleError">{{ titleError }}</em>
    </label>
    <label>Status
      <select name="status">
        <option v-for="opt in statusOptions" :value="opt.value" :selected="opt.selected">{{ opt.label }}</option>
      </select>
    </label>
    <button type="submit">create</button>
  </form>
</template>
