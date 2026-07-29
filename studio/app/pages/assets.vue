<script setup lang="ts">
// Assets (media library): spacets uploadede billeder + filer. Upload går til
// Resonans' asset-API (Bunny/in-memory backend); alt er tenant-scoped (RLS).
const { listAssets, uploadAsset } = useResonans()

const { data: assets, refresh, pending } = await useAsyncData('assets', () => listAssets())
const busy = ref(false)
const msg = ref('')
const fileInput = ref<HTMLInputElement | null>(null)

async function onPick(e: Event) {
  const file = (e.target as HTMLInputElement).files?.[0]
  if (!file) return
  busy.value = true
  msg.value = ''
  try {
    await uploadAsset(file)
    await refresh()
    msg.value = `Uploadet: ${file.name}`
  } catch (err) {
    msg.value = `Fejl: ${(err as Error).message}`
  } finally {
    busy.value = false
    if (fileInput.value) fileInput.value.value = ''
  }
}

function copyId(id: string) {
  navigator.clipboard?.writeText(id)
  msg.value = 'Asset-id kopieret'
}

function kb(n: number) {
  return n > 1_048_576 ? `${(n / 1_048_576).toFixed(1)} MB` : `${Math.max(1, Math.round(n / 1024))} KB`
}
</script>

<template>
  <div style="max-width: 880px">
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 6px">
      <h1 style="margin: 0; font-size: 26px; font-weight: 800">Assets</h1>
      <div style="display: flex; gap: 10px; align-items: center">
        <span class="rz-field__hint">{{ msg }}</span>
        <button class="rz-btn rz-btn--primary" :disabled="busy" @click="fileInput?.click()">
          {{ busy ? 'Uploader…' : '+ Upload' }}
        </button>
        <input ref="fileInput" type="file" style="display: none" @change="onPick" />
      </div>
    </div>
    <p class="rz-field__hint" style="margin: 0 0 18px">
      Billeder og filer til dette space — refereres fra image/download-blokke via asset-id.
    </p>

    <p v-if="pending" class="rz-field__hint">Henter…</p>
    <p v-else-if="!assets?.length" class="rz-field__hint">Ingen assets endnu — upload det første.</p>

    <div class="as__grid">
      <div v-for="a in assets || []" :key="a.id" class="as__card">
        <div class="as__thumb">
          <img v-if="a.kind === 'image'" :src="a.url" :alt="a.filename" loading="lazy" />
          <span v-else class="as__fileic">▤</span>
        </div>
        <div class="as__meta">
          <div class="as__name" :title="a.filename">{{ a.filename }}</div>
          <div class="as__sub">
            {{ a.kind }} · {{ kb(a.sizeBytes) }}<template v-if="a.width"> · {{ a.width }}×{{ a.height }}</template>
          </div>
          <button type="button" class="as__copy" @click="copyId(a.id)">Kopiér id</button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.as__grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
  gap: 14px;
}
.as__card {
  background: var(--rz-raised, #fff);
  border: 1px solid var(--rz-border, #e6e2da);
  border-radius: 10px;
  overflow: hidden;
}
.as__thumb {
  height: 110px;
  background: var(--rz-surface-2, #ece9e2);
  display: grid;
  place-items: center;
  overflow: hidden;
}
.as__thumb img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.as__fileic {
  font-size: 26px;
  color: var(--rz-text-muted, #6b6b78);
}
.as__meta {
  padding: 10px 12px;
}
.as__name {
  font-size: 12.5px;
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.as__sub {
  font-size: 11px;
  color: var(--rz-text-muted, #6b6b78);
  margin: 2px 0 6px;
}
.as__copy {
  font-size: 11px;
  background: none;
  border: 1px solid var(--rz-border, #e6e2da);
  border-radius: 5px;
  padding: 2px 8px;
  cursor: pointer;
  color: var(--rz-text-muted, #6b6b78);
  font-family: inherit;
}
.as__copy:hover {
  color: var(--rz-primary, #e94560);
  border-color: var(--rz-primary, #e94560);
}
</style>
