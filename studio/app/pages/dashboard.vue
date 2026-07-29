<script setup lang="ts">
// Space-dashboard (Storyblok-mønstret, Allan 2026-07-28): overblik når man går
// ind i et space — info-bar (miljø/region/profil), stat-fliser med ÆGTE tal fra
// API'et og seneste aktivitet (senest opdaterede dokumenter på tværs af typer).
import type { Document } from '~/composables/useResonans'

const { active } = useSpaces()
const { schema, listDocuments, getSettings } = useResonans()

const { data, pending } = await useAsyncData('dashboard', async () => {
  const [sch, settings] = await Promise.all([
    schema(),
    getSettings().catch(() => ({ enabledTypes: [], allTypes: [] as string[] })),
  ])
  const types = sch.docTypes.map((d) => d.type)
  const perType = await Promise.all(
    types.map(async (t) => ({ type: t, docs: await listDocuments(t).catch(() => [] as Document[]) })),
  )
  const allDocs = perType.flatMap((p) => p.docs)
  const recent = [...allDocs]
    .sort((a, b) => (b.updatedAt || '').localeCompare(a.updatedAt || ''))
    .slice(0, 8)
  return {
    types,
    blocks: sch.blocks.length,
    enabledCount: (settings.enabledTypes?.length ?? 0) || settings.allTypes.length || sch.docTypes.length,
    allCount: settings.allTypes.length || sch.docTypes.length,
    perType: perType.map((p) => ({ type: p.type, count: p.docs.length })),
    total: allDocs.length,
    published: allDocs.filter((d) => d.status === 'published').length,
    recent,
    lastUpdated: recent[0]?.updatedAt || null,
  }
})

const tiles = computed(() => {
  if (!data.value) return []
  const d = data.value
  return [
    { label: 'Dokumenter', value: d.total, sub: `${d.published} udgivet`, ic: '▤', bg: '#eef2ff' },
    { label: 'Indholdstyper', value: `${d.enabledCount} / ${d.allCount}`, sub: 'aktive i profilen', ic: '☰', bg: '#ecfdf5' },
    { label: 'Blok-bibliotek', value: d.blocks, sub: 'blokke til rådighed', ic: '▦', bg: '#fef3e2' },
    ...d.perType.filter((p) => p.count > 0).slice(0, 5).map((p) => ({
      label: p.type.charAt(0).toUpperCase() + p.type.slice(1),
      value: p.count,
      sub: 'dokumenter',
      ic: '▢',
      bg: '#fdf2f4',
    })),
  ]
})

function fmt(iso?: string | null) {
  return iso ? new Date(iso).toLocaleString('da-DK', { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' }) : '—'
}
</script>

<template>
  <div class="db" style="max-width: 880px">
    <div class="db__head">
      <div>
        <h1>{{ active?.name || 'Space' }}</h1>
        <p class="rz-field__hint" style="margin: 2px 0 0">
          Senest opdateret {{ fmt(data?.lastUpdated) }}
        </p>
      </div>
    </div>

    <!-- Info-bar (Storybloks "Plan Details"-mønster) -->
    <div class="db__plan">
      <div class="db__planitem"><span>Miljø</span><strong>Staging</strong></div>
      <div class="db__planitem"><span>Region</span><strong>EU</strong></div>
      <div class="db__planitem"><span>Content-profil</span><strong>{{ data?.enabledCount }} / {{ data?.allCount }} typer</strong></div>
      <div class="db__planitem"><span>Tenant</span><strong style="font-family: var(--rz-font-mono, monospace); font-size: 12px">{{ (active?.id || '').slice(0, 8) }}…</strong></div>
      <NuxtLink to="/settings" class="db__upgrade">Indstillinger</NuxtLink>
    </div>

    <p v-if="pending" class="rz-field__hint">Henter overblik…</p>

    <!-- Stat-fliser -->
    <div class="db__tiles">
      <div v-for="t in tiles" :key="t.label" class="db__tile">
        <span class="db__tileic" :style="{ background: t.bg }">{{ t.ic }}</span>
        <div>
          <div class="db__tilelabel">{{ t.label }}</div>
          <div class="db__tilevalue">{{ t.value }}</div>
          <div class="db__tilesub">{{ t.sub }}</div>
        </div>
      </div>
    </div>

    <!-- Aktivitet -->
    <div class="db__activity">
      <h2>Aktivitet</h2>
      <p v-if="!data?.recent.length" class="rz-field__hint">Ingen dokumenter endnu — opret dit første fra menuen til venstre.</p>
      <NuxtLink v-for="d in data?.recent || []" :key="d.id" :to="`/${d.type}/${d.id}`" class="db__row">
        <span class="rz-chip">{{ d.type }}</span>
        <span class="db__rowtitle">{{ d.title }}</span>
        <span class="rz-chip" :style="d.status === 'published' ? 'color: var(--rz-success)' : ''">{{ d.status }}</span>
        <span class="db__rowtime">{{ fmt(d.updatedAt) }}</span>
      </NuxtLink>
    </div>
  </div>
</template>

<style scoped>
.db__head h1 {
  margin: 0;
  font-size: 26px;
  font-weight: 800;
}
.db__plan {
  display: flex;
  align-items: center;
  gap: 28px;
  background: #1a1a2e;
  color: #fff;
  border-radius: 12px;
  padding: 16px 20px;
  margin: 18px 0 20px;
}
.db__planitem {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.db__planitem span {
  font-size: 11px;
  color: rgba(255, 255, 255, 0.6);
}
.db__planitem strong {
  font-size: 14px;
}
.db__upgrade {
  margin-left: auto;
  background: #fff;
  color: #1a1a2e;
  font-weight: 600;
  font-size: 13px;
  padding: 8px 16px;
  border-radius: 8px;
}
.db__tiles {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(190px, 1fr));
  gap: 14px;
  margin-bottom: 26px;
}
.db__tile {
  display: flex;
  gap: 12px;
  align-items: flex-start;
  background: var(--rz-raised, #fff);
  border: 1px solid var(--rz-border, #e6e2da);
  border-radius: 10px;
  padding: 14px;
}
.db__tileic {
  width: 34px;
  height: 34px;
  border-radius: 8px;
  display: grid;
  place-items: center;
  font-size: 15px;
  color: #1a1a2e;
  flex: none;
}
.db__tilelabel {
  font-size: 12px;
  color: var(--rz-text-muted, #6b6b78);
}
.db__tilevalue {
  font-size: 20px;
  font-weight: 800;
  line-height: 1.2;
}
.db__tilesub {
  font-size: 11px;
  color: var(--rz-text-muted, #6b6b78);
}
.db__activity h2 {
  font-size: 17px;
  font-weight: 700;
  margin: 0 0 10px;
}
.db__row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 12px;
  border: 1px solid var(--rz-border, #e6e2da);
  border-radius: 9px;
  margin-bottom: 6px;
  background: var(--rz-raised, #fff);
  color: inherit;
  font-size: 13.5px;
}
.db__row:hover {
  border-color: var(--rz-primary, #e94560);
}
.db__rowtitle {
  font-weight: 600;
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.db__rowtime {
  font-size: 11.5px;
  color: var(--rz-text-muted, #6b6b78);
  font-family: var(--rz-font-mono, monospace);
}
</style>
