<script setup lang="ts">
// Indstillinger — content-profil: hvilke doc-typer denne tenant bruger
// (Allan-beslutning 2026-07-28: undervisning og web skal kunne til-/fravælges).
// Grupperingen er PRÆSENTATION — backend'en er stadig type-agnostisk; typer
// udenfor grupperne (fra fremtidige schemas) lander automatisk i "Andet".
const { getSettings, putSettings } = useResonans()

const GROUPS: { label: string; types: string[] }[] = [
  { label: 'Web (blog + sider)', types: ['post', 'page'] },
  { label: 'Undervisning (LMS)', types: ['course', 'module', 'lesson', 'quiz', 'resource'] },
]

const all = ref<string[]>([])
const checked = ref<Record<string, boolean>>({})
const msg = ref('')
const saving = ref(false)

const { data, refresh } = await useAsyncData('settings', () => getSettings())
watch(
  data,
  (d) => {
    if (!d) return
    all.value = d.allTypes
    const enabled = d.enabledTypes.length ? d.enabledTypes : d.allTypes
    checked.value = Object.fromEntries(d.allTypes.map((t) => [t, enabled.includes(t)]))
  },
  { immediate: true },
)

const grouped = computed(() => {
  const known = new Set(GROUPS.flatMap((g) => g.types))
  const groups = GROUPS.map((g) => ({ ...g, types: g.types.filter((t) => all.value.includes(t)) }))
  const rest = all.value.filter((t) => !known.has(t))
  if (rest.length) groups.push({ label: 'Andet', types: rest })
  return groups
})

function setGroup(types: string[], on: boolean) {
  for (const t of types) checked.value[t] = on
}

async function save() {
  saving.value = true
  msg.value = ''
  try {
    const enabled = all.value.filter((t) => checked.value[t])
    // Alle valgt = gem tom liste ("alt aktivt") så nye schema-typer automatisk
    // er aktive for tenanten fremover.
    await putSettings(enabled.length === all.value.length ? [] : enabled)
    await refresh()
    // Nav'en er schema-drevet — genindlæs så den afspejler profilen.
    await refreshNuxtData('schema')
    msg.value = 'Gemt'
  } catch (e) {
    msg.value = `Fejl: ${(e as Error).message}`
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div style="max-width: 640px">
    <h1 style="margin: 4px 0 6px">Indstillinger</h1>
    <p class="rz-field__hint" style="margin: 0 0 18px">
      Content-profil: vælg hvilke indholdstyper dette space bruger. Fravalgte typer forsvinder fra
      menuen og kan ikke oprettes — eksisterende indhold slettes ikke.
    </p>

    <div v-for="g in grouped" :key="g.label" class="rz-settings__group">
      <div class="rz-settings__head">
        <strong>{{ g.label }}</strong>
        <span style="display: flex; gap: 6px">
          <button class="rz-btn rz-btn--sm" type="button" @click="setGroup(g.types, true)">Alle til</button>
          <button class="rz-btn rz-btn--sm" type="button" @click="setGroup(g.types, false)">Alle fra</button>
        </span>
      </div>
      <label v-for="t in g.types" :key="t" class="rz-settings__row">
        <input v-model="checked[t]" type="checkbox" />
        <span style="text-transform: capitalize">{{ t }}</span>
      </label>
    </div>

    <div style="display: flex; gap: 10px; align-items: center; margin-top: 16px">
      <button class="rz-btn rz-btn--primary" :disabled="saving" @click="save">Gem profil</button>
      <span class="rz-field__hint">{{ msg }}</span>
    </div>
  </div>
</template>

<style scoped>
.rz-settings__group {
  border: 1px solid var(--rz-border);
  border-radius: var(--rz-radius-md, 9px);
  background: var(--rz-surface);
  margin-bottom: 14px;
  overflow: hidden;
}
.rz-settings__head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 10px 14px;
  border-bottom: 1px solid var(--rz-border);
}
.rz-settings__row {
  display: flex;
  gap: 10px;
  align-items: center;
  padding: 8px 14px;
  cursor: pointer;
}
.rz-settings__row:hover {
  background: var(--rz-raised, #fff);
}
.rz-btn--sm {
  font-size: 11.5px;
  padding: 3px 9px;
}
</style>
