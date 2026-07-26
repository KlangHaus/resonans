<script setup lang="ts">
// Document list for one doc-type — a generic CMS list (not LMS-specific). v1
// shows title/status/updatedAt metadata; per-type list.columns (schema-driven)
// is a follow-up refinement.
const route = useRoute()
const type = computed(() => route.params.type as string)
const { listDocuments, createDocument } = useResonans()

const { data: docs, refresh, error } = await useAsyncData(
  () => `docs-${type.value}`,
  () => listDocuments(type.value),
  { watch: [type] },
)

async function create() {
  const slug = window.prompt(`Ny ${type.value} — slug?`)
  if (!slug) return
  const title = window.prompt('Titel?') || slug
  try {
    const d = await createDocument({ type: type.value, slug, title })
    await navigateTo(`/${type.value}/${d.id}`)
  } catch (e) {
    window.alert(`Kunne ikke oprette: ${(e as Error).message}`)
  }
}
</script>

<template>
  <div>
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px">
      <h1 style="text-transform: capitalize; margin: 0">{{ type }}</h1>
      <button class="rz-btn rz-btn--primary" @click="create">Ny {{ type }}</button>
    </div>

    <p v-if="error" class="rz-field__hint" style="color: var(--rz-danger)">{{ (error as Error).message }}</p>

    <table v-if="docs?.length" style="width: 100%; border-collapse: collapse">
      <thead>
        <tr style="text-align: left; border-bottom: 1px solid var(--rz-border)">
          <th style="padding: 8px">Titel</th>
          <th style="padding: 8px">Slug</th>
          <th style="padding: 8px">Status</th>
          <th style="padding: 8px">Opdateret</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="d in docs" :key="d.id" style="border-bottom: 1px solid var(--rz-border)">
          <td style="padding: 8px">
            <NuxtLink :to="`/${type}/${d.id}`" style="color: var(--rz-primary)">{{ d.title }}</NuxtLink>
          </td>
          <td style="padding: 8px; color: var(--rz-text-muted)">{{ d.slug }}</td>
          <td style="padding: 8px"><span class="rz-chip">{{ d.status }}</span></td>
          <td style="padding: 8px; color: var(--rz-text-muted)">{{ new Date(d.updatedAt).toLocaleString() }}</td>
        </tr>
      </tbody>
    </table>
    <p v-else class="rz-field__hint">Ingen {{ type }}-dokumenter endnu.</p>
  </div>
</template>
