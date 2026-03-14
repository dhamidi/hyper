<template>
  <main id="task-list-root" class="min-h-screen bg-stone-100 text-zinc-800 font-mono px-8 py-8">
    <meta name="htmx-config" content='{"responseHandling":[{"code":"204","swap":false},{"code":"[23]..","swap":true},{"code":"422","swap":true},{"code":"[45]..","swap":false},{"code":"...","swap":true}]}' />
    <style>
      :root {
        --spacing: 0.25rem;
        --font-mono: "Courier New", Courier, monospace;
        --font-weight-semibold: 600;
        --text-xs: 0.75rem;
        --text-xs--line-height: 1rem;
        --text-sm: 0.875rem;
        --text-sm--line-height: 1.25rem;
        --text-lg: 1.125rem;
        --text-lg--line-height: 1.75rem;
        --text-xl: 1.25rem;
        --text-xl--line-height: 1.75rem;
        --text-2xl: 1.5rem;
        --text-2xl--line-height: 2rem;
        --text-3xl: 1.875rem;
        --text-3xl--line-height: 2.25rem;
        --text-5xl: 3rem;
        --text-5xl--line-height: 1;
        --color-stone-100: #f5f5f4;
        --color-zinc-500: #71717a;
        --color-zinc-700: #3f3f46;
        --color-zinc-800: #27272a;
        --color-red-900: #7f1d1d;
      }
    </style>
    <link rel="stylesheet" :href="tailwindHref">
    <script src="https://unpkg.com/htmx.org@2.0.4"></script>
    <div class="mx-auto max-w-3xl">
      <task-list-content :taskCount="taskCount" :actions="actions" :embedded="embedded" :noTasks="noTasks" :rootHxSwapOob="rootHxSwapOob"></task-list-content>
    </div>
  </main>
</template>
