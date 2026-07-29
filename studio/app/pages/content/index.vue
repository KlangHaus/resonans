<script setup lang="ts">
// Content — den SAMLEDE dokument-liste (Storyblok-mønstret, Allan 2026-07-29):
// content-typerne er ikke nav-punkter; de er en KOLONNE her og et VALG ved
// oprettelse. Liste på tværs af alle aktive typer m/ søgning, type-chips og
// status-filter, sorteret senest-opdateret.
import type { Document } from '~/composables/useResonans'

const { schema, listDocuments } = useResonans()
const { activeId } = useSpaces()
const cfgPub = useRuntimeConfig().public
// Uden space-kontekst (intet aktivt space + ingen dev-tenant) ville API'et
// 403'e — send brugeren til Spaces-forsiden i stedet.
if (!activeId.value && !(cfgPub.devTenant as string)) {
  navigateTo('/')
}

const { data, pending, refresh } = await useAsyncData('content-all', async () => {
  const sch = await schema()
  const types = sch.docTypes.map((d) => d.type)
  const perType = await Promise.all(
    types.map(async (t) => (await listDocuments(t).catch(() => [] as Document[]))),
  )
  return { types, docs: perType.flat() }
})

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
  return iso ? new Date(iso).toLocaleString('da-DK', { day: '2-digit', month: '2-digit', year: 'numeric', hour: '2-digit', minute: '2-digit' }) : '—'
}
function cap(s: string) {
  return s.charAt(0).toUpperCase() + s.slice(1)
}
</script>

<template>
  <div style="max-width: 980px">
    <div class="ct__head">
      <h1>Content</h1>
      <NuxtLink to="/content/new" class="rz-btn rz-btn--primary">+ Opret ny</NuxtLink>
    </div>

    <div class="ct__tools">
      <input v-model="q" class="rz-input" placeholder="Søg i indhold…" style="flex: 1" />
      <select v-model="typeFilter" class="rz-input" style="width: 170px">
        <option value="">Alle typer</option>
        <option v-for="t in data?.types || []" :key="t" :value="t">{{ cap(t) }}</option>
      </select>
      <select v-model="statusFilter" class="rz-input" style="width: 150px">
        <option value="">Alle statusser</option>
        <option value="published">Udgivet</option>
        <option value="draft">Kladde</option>
      </select>
    </div>

    <div class="ct__cols">
      <span style="flex: 1">Navn</span>
      <span style="width: 110px">Type</span>
      <span style="width: 90px">Status</span>
      <span style="width: 130px; text-align: right">Senest opdateret</span>
    </div>

    <p v-if="pending" class="rz-field__hint">Henter…</p>
    <p v-else-if="!rows.length" class="rz-field__hint" style="padding: 18px 0">
      {{ q || typeFilter || statusFilter ? 'Intet indhold matcher filtrene.' : 'Intet indhold endnu — opret det første med "+ Opret ny".' }}
    </p>

    <NuxtLink v-for="d in rows" :key="d.id" :to="`/${d.type}/${d.id}`" class="ct__row">
      <span class="ct__name">
        <strong>{{ d.title }}</strong>
        <em>{{ d.slug }}</em>
      </span>
      <span style="width: 110px"><span class="rz-chip">{{ cap(d.type) }}</span></span>
      <span style="width: 90px">
        <span class="rz-chip" :style="d.status === 'published' ? 'color: var(--rz-success, #198754)' : ''">{{ d.status === 'published' ? 'Udgivet' : 'Kladde' }}</span>
      </span>
      <span class="ct__time">{{ fmt(d.updatedAt) }}</span>
    </NuxtLink>

    <p v-if="rows.length" class="rz-field__hint" style="margin-top: 12px">
      {{ rows.length }} dokument{{ rows.length === 1 ? '' : 'er' }}
      <button type="button" class="ct__refresh" @click="refresh()">↻ Opdatér</button>
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
</style>
