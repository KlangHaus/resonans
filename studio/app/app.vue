<script setup lang="ts">
// Studio shell — Storyblok-pattern (Allan-direktiv 2026-07-28): a persistent
// dark rail (My Spaces / My Organization on top; What's New, Help & Inspiration,
// My account at the bottom) + a contextual light panel with the schema-driven
// doc-type nav ONLY when inside a space. What's New is an overlay panel, not a
// page. [designer] delivers the polished forlæg; this is the functional shell.
const route = useRoute()
const { active, leave } = useSpaces()
const { schema } = useResonans()

// The doc-type panel shows only inside a space (not on landing/org/create).
const SHELL_ROUTES = ['/', '/organization', '/spaces/new']
const inSpace = computed(() => !!active.value && !SHELL_ROUTES.includes(route.path))

const { data: sch, error, refresh } = await useAsyncData('schema', () => schema())
// Space-switch → the schema (and its content-profile filtering) must refetch.
watch(() => active.value?.id, () => refresh())

function backToSpaces() {
  leave()
  navigateTo('/')
}

// --- What's New (overlay, Storyblok-style) ---
const showNews = ref(false)
const NEWS = [
  {
    date: '28. jul 2026',
    tag: 'Produkt',
    title: 'Content-profiler pr. space',
    body: 'Til- og fravælg indholdstyper (Web / Undervisning) pr. space under Indstillinger — menuen følger automatisk.',
  },
  {
    date: '28. jul 2026',
    tag: 'Produkt',
    title: 'Visuel blok-editor',
    body: 'Blocks-felter redigeres nu visuelt: tilføj fra paletten, flyt op/ned, slet — plus rich-text med fed/kursiv/overskrifter/citater.',
  },
  {
    date: '28. jul 2026',
    tag: 'Milepæl',
    title: 'KlangHaus Journal er live',
    body: 'Bloggen renderer nu direkte fra Resonans delivery-API — første offentlige flade på CMS-motoren.',
  },
]
</script>

