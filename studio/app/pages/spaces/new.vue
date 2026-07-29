<script setup lang="ts">
// "Create your Space" (Storyblok-mønstret). Et space = en tenant: oprettelsen
// minter en frisk UUID (Beslutning K: klient-leveret tenant-id) — RLS isolerer
// indholdet automatisk, så intet backend-kald er nødvendigt for selve
// oprettelsen. I prod-Studio bliver dette en auth-org-oprettelse i stedet.
const { create, enter } = useSpaces()

const name = ref('')
const busy = ref(false)

async function submit() {
  if (!name.value.trim() || busy.value) return
  busy.value = true
  const s = create(name.value)
  enter(s.id)
  navigateTo('/dashboard')
}
</script>

<template>
  <div class="ns">
    <NuxtLink to="/" class="ns__back">← Tilbage til Spaces</NuxtLink>
    <div class="ns__inner">
      <h1>Opret dit Space</h1>
      <p class="ns__sub">
        Et space er hvor du styrer alt indhold, komponenter og konfiguration for ét projekt —
        fx en blog, et kursus-katalog eller en hjemmeside.
      </p>

      <div class="ns__illustration">
        <div class="ns__card">
          <div class="ns__cardhead">Space <span>⋯</span></div>
          <div class="ns__avatars">
            <span v-for="i in 4" :key="i" class="ns__av" :style="{ opacity: 1 - i * 0.15 }" />
            <span class="ns__avmore">+3</span>
          </div>
          <div class="ns__bars"><span /><span /></div>
        </div>
      </div>

      <label class="rz-field__label" for="space-name">Space-navn <span style="color: var(--rz-danger)">*</span></label>
      <input
        id="space-name"
        v-model="name"
        class="rz-input"
        placeholder="Mit nye space"
        @keydown.enter="submit"
      />

      <button class="rz-btn rz-btn--primary ns__cta" :disabled="!name.trim() || busy" @click="submit">
        Opret Space
      </button>
      <p class="rz-field__hint" style="margin-top: 10px">
        Spacet får sin egen isolerede content-tenant (RLS) og starter med alle indholdstyper aktive —
        justér profilen under ⚙ Indstillinger.
      </p>
    </div>
  </div>
</template>

<style scoped>
.ns__back {
  font-family: var(--rz-font-mono, ui-monospace, monospace);
  font-size: 11.5px;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--rz-text-muted, #6b6b78);
}
.ns__inner {
  max-width: 480px;
  margin: 5vh auto 0;
  text-align: center;
}
.ns__inner h1 {
  font-size: 28px;
  font-weight: 800;
  margin: 0 0 8px;
}
.ns__sub {
  color: var(--rz-text-muted, #6b6b78);
  font-size: 14px;
  line-height: 1.55;
  margin: 0 0 26px;
}
.ns__illustration {
  background: linear-gradient(135deg, color-mix(in srgb, #e94560 18%, var(--rz-surface, #f4f2ee)), color-mix(in srgb, #c4a35a 22%, var(--rz-surface, #f4f2ee)));
  border-radius: 14px;
  padding: 26px;
  margin-bottom: 26px;
  display: flex;
  justify-content: center;
}
.ns__card {
  background: var(--rz-raised, #fff);
  border-radius: 10px;
  box-shadow: var(--rz-shadow-md, 0 8px 26px rgba(26, 26, 46, 0.12));
  padding: 14px 16px;
  width: 240px;
  text-align: left;
}
.ns__cardhead {
  display: flex;
  justify-content: space-between;
  font-weight: 700;
  font-size: 14px;
  margin-bottom: 12px;
}
.ns__avatars {
  display: flex;
  align-items: center;
  margin-bottom: 12px;
}
.ns__av {
  width: 22px;
  height: 22px;
  border-radius: 999px;
  background: linear-gradient(135deg, #e94560, #c4a35a);
  border: 2px solid var(--rz-raised, #fff);
  margin-right: -6px;
}
.ns__avmore {
  font-size: 10px;
  color: var(--rz-text-muted, #6b6b78);
  margin-left: 10px;
}
.ns__bars {
  display: flex;
  gap: 8px;
}
.ns__bars span {
  height: 8px;
  border-radius: 999px;
  background: var(--rz-surface-2, #ece9e2);
  flex: 1;
}
.rz-field__label {
  display: block;
  text-align: left;
  margin-bottom: 6px;
}
.ns__cta {
  width: 100%;
  margin-top: 14px;
  padding: 11px;
}
</style>
