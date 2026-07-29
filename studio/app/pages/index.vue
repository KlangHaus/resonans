<script setup lang="ts">
// Spaces-forsiden (Storyblok-mønstret): kort-grid over spaces m/ søgning, sort
// og "+ Tilføj Space". Et space = en tenant (Beslutning K) — klik går IND i
// spacet (sætter aktiv tenant) og videre til første content-type.
const { spaces, enter } = useSpaces()
const { schema } = useResonans()

const q = ref('')
const sort = ref<'updated' | 'name'>('updated')

const filtered = computed(() => {
  const list = spaces.value.filter((s) => s.name.toLowerCase().includes(q.value.toLowerCase()))
  return sort.value === 'name'
    ? [...list].sort((a, b) => a.name.localeCompare(b.name))
    : [...list].sort((a, b) => b.createdAt.localeCompare(a.createdAt))
})

const covers = ['cov0', 'cov1', 'cov2', 'cov3']

async function open(id: string) {
  enter(id)
  // Ind i spacet: første aktive content-type (profil-filtreret pr. space).
  const sch = await schema()
  navigateTo(`/${sch.docTypes[0]?.type || 'post'}`)
}

function since(iso: string) {
  const days = Math.floor((Date.now() - new Date(iso).getTime()) / 86_400_000)
  if (days < 1) return 'Opdateret i dag'
  if (days === 1) return 'Opdateret i går'
  return `Opdateret for ${days} dage siden`
}
</script>

<template>
  <div class="sp">
    <div class="sp__head">
      <h1>Spaces</h1>
      <NuxtLink to="/spaces/new" class="rz-btn rz-btn--primary">+ Tilføj Space</NuxtLink>
    </div>

    <div class="sp__tools">
      <input v-model="q" class="rz-input sp__search" placeholder="Søg i spaces…" />
      <select v-model="sort" class="rz-input sp__sort">
        <option value="updated">Sortér: Senest opdateret</option>
        <option value="name">Sortér: Navn</option>
      </select>
    </div>

    <div class="sp__label">Mine Spaces · {{ filtered.length }}</div>

    <div class="sp__grid">
      <button
        v-for="(s, i) in filtered"
        :key="s.id"
        type="button"
        class="sp__card"
        @click="open(s.id)"
      >
        <div class="sp__cardtop">
          <strong>{{ s.name }}</strong>
          <span class="sp__dots">⋯</span>
        </div>
        <div class="sp__cover" :class="covers[i % covers.length]" />
        <div class="sp__cardfoot">
          <span class="sp__avatar">{{ s.name.slice(0, 2).toUpperCase() }}</span>
          <span class="sp__meta">{{ since(s.createdAt) }}</span>
          <span class="sp__badges"><span class="rz-chip">Staging</span> <span class="rz-chip">EU</span></span>
        </div>
      </button>
    </div>

    <p v-if="!filtered.length" class="rz-field__hint">Ingen spaces matcher søgningen.</p>
  </div>
</template>

<style scoped>
.sp__head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}
.sp__head h1 {
  margin: 0;
  font-size: 26px;
  font-weight: 800;
}
.sp__tools {
  display: flex;
  gap: 10px;
  margin-bottom: 20px;
}
.sp__search {
  flex: 1;
}
.sp__sort {
  width: 220px;
}
.sp__label {
  font-size: 12.5px;
  font-weight: 600;
  color: var(--rz-text-muted, #6b6b78);
  margin-bottom: 12px;
}
.sp__grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
  gap: 18px;
}
.sp__card {
  background: var(--rz-raised, #fff);
  border: 1px solid var(--rz-border, #e6e2da);
  border-radius: 12px;
  padding: 14px;
  text-align: left;
  cursor: pointer;
  font-family: inherit;
  color: inherit;
  transition: transform 0.15s ease, box-shadow 0.15s ease;
}
.sp__card:hover {
  transform: translateY(-2px);
  box-shadow: var(--rz-shadow-md, 0 8px 26px rgba(26, 26, 46, 0.1));
}
.sp__cardtop {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 10px;
  font-size: 15px;
}
.sp__dots {
  color: var(--rz-text-muted, #6b6b78);
}
.sp__cover {
  height: 64px;
  border-radius: 8px;
  margin-bottom: 12px;
}
.cov0 { background: linear-gradient(135deg, #2c2c50, #e94560); }
.cov1 { background: linear-gradient(135deg, #1a1a2e, #c4a35a); }
.cov2 { background: linear-gradient(135deg, #e94560, #c4a35a); }
.cov3 { background: linear-gradient(135deg, #23233f, #3a3a5c); }
.sp__cardfoot {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 11.5px;
  color: var(--rz-text-muted, #6b6b78);
}
.sp__avatar {
  width: 24px;
  height: 24px;
  border-radius: 999px;
  background: linear-gradient(135deg, #e94560, #c4a35a);
  color: #1a1a2e;
  display: grid;
  place-items: center;
  font-size: 9px;
  font-weight: 800;
}
.sp__meta {
  flex: 1;
}
</style>
