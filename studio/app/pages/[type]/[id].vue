<script setup lang="ts">
// Generic schema-driven document editor: fetch the doc + its doc-type schema,
// render one SchemaField per field, save (PUT) / publish. Same editor for every
// content type — the "one editor, many domains" core.
//
// Visual mode (forlæg #4, docs/design/resonans/prototypes/preview-split-screen.html)
// splits the canvas: the form in a 290px editor column, the tenant's frontend in
// draft mode next to it, updated live from the unsaved body. Selection works
// both ways: click an element in the preview → its block/field is selected in
// the editor; click or focus a block/field in the editor → it is selected in
// the preview.
import { bodyToEnvelopePath, envelopeToBodyPath } from '~/utils/visualEditing'

const route = useRoute()
const router = useRouter()
const id = computed(() => route.params.id as string)
const { schema, getDocument, updateDocument, publishDocument, deleteDocument, getPreviewSettings, previewEnvelope } =
  useResonans()

const { data: sch } = await useAsyncData('schema-edit', () => schema())
const { data: doc, refresh } = await useAsyncData(() => `doc-${id.value}`, () => getDocument(id.value), { watch: [id] })

const docType = computed(() => sch.value?.docTypes.find((d) => d.type === doc.value?.type))
const blocksField = computed(() => docType.value?.fields.find((f) => f.type === 'blocks')?.name)
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

