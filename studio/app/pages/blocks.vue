<script setup lang="ts">
// Block Library: a catalog of the block registry, read straight from the schema
// API (defined in code, in git), so the page is always in sync with the registry.
const { t } = useT()
const { schema } = useResonans()
const { data: sch } = await useAsyncData('blocks-lib', () => schema())

const q = ref('')
const filtered = computed(() =>
  (sch.value?.blocks || []).filter((b) => b.name.includes(q.value.toLowerCase())),
)
</script>

<template>
  <div style="max-width: 880px">
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 6px">
      <h1 style="margin: 0; font-size: 26px; font-weight: 800">{{ t('Block Library') }}</h1>
      <span class="rz-chip">{{ t('{n} blocks', { n: sch?.blocks.length || 0 }) }}</span>
    </div>
    <p class="rz-field__hint" style="margin: 0 0 16px">
      {{ t('The block library is defined in code (schema in git). Each content type chooses its allowed blocks through its allow list.') }}
    </p>
    <input v-model="q" class="rz-input" :placeholder="t('Search blocks…')" style="margin-bottom: 18px" />

    <div class="bl__grid">
      <div v-for="b in filtered" :key="b.name" class="bl__card">
        <div class="bl__name">{{ b.name }}</div>
        <div class="bl__fields">
          <span v-for="f in b.fields" :key="f.name" class="bl__field">
            {{ f.name }}<em>{{ f.type }}{{ f.i18n ? ' · i18n' : '' }}</em>
          </span>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.bl__grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
  gap: 14px;
}
.bl__card {
  background: var(--rz-raised, #fff);
  border: 1px solid var(--rz-border, #e6e2da);
  border-radius: 10px;
  padding: 14px 15px;
}
.bl__name {
  font-family: var(--rz-font-mono, ui-monospace, monospace);
  font-size: 13px;
  font-weight: 700;
  color: var(--rz-primary, #e94560);
  margin-bottom: 10px;
}
.bl__fields {
  display: flex;
  flex-direction: column;
  gap: 5px;
}
.bl__field {
  font-size: 12.5px;
  display: flex;
  justify-content: space-between;
  gap: 8px;
}
.bl__field em {
  font-style: normal;
  color: var(--rz-text-muted, #6b6b78);
  font-size: 11px;
}
</style>
