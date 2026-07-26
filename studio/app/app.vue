<script setup lang="ts">
// The Studio shell: a schema-driven left nav (doc-types come from the registry,
// never hardcoded — so an Etude tenant sees Course/Lesson while a marketing
// space sees Page/Post) + the routed editor surface.
const { schema } = useResonans()
const { data: sch, error } = await useAsyncData('schema', () => schema())
</script>

<template>
  <div class="rz-shell">
    <aside class="rz-sidebar">
      <div class="rz-brand">Resonans <span class="rz-chip">Studio</span></div>
      <nav class="rz-nav">
        <NuxtLink v-for="dt in sch?.docTypes || []" :key="dt.type" :to="`/${dt.type}`" class="rz-nav__item">
          {{ dt.type }}
        </NuxtLink>
        <p v-if="error" class="rz-field__hint" style="padding: 0 12px">
          Kan ikke hente schema — kører backend'en? ({{ (error as Error).message }})
        </p>
      </nav>
    </aside>
    <main class="rz-main">
      <NuxtPage />
    </main>
  </div>
</template>

<style scoped>
.rz-shell {
  display: grid;
  grid-template-columns: var(--rz-sidebar-w) 1fr;
  height: 100vh;
}
.rz-sidebar {
  background: var(--rz-bg-subtle);
  border-right: 1px solid var(--rz-border);
  padding: 16px 0;
  overflow: auto;
}
.rz-brand {
  font-weight: 700;
  padding: 0 16px 16px;
}
.rz-nav {
  display: flex;
  flex-direction: column;
}
.rz-nav__item {
  padding: 8px 16px;
  text-transform: capitalize;
  color: var(--rz-text-muted);
}
.rz-nav__item.router-link-active {
  color: var(--rz-text);
  background: var(--rz-surface);
  border-left: 2px solid var(--rz-primary);
  font-weight: 600;
}
.rz-main {
  overflow: auto;
  padding: 24px 28px;
}
</style>
