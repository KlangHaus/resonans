<script setup lang="ts">
// Min organisation (Storyblok-mønstret): org-overblik + spaces-administration —
// inkl. space-oprettelse (Allan-krav: "i org skal man kunne kreere new spaces").
// I prod-Studio kommer org-data fra auth (BetterAuth-org); dette er dev-formen.
const { spaces, enter, create } = useSpaces()

const newName = ref('')
const busy = ref(false)

async function createSpace() {
  if (!newName.value.trim() || busy.value) return
  busy.value = true
  const s = create(newName.value)
  newName.value = ''
  busy.value = false
  // Bliv på org-siden — spacet dukker op i listen; klik åbner det.
  return s
}

async function open(id: string) {
  enter(id)
  navigateTo('/dashboard')
}
</script>

<template>
  <div style="max-width: 720px">
    <h1 style="margin: 0 0 4px; font-size: 26px; font-weight: 800">Min organisation</h1>
    <p class="rz-field__hint" style="margin: 0 0 22px">
      KlangHaus · EU · {{ spaces.length }} space{{ spaces.length === 1 ? '' : 's' }}
    </p>

    <div class="org__create">
      <strong style="display: block; margin-bottom: 8px">Opret nyt space</strong>
      <div style="display: flex; gap: 10px">
        <input
          v-model="newName"
          class="rz-input"
          placeholder="Space-navn (fx Marketing, Explainers, Podcast…)"
          style="flex: 1"
          @keydown.enter="createSpace"
        />
        <button class="rz-btn rz-btn--primary" :disabled="!newName.trim() || busy" @click="createSpace">
          Opret
        </button>
      </div>
      <p class="rz-field__hint" style="margin-top: 8px">
        Hvert space er sin egen isolerede content-tenant. Vil du hellere have den guidede oprettelse:
        <NuxtLink to="/spaces/new" style="color: var(--rz-primary)">Opret dit Space →</NuxtLink>
      </p>
    </div>

    <div class="org__list">
      <div class="rz-field__label" style="margin-bottom: 8px">Organisationens spaces</div>
      <button v-for="s in spaces" :key="s.id" type="button" class="org__row" @click="open(s.id)">
        <span class="org__avatar">{{ s.name.slice(0, 1).toUpperCase() }}</span>
        <span class="org__name">{{ s.name }}</span>
        <span class="org__id">{{ s.id.slice(0, 8) }}…</span>
        <span class="rz-chip">Staging</span>
        <span class="org__go">Åbn →</span>
      </button>
    </div>
  </div>
</template>

<style scoped>
.org__create {
  background: var(--rz-surface, #f4f2ee);
  border: 1px solid var(--rz-border, #e6e2da);
  border-radius: 12px;
  padding: 16px 18px;
  margin-bottom: 24px;
}
.org__list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.org__row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 14px;
  background: var(--rz-raised, #fff);
  border: 1px solid var(--rz-border, #e6e2da);
  border-radius: 10px;
  cursor: pointer;
  font-family: inherit;
  color: inherit;
  font-size: 14px;
  text-align: left;
}
.org__row:hover {
  box-shadow: var(--rz-shadow-sm, 0 1px 2px rgba(26, 26, 46, 0.06));
  border-color: var(--rz-primary, #e94560);
}
.org__avatar {
  width: 28px;
  height: 28px;
  border-radius: 8px;
  background: linear-gradient(135deg, #e94560, #c4a35a);
  color: #1a1a2e;
  display: grid;
  place-items: center;
  font-weight: 800;
  font-size: 13px;
}
.org__name {
  font-weight: 600;
  flex: 1;
}
.org__id {
  font-family: var(--rz-font-mono, ui-monospace, monospace);
  font-size: 11px;
  color: var(--rz-text-muted, #6b6b78);
}
.org__go {
  color: var(--rz-primary, #e94560);
  font-size: 13px;
  font-weight: 600;
}
</style>
