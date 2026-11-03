<script setup lang="ts">
// Content: the COMBINED document list. Content types are not nav items; they are a
// COLUMN here and a CHOICE when creating. Lists all active types with search, type
// chips and a status filter, most recently updated first.
import type { Document } from '~/composables/useResonans'

const { t, locale } = useT()
const { schema, listDocuments, listDeletedDocuments, restoreDocument } = useResonans()
const { activeId } = useSpaces()
const cfgPub = useRuntimeConfig().public
// With no space context (no active space and no dev tenant) the API would answer
// 403, so send the user to the Spaces landing page instead.
if (!activeId.value && !(cfgPub.devTenant as string)) {
  navigateTo('/')
}

// 🔴 THE BIN IS ITS OWN CALL, not a filter on the list. The server leaves deleted
// documents out of the ordinary list on purpose (it would rather hide too much than
// show deleted content), so the bin exists only when asked for.
const showBin = ref(false)
const { data, pending, refresh } = await useAsyncData(
  'content-all',
  async () => {
    const sch = await schema()
    const types = sch.docTypes.map((d) => d.type)
    const fetchOne = showBin.value ? listDeletedDocuments : (ty: string) => listDocuments(ty)
    const perType = await Promise.all(
      types.map(async (ty) => await fetchOne(ty).catch(() => [] as Document[])),
    )
    return { types, docs: perType.flat() }
  },
  { watch: [showBin] },
)

const restoring = ref('')
async function restore(d: Document) {
  if (restoring.value) return
  restoring.value = d.id
  try {
    await restoreDocument(d.id)
    await refresh()
  } catch (e: unknown) {
    // A 409 here almost always means ONE thing: a new page has taken the slug.
    // Say so, rather than showing "409".
    const status = (e as { statusCode?: number })?.statusCode
    alert(
      status === 409
        ? t('"{title}" cannot be restored, because another page already uses the address /{slug}. Rename or delete the other page first.', { title: d.title, slug: d.slug })
        : t('Could not restore: {message}', { message: (e as { message?: string })?.message ?? t('unknown error') }),
    )
  } finally {
    restoring.value = ''
  }
}

const q = ref('')
const typeFilter = ref<string>('')
const statusFilter = ref<string>('')

const rows = computed(() => {
  let list = data.value?.docs || []
  if (typeFilter.value) list = list.filter((d) => d.type === typeFilter.value)
  if (statusFilter.value) list = list.filter((d) => d.status === statusFilter.value)
  const needle = q.value.trim().toLowerCase()
  if (needle) list = list.filter((d) => d.title.toLowerCase().includes(needle) || d.slug.includes(needle))
  return [...list].sort((a, b) => (b.updatedAt || '').localeCompare(a.updatedAt || ''))
})

function fmt(iso?: string) {
  return iso ? new Date(iso).toLocaleString(locale, { day: '2-digit', month: '2-digit', year: 'numeric', hour: '2-digit', minute: '2-digit' }) : '—'
}
function cap(s: string) {
  return s.charAt(0).toUpperCase() + s.slice(1)
}
</script>

