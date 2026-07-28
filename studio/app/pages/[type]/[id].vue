<script setup lang="ts">
// Generic schema-driven document editor: fetch the doc + its doc-type schema,
// render one SchemaField per field, save (PUT) / publish. Same editor for every
// content type — the "one editor, many domains" core.
const route = useRoute()
const id = computed(() => route.params.id as string)
const { schema, getDocument, updateDocument, publishDocument } = useResonans()

const { data: sch } = await useAsyncData('schema-edit', () => schema())
const { data: doc, refresh } = await useAsyncData(() => `doc-${id.value}`, () => getDocument(id.value), { watch: [id] })

const docType = computed(() => sch.value?.docTypes.find((d) => d.type === doc.value?.type))
const body = ref<Record<string, unknown>>({})
watch(doc, (d) => { body.value = d ? { ...d.body } : {} }, { immediate: true })

const saving = ref(false)
const msg = ref('')

async function save() {
  if (!doc.value) return
  saving.value = true
  msg.value = ''
  try {
    await updateDocument(doc.value.id, { body: body.value })
    msg.value = 'Gemt'
    await refresh()
  } catch (e) {
    msg.value = `Fejl: ${(e as Error).message}`
  } finally {
    saving.value = false
  }
}

async function publish() {
  if (!doc.value) return
  try {
    await updateDocument(doc.value.id, { body: body.value })
    await publishDocument(doc.value.id)
    await refresh()
    msg.value = 'Udgivet'
  } catch (e) {
    msg.value = `Fejl: ${(e as Error).message}`
  }
}
</script>

<template>
  <div v-if="doc">
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px">
      <div>
        <NuxtLink :to="`/${doc.type}`" class="rz-field__hint">← {{ doc.type }}</NuxtLink>
        <h1 style="margin: 4px 0">{{ doc.title }}</h1>
        <span class="rz-chip">{{ doc.status }}</span>
        <span class="rz-chip">{{ doc.slug }}</span>
      </div>
      <div style="display: flex; gap: 8px; align-items: center">
        <span class="rz-field__hint">{{ msg }}</span>
        <button class="rz-btn" :disabled="saving" @click="save">Gem</button>
        <button class="rz-btn rz-btn--primary" @click="publish">Udgiv</button>
      </div>
    </div>

    <div style="max-width: 720px; margin-top: 16px">
      <SchemaField
        v-for="f in docType?.fields || []"
        :key="f.name"
        :field="f"
        :block-defs="sch?.blocks || []"
        :model-value="body[f.name]"
        @update:model-value="(v) => (body[f.name] = v)"
      />
      <p v-if="!docType" class="rz-field__hint">Ukendt doc-type «{{ doc.type }}» — ikke i schemaet.</p>
    </div>
  </div>
  <p v-else class="rz-field__hint">Indlæser…</p>
</template>
