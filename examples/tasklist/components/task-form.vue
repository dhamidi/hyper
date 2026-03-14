<template>
  <form id="task-create-form" method="POST" :action="href" v-bind="hxAttrs" class="space-y-4">
    <template v-for="field in fields">
      <label v-if="field.type !== 'hidden' && field.type !== 'select'" class="block space-y-1 font-semibold text-xl">{{ field.label }}
        <input :type="field.type" :name="field.name" :value="field.value || ''" :required="field.required" class="w-full border-0 border-b-2 border-zinc-700 bg-transparent px-0 py-1 text-xl outline-none focus:border-zinc-900">
        <em v-if="field.error" class="block text-sm text-red-900">{{ field.error }}</em>
      </label>
      <label v-if="field.type === 'select'" class="block space-y-1 font-semibold text-xl">{{ field.label }}
        <select :name="field.name" class="w-full border-0 border-b-2 border-zinc-700 bg-transparent px-0 py-1 text-xl outline-none focus:border-zinc-900">
          <option v-for="opt in field.options" :value="opt.value" :selected="opt.selected">{{ opt.label }}</option>
        </select>
      </label>
    </template>
    <button type="submit" class="inline-flex items-center border-2 border-solid border-zinc-700 px-4 py-1 text-lg leading-none hover:bg-zinc-800 hover:text-stone-100 focus:outline-none focus-visible:outline focus-visible:outline-2 focus-visible:outline-zinc-800">{{ name }}</button>
  </form>
</template>
