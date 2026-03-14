<template>
  <form method="POST" :action="href" v-bind="hxAttrs">
    <h2>{{ name }}</h2>
    <template v-for="field in fields">
      <label v-if="field.type !== 'hidden' && field.type !== 'select'">{{ field.label }}
        <input :type="field.type" :name="field.name" :value="field.value" :required="field.required">
        <em v-if="field.error">{{ field.error }}</em>
      </label>
      <label v-if="field.type === 'select'">{{ field.label }}
        <select :name="field.name">
          <option v-for="opt in field.options" :value="opt.value" :selected="opt.selected">{{ opt.label }}</option>
        </select>
      </label>
    </template>
    <button type="submit">{{ name }}</button>
  </form>
</template>
