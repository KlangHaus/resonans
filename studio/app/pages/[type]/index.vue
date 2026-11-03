<script setup lang="ts">
// Document list for one doc-type — a generic CMS list (not LMS-specific). v1
// shows title/status/updatedAt metadata; per-type list.columns (schema-driven)
// is a follow-up refinement.
const { t } = useT()
const route = useRoute()
const type = computed(() => route.params.type as string)
const { listDocuments, createDocument } = useResonans()

const { data: docs, refresh, error } = await useAsyncData(
  () => `docs-${type.value}`,
  () => listDocuments(type.value),
  { watch: [type] },
)

async function create() {
  const slug = window.prompt(t('New {type}: slug?', { type: type.value }))
  if (!slug) return
  const title = window.prompt(t('Title?')) || slug
  try {
    const d = await createDocument({ type: type.value, slug, title })
    await navigateTo(`/${type.value}/${d.id}`)
  } catch (e) {
    window.alert(t('Could not create: {message}', { message: (e as Error).message }))
  }
}
</script>

<template>
  <div>
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px">
      <h1 style="text-transform: capitalize; margin: 0">{{ type }}</h1>
      <button class="rz-btn rz-btn--primary" @click="create">{{ t('New {type}', { type }) }}</button>
    </div>

    <p v-if="error" class="rz-field__hint" style="color: var(--rz-danger)">{{ (error as Error).message }}</p>

    <table v-if="docs?.length" style="width: 100%; border-collapse: collapse">
      <thead>
        <tr style="text-align: left; border-bottom: 1px solid var(--rz-border)">
          <th style="padding: 8px">{{ t('Title') }}</th>
          <th style="padding: 8px">{{ t('Slug') }}</th>
          <th style="padding: 8px">{{ t('Status') }}</th>
          <th style="padding: 8px">{{ t('Updated') }}</th>
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
    <p v-else class="rz-field__hint">{{ t('No {type} documents yet.', { type }) }}</p>
  </div>
</template>