// 🔴 SLETNING ER EN PAPIRKURV, IKKE EN UDSLETNING (Allan 20/9). Derfor
// spørger vi én gang og siger hvad der faktisk sker — "kan gendannes" er
// forskellen på en advarsel folk læser og en de klikker forbi.
const deleting = ref(false)
async function remove() {
  if (!doc.value || deleting.value) return
  const ok = confirm(
    `Flyt "${doc.value.title}" til papirkurven?\n\nDen forsvinder fra listen og fra jeres sites med det samme. Du kan gendanne den fra Papirkurv-filteret i Content.`,
  )
  if (!ok) return
  deleting.value = true
  try {
    await deleteDocument(doc.value.id)
    await navigateTo('/content')
  } catch (e: unknown) {
    msg.value = `Kunne ikke slette: ${(e as { message?: string })?.message ?? 'ukendt fejl'}`
  } finally {
    deleting.value = false
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

// --- Visual mode ---
const visual = computed(() => route.query.view === 'visual')
function setView(v: 'form' | 'visual') {
  router.replace({ query: { ...route.query, view: v === 'visual' ? 'visual' : undefined } })
}

const { data: previewSettings } = await useAsyncData('preview-settings', () =>
  getPreviewSettings().catch(() => null),
)
const locale = ref('da')

// Live envelope: rebuilt from the unsaved body, debounced. `seq` drops a slow
// response that arrives after a newer one, so the preview never goes back in time.
const envelope = ref<Record<string, unknown> | null>(null)
const envelopeError = ref('')
let seq = 0
let timer: ReturnType<typeof setTimeout> | undefined
function scheduleEnvelope() {
  if (!visual.value || !doc.value) return
  clearTimeout(timer)
  timer = setTimeout(async () => {
    const mine = ++seq
    try {
      const env = await previewEnvelope({
        type: doc.value!.type,
        slug: doc.value!.slug,
        title: doc.value!.title,
        locale: locale.value,
        body: body.value,
      })
      if (mine === seq) {
        envelope.value = env
        envelopeError.value = ''
      }
    } catch (e) {
      if (mine === seq) envelopeError.value = `Preview kunne ikke opdateres: ${(e as Error).message}`
    }
  }, 250)
}
watch([body, visual, locale], scheduleEnvelope, { deep: true, immediate: true })
onBeforeUnmount(() => clearTimeout(timer))

// Selection, kept as a BODY path ("body.2.heading"). The editor marks the field
// and its block; the preview gets the envelope path.
const editorPane = ref<HTMLElement | null>(null)
const selected = ref<string | null>(null)
const selectedInPreview = computed(() =>
  selected.value ? bodyToEnvelopePath(selected.value, blocksField.value) : null,
)

function fieldEl(path: string) {
  return editorPane.value?.querySelector<HTMLElement>(`[data-field-path="${CSS.escape(path)}"]`) ?? null
}

// Preview click → nearest ancestor that has a field in the editor (a click on a
// block's inner element without its own field lands on the block).
function onPreviewSelect(envPath: string) {
  const bodyPath = envelopeToBodyPath(envPath, blocksField.value)
  if (!bodyPath) return
  const segs = bodyPath.split('.')
  for (let n = segs.length; n > 0; n--) {
    const p = segs.slice(0, n).join('.')
    const el = fieldEl(p)
    if (el) {
      selected.value = p
      el.scrollIntoView({ block: 'center', behavior: 'smooth' })
      el.querySelector<HTMLElement>('input, textarea, [contenteditable="true"]')?.focus({ preventScroll: true })
      return
    }
  }
}

// Editor click or focus → innermost field (or block card) under it.
function onEditorPick(e: Event) {
  const el = (e.target as HTMLElement | null)?.closest<HTMLElement>('[data-field-path]')
  if (el?.dataset.fieldPath) selected.value = el.dataset.fieldPath
}

// Mark the selected field AND the block card that contains it (forlæg:
// .pv__blk--on). Applied to the DOM because fields are rendered recursively.
watch(
  [selected, visual],
  () => {
    if (!editorPane.value) return
    editorPane.value.querySelectorAll('.ve-on, .ve-blk-on').forEach((n) => n.classList.remove('ve-on', 've-blk-on'))
    if (!visual.value || !selected.value) return
    fieldEl(selected.value)?.classList.add('ve-on')
    const segs = selected.value.split('.')
    if (blocksField.value && segs[0] === blocksField.value && segs.length >= 2) {
      fieldEl(segs.slice(0, 2).join('.'))?.classList.add('ve-blk-on')
    }
  },
  { flush: 'post' },
)
</script>

<template>
  <div v-if="doc" :class="{ 've-layout': visual }">
    <div class="ve-head">
      <div>
        <NuxtLink :to="`/${doc.type}`" class="rz-field__hint">← {{ doc.type }}</NuxtLink>
        <h1 style="margin: 4px 0">{{ doc.title }}</h1>
        <span class="rz-chip">{{ doc.status }}</span>
        <span class="rz-chip">{{ doc.slug }}</span>
      </div>
      <div style="display: flex; gap: 8px; align-items: center">
        <div class="ve-toggle" role="group" aria-label="Visning">
          <button type="button" :class="{ on: !visual }" :aria-pressed="!visual" @click="setView('form')">Formular</button>
          <button type="button" :class="{ on: visual }" :aria-pressed="visual" @click="setView('visual')">Visuel</button>
        </div>
        <span class="rz-field__hint">{{ msg }}</span>
        <button class="rz-btn" :disabled="saving" @click="save">Gem</button>
        <button class="rz-btn rz-btn--primary" @click="publish">Udgiv</button>
        <button class="rz-btn rz-btn--danger" :disabled="deleting" title="Flyt til papirkurven" @click="remove">
          {{ deleting ? 'Sletter…' : 'Slet' }}
        </button>
      </div>
    </div>
    <p v-if="visual && envelopeError" class="rz-field__hint" style="color: var(--rz-danger); margin: 0 0 6px">
      {{ envelopeError }}
    </p>

    <template v-if="visual">
      <div class="ve-canvas">
        <VisualPreview
          v-model:locale="locale"
          :settings="previewSettings ?? { previewUrl: '', previewOrigin: '', previewReady: false }"
          :type="doc.type"
          :slug="doc.slug"
          :envelope="envelope"
          :selected="selectedInPreview"
          :doc-fields="docType?.fields || []"
          :block-defs="sch?.blocks || []"
          @select="onPreviewSelect"
          @back="setView('form')"
        >
          <template #editor>
            <div ref="editorPane" class="ve-editor" @click="onEditorPick" @focusin="onEditorPick">
              <SchemaField
                v-for="f in docType?.fields || []"
                :key="f.name"
                :field="f"
                :path="f.name"
                :block-defs="sch?.blocks || []"
                :model-value="body[f.name]"
                @update:model-value="(v: unknown) => (body[f.name] = v)"
              />
            </div>
          </template>
        </VisualPreview>
      </div>
    </template>

    <div v-else style="max-width: 720px; margin-top: 16px">
      <SchemaField
        v-for="f in docType?.fields || []"
        :key="f.name"
        :field="f"
        :path="f.name"
        :block-defs="sch?.blocks || []"
        :model-value="body[f.name]"
        @update:model-value="(v: unknown) => (body[f.name] = v)"
      />
      <p v-if="!docType" class="rz-field__hint">Ukendt doc-type «{{ doc.type }}» — ikke i schemaet.</p>
    </div>
  </div>
  <p v-else class="rz-field__hint">Indlæser…</p>
</template>

<style scoped>
.ve-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
  gap: 12px;
  flex-wrap: wrap;
}
.ve-toggle {
  display: flex;
  border: 1px solid var(--rz-border);
  border-radius: 8px;
  overflow: hidden;
}
.ve-toggle button {
  font-size: 13px;
  padding: 6px 12px;
  border: 0;
  background: var(--rz-bg);
  color: var(--rz-text-muted);
  cursor: pointer;
}
.ve-toggle button.on {
  background: var(--rz-bg-subtle);
  color: var(--rz-text);
  font-weight: 600;
}
.ve-layout {
  display: flex;
  flex-direction: column;
  height: calc(100vh - 48px);
}
.ve-canvas {
  flex: 1;
  min-height: 0;
  margin-top: 8px;
}
/* Forlæg: .pv__blk--on — the selected block card. */
.ve-editor :deep(.ve-blk-on) {
  border-color: var(--rz-primary);
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--rz-primary) 25%, transparent);
}
.ve-editor :deep(.ve-on > .rz-field__label) {
  color: var(--rz-primary);
}
@media (max-width: 900px) {
  .ve-layout {
    height: auto;
  }
  .ve-canvas {
    height: 90vh;
  }
}
</style>

<style scoped>
/* The only destructive action in the editor: it must not look like Gem. */
.rz-btn--danger {
  color: #b3261e;
  border-color: color-mix(in srgb, #b3261e 35%, var(--rz-border));
}
.rz-btn--danger:hover:not(:disabled) {
  background: color-mix(in srgb, #b3261e 8%, transparent);
}
</style>
