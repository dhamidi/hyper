<template>
  <div class="flex items-center gap-2 shrink-0">
    <form method="POST" :action="toggle.href" v-bind="toggle.hxAttrs">
      <button type="submit" class="inline-flex items-center border-2 border-solid border-zinc-700 px-3 py-1 text-lg leading-none hover:bg-zinc-800 hover:text-stone-100 focus:outline-none focus-visible:outline focus-visible:outline-2 focus-visible:outline-zinc-800">toggle</button>
    </form>
    <form method="POST" :action="delete.href" v-bind="delete.hxAttrs">
      <input type="hidden" name="_method" value="DELETE">
      <button type="submit" class="inline-flex items-center border-2 border-solid border-red-900 text-red-900 px-3 py-1 text-lg leading-none hover:bg-red-900 hover:text-stone-100 focus:outline-none focus-visible:outline focus-visible:outline-2 focus-visible:outline-red-900">delete</button>
    </form>
  </div>
</template>