<template>
  <div class="sb-shell">
    <!-- Dark rail -->
    <aside class="sb-rail">
      <NuxtLink to="/" class="sb-rail__brand" @click="leave()">
        <span class="sb-rail__mark">R</span> resonans
      </NuxtLink>

      <nav class="sb-rail__nav">
        <NuxtLink to="/" class="sb-rail__item" :class="{ on: route.path === '/' || inSpace }" @click="leave()">
          <span class="sb-rail__ic">▢</span> Mine Spaces
        </NuxtLink>
        <NuxtLink to="/organization" class="sb-rail__item" :class="{ on: route.path === '/organization' }">
          <span class="sb-rail__ic">🏢</span> Min organisation
        </NuxtLink>
      </nav>

      <div class="sb-rail__bottom">
        <button type="button" class="sb-rail__item" @click="showNews = true">
          <span class="sb-rail__ic">✦</span> Nyheder <span class="sb-rail__badge">{{ NEWS.length }}</span>
        </button>
        <a class="sb-rail__item" href="https://github.com/KlangHaus/resonans" target="_blank" rel="noopener">
          <span class="sb-rail__ic">◎</span> Hjælp & inspiration
        </a>
        <div class="sb-rail__item sb-rail__account">
          <span class="sb-rail__avatar">AC</span> Min konto
        </div>
      </div>
    </aside>

    <!-- Contextual doc-type panel (inside a space) -->
    <aside v-if="inSpace" class="sb-panel">
      <button type="button" class="sb-panel__back" @click="backToSpaces">← Spaces</button>
      <div class="sb-panel__space">
        <span class="sb-panel__spaceavatar">{{ (active?.name || '?').slice(0, 1) }}</span>
        <strong>{{ active?.name }}</strong>
      </div>
      <nav class="sb-panel__nav">
        <NuxtLink v-for="dt in sch?.docTypes || []" :key="dt.type" :to="`/${dt.type}`" class="sb-panel__item">
          {{ dt.type }}
        </NuxtLink>
        <p v-if="error" class="rz-field__hint" style="padding: 0 12px">
          Kan ikke hente schema — kører backend'en?
        </p>
      </nav>
      <div class="sb-panel__foot">
        <NuxtLink to="/settings" class="sb-panel__item">⚙ Indstillinger</NuxtLink>
      </div>
    </aside>

    <main class="sb-main">
      <NuxtPage />
    </main>

    <!-- What's New overlay -->
    <div v-if="showNews" class="sb-news" @click.self="showNews = false">
      <div class="sb-news__panel">
        <button type="button" class="sb-news__close" aria-label="Luk" @click="showNews = false">✕</button>
        <h2>Nyheder</h2>
        <div v-for="n in NEWS" :key="n.title" class="sb-news__item">
          <div class="sb-news__meta">{{ n.date }} · <span class="sb-news__tag">{{ n.tag }}</span></div>
          <h3>{{ n.title }}</h3>
          <p>{{ n.body }}</p>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.sb-shell {
  display: flex;
  height: 100vh;
}
/* Rail */
.sb-rail {
  width: 232px;
  flex: none;
  background: #1a1a2e; /* navy — brand */
  color: rgba(255, 255, 255, 0.82);
  display: flex;
  flex-direction: column;
  padding: 14px 10px;
}
.sb-rail__brand {
  display: flex;
  align-items: center;
  gap: 9px;
  font-weight: 800;
  font-size: 16px;
  color: #fff;
  padding: 6px 10px 18px;
}
.sb-rail__mark {
  width: 26px;
  height: 26px;
  border-radius: 7px;
  background: linear-gradient(135deg, #e94560, #c4a35a);
  display: grid;
  place-items: center;
  color: #1a1a2e;
  font-weight: 800;
  font-size: 14px;
}
.sb-rail__nav {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.sb-rail__item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 9px 10px;
  border-radius: 8px;
  font-size: 13.5px;
  color: rgba(255, 255, 255, 0.72);
  background: none;
  border: 0;
  cursor: pointer;
  font-family: inherit;
  text-align: left;
  width: 100%;
}
.sb-rail__item:hover {
  background: rgba(255, 255, 255, 0.07);
  color: #fff;
}
.sb-rail__item.on {
  background: rgba(255, 255, 255, 0.1);
  color: #fff;
  font-weight: 600;
}
.sb-rail__ic {
  width: 18px;
  text-align: center;
  opacity: 0.85;
}
.sb-rail__badge {
  margin-left: auto;
  background: #e94560;
  color: #fff;
  font-size: 10.5px;
  font-weight: 700;
  border-radius: 999px;
  padding: 1px 7px;
}
.sb-rail__bottom {
  margin-top: auto;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.sb-rail__avatar {
  width: 24px;
  height: 24px;
  border-radius: 999px;
  background: linear-gradient(135deg, #e94560, #c4a35a);
  color: #1a1a2e;
  display: grid;
  place-items: center;
  font-size: 10px;
  font-weight: 800;
}
.sb-rail__account {
  cursor: default;
}
/* Contextual panel */
.sb-panel {
  width: 208px;
  flex: none;
  background: var(--rz-bg-subtle, #f4f2ee);
  border-right: 1px solid var(--rz-border, #e6e2da);
  display: flex;
  flex-direction: column;
  padding: 12px 0;
  overflow: auto;
}
.sb-panel__back {
  background: none;
  border: 0;
  cursor: pointer;
  font-family: var(--rz-font-mono, ui-monospace, monospace);
  font-size: 11px;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--rz-text-muted, #6b6b78);
  text-align: left;
  padding: 2px 14px 10px;
}
.sb-panel__back:hover {
  color: var(--rz-text, #1a1a2e);
}
.sb-panel__space {
  display: flex;
  align-items: center;
  gap: 9px;
  padding: 4px 14px 12px;
  border-bottom: 1px solid var(--rz-border, #e6e2da);
  margin-bottom: 8px;
}
.sb-panel__spaceavatar {
  width: 26px;
  height: 26px;
  border-radius: 7px;
  background: linear-gradient(135deg, #e94560, #c4a35a);
  color: #1a1a2e;
  display: grid;
  place-items: center;
  font-weight: 800;
  font-size: 12px;
  text-transform: uppercase;
}
.sb-panel__nav {
  display: flex;
  flex-direction: column;
}
.sb-panel__item {
  padding: 8px 16px;
  text-transform: capitalize;
  color: var(--rz-text-muted, #6b6b78);
  font-size: 13.5px;
}
.sb-panel__item.router-link-active {
  color: var(--rz-text, #1a1a2e);
  background: var(--rz-surface, #fff);
  border-left: 2px solid var(--rz-primary, #e94560);
  font-weight: 600;
}
.sb-panel__foot {
  margin-top: auto;
  border-top: 1px solid var(--rz-border, #e6e2da);
  padding-top: 8px;
}
.sb-main {
  flex: 1;
  overflow: auto;
  padding: 22px 28px;
}
/* What's New overlay */
.sb-news {
  position: fixed;
  inset: 0;
  background: rgba(26, 26, 46, 0.35);
  display: flex;
  justify-content: center;
  z-index: 50;
}
.sb-news__panel {
  background: var(--rz-raised, #fff);
  color: var(--rz-text, #1a1a2e);
  width: min(560px, 92vw);
  margin: 4vh 0;
  border-radius: 14px;
  padding: 26px 30px;
  overflow: auto;
  position: relative;
  box-shadow: 0 24px 60px rgba(0, 0, 0, 0.3);
}
.sb-news__close {
  position: absolute;
  top: 14px;
  right: 16px;
  background: none;
  border: 0;
  font-size: 16px;
  cursor: pointer;
  color: var(--rz-text-muted, #6b6b78);
}
.sb-news__panel h2 {
  margin: 0 0 18px;
  font-size: 24px;
  font-weight: 800;
}
.sb-news__item {
  border-top: 1px solid var(--rz-border, #e6e2da);
  padding: 14px 0;
}
.sb-news__meta {
  font-size: 12px;
  color: var(--rz-text-muted, #6b6b78);
  font-family: var(--rz-font-mono, ui-monospace, monospace);
}
.sb-news__tag {
  color: var(--rz-primary, #e94560);
  font-weight: 600;
}
.sb-news__item h3 {
  margin: 6px 0 4px;
  font-size: 16px;
}
.sb-news__item p {
  margin: 0;
  font-size: 13.5px;
  line-height: 1.55;
  color: var(--rz-text-muted, #6b6b78);
}
</style>