<template>
  <div style="max-width: 980px">
    <div class="ct__head">
      <h1>{{ t('Content') }}</h1>
      <NuxtLink to="/content/new" class="rz-btn rz-btn--primary">+ {{ t('Create new') }}</NuxtLink>
    </div>

    <div class="ct__tools">
      <input v-model="q" class="rz-input" :placeholder="t('Search content…')" style="flex: 1" />
      <select v-model="typeFilter" class="rz-input" style="width: 170px">
        <option value="">{{ t('All types') }}</option>
        <option v-for="t in data?.types || []" :key="t" :value="t">{{ cap(t) }}</option>
      </select>
      <button
        type="button"
        class="rz-btn"
        :class="{ 'rz-btn--primary': showBin }"
        :title="showBin ? t('Show active content') : t('Show deleted documents')"
        @click="showBin = !showBin"
      >
        {{ showBin ? `← ${t('Active content')}` : `🗑 ${t('Bin')}` }}
      </button>
      <select v-show="!showBin" v-model="statusFilter" class="rz-input" style="width: 150px">
        <option value="">{{ t('All statuses') }}</option>
        <option value="published">{{ t('Published') }}</option>
        <option value="draft">{{ t('Draft') }}</option>
      </select>
    </div>

    <div class="ct__cols">
      <span style="flex: 1">{{ t('Name') }}</span>
      <span style="width: 110px">{{ t('Type') }}</span>
      <span style="width: 90px">{{ t('Status') }}</span>
      <span style="width: 130px; text-align: right">{{ t('Last updated') }}</span>
    </div>

    <p v-if="pending" class="rz-field__hint">{{ t('Fetching…') }}</p>
    <p v-else-if="!rows.length" class="rz-field__hint" style="padding: 18px 0">
      <template v-if="showBin">{{ t('The bin is empty.') }}</template>
      <template v-else>{{
        q || typeFilter || statusFilter
          ? t('No content matches the filters.')
          : t('No content yet. Create the first with "+ Create new".')
      }}</template>
    </p>

    <!--
      In the bin the row is NOT a link: opening a deleted document and being able to
      edit it would promise something the server refuses (409). The row offers exactly
      one action: restore.
    -->
    <component
      :is="showBin ? 'div' : resolveComponent('NuxtLink')"
      v-for="d in rows"
      :key="d.id"
      v-bind="showBin ? {} : { to: `/${d.type}/${d.id}` }"
      class="ct__row"
      :class="{ 'ct__row--bin': showBin }"
    >
      <span class="ct__name">
        <strong>{{ d.title }}</strong>
        <em>{{ d.slug }}</em>
      </span>
      <span style="width: 110px"><span class="rz-chip">{{ cap(d.type) }}</span></span>
      <span style="width: 90px">
        <span
          class="rz-chip"
          :style="d.status === 'published' ? 'color: var(--rz-success, #198754)' : ''"
        >{{ d.status === 'published' ? t('Published') : t('Draft') }}</span>
      </span>
      <span v-if="showBin" class="ct__time">
        {{ t('Deleted {date}', { date: fmt(d.deletedAt) }) }}
        <button type="button" class="rz-btn ct__restore" :disabled="restoring === d.id" @click="restore(d)">
          {{ restoring === d.id ? t('Restoring…') : t('Restore') }}
        </button>
      </span>
      <span v-else class="ct__time">{{ fmt(d.updatedAt) }}</span>
    </component>

    <p v-if="rows.length" class="rz-field__hint" style="margin-top: 12px">
      {{ rows.length === 1 ? t('{n} document', { n: rows.length }) : t('{n} documents', { n: rows.length }) }}
      <button type="button" class="ct__refresh" @click="refresh()">↻ {{ t('Refresh') }}</button>
    </p>
  </div>
</template>

<style scoped>
.ct__head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}
.ct__head h1 {
  margin: 0;
  font-size: 26px;
  font-weight: 800;
}
.ct__tools {
  display: flex;
  gap: 10px;
  margin-bottom: 18px;
}
.ct__cols {
  display: flex;
  gap: 12px;
  padding: 0 14px 8px;
  font-size: 11.5px;
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--rz-text-muted, #6b6b78);
  border-bottom: 1px solid var(--rz-border, #e6e2da);
}
.ct__row {
  display: flex;
  gap: 12px;
  align-items: center;
  padding: 12px 14px;
  border-bottom: 1px solid var(--rz-border, #e6e2da);
  color: inherit;
  font-size: 14px;
}
.ct__row:hover {
  background: var(--rz-surface, #f4f2ee);
}
.ct__name {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 2px;
  overflow: hidden;
}
.ct__name strong {
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.ct__name em {
  font-style: normal;
  font-family: var(--rz-font-mono, ui-monospace, monospace);
  font-size: 11.5px;
  color: var(--rz-text-muted, #6b6b78);
}
.ct__time {
  width: 130px;
  text-align: right;
  font-size: 12px;
  color: var(--rz-text-muted, #6b6b78);
  font-family: var(--rz-font-mono, ui-monospace, monospace);
}
.ct__refresh {
  margin-left: 10px;
  background: none;
  border: 0;
  color: var(--rz-primary, #e94560);
  cursor: pointer;
  font-family: inherit;
  font-size: 12px;
}
.ct__row--bin {
  cursor: default;
  opacity: 0.85;
}
.ct__restore {
  margin-left: 10px;
  padding: 3px 9px;
  font-size: 11.5px;
}
</style>
