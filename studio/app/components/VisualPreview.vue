<script setup lang="ts">
// VisualPreview — the preview pane of visual editing (forlæg:
// docs/design/resonans/prototypes/preview-split-screen.html, #4). It loads the
// tenant's frontend in draft mode (with a short-lived token), pushes the
// envelope of the UNSAVED body into it on every edit, and exchanges the selected
// element both ways. All postMessage traffic is pinned to one origin — the one
// the server derived from the tenant's preview URL — in both directions.
//
// Without a preview URL it shows the FALLBACK preview (utils/fallbackPreview):
// a generic, schema-driven render of the same envelope in a sandboxed srcdoc
// iframe with an opaque origin — so every space gets a preview before its site
// integrates the bridge.
import type { BlockType, Field, PreviewSettings } from '~/composables/useResonans'
import { buildSrcdoc, renderBody } from '~/utils/fallbackPreview'
import { BRIDGE_VERSION, FROM_STUDIO, fillPreviewUrl, parseBridgeMessage } from '~/utils/visualEditing'

const props = defineProps<{
  settings: PreviewSettings
  type: string
  slug: string
  locale: string
  envelope: Record<string, unknown> | null
  // Envelope path of the element selected in the editor (null = none).
  selected: string | null
  // Schema of the document, for the fallback renderer.
  docFields: Field[]
  blockDefs: BlockType[]
}>()
const emit = defineEmits<{ select: [path: string]; back: []; 'update:locale': [locale: string] }>()

const { previewToken } = useResonans()

const frame = ref<HTMLIFrameElement | null>(null)
const src = ref('')
const error = ref('')
const connected = ref(false)
const VIEWPORTS = [
  { id: 'desktop', label: 'Desktop', width: '100%' },
  { id: 'tablet', label: 'Tablet', width: '768px' },
  { id: 'mobile', label: 'Mobil', width: '390px' },
] as const
const LOCALES = ['da', 'en'] as const
const viewport = ref<(typeof VIEWPORTS)[number]['id']>('desktop')
const frameWidth = computed(() => VIEWPORTS.find((v) => v.id === viewport.value)!.width)

// Fallback when the space has no usable preview URL. Also when the URL points at
// the Studio's OWN origin: allow-scripts + allow-same-origin on our own origin
// would let the framed page remove its sandbox ([sikkerhed], resonans#50).
const sameOrigin = computed(
  () => !!props.settings.previewOrigin && props.settings.previewOrigin === window.location.origin,
)
const fallback = computed(() => !props.settings.previewReady || sameOrigin.value)
const srcdoc = ref('')
// An opaque-origin document reports its origin as "null" and can only be
// addressed with targetOrigin '*'; both directions are pinned to the frame's
// window instead (parseBridgeMessage checks e.source).
const peerOrigin = computed(() => (fallback.value ? 'null' : props.settings.previewOrigin))

// Fullscreen: the preview takes the whole canvas; Esc returns to the split view.
const fullscreen = ref(false)
function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape' && fullscreen.value) fullscreen.value = false
}

async function load() {
  error.value = ''
  if (fallback.value) {
    connected.value = false
    src.value = ''
    // The sandboxed document cannot read the Studio's CSS variables, so the
    // theme is passed in (and validated as plain colors inside buildSrcdoc).
    const css = getComputedStyle(document.documentElement)
    const v = (name: string) => css.getPropertyValue(name).trim()
    srcdoc.value = buildSrcdoc(window.location.origin, {
      bg: v('--rz-bg'),
      surface: v('--rz-bg-subtle'),
      border: v('--rz-border'),
      text: v('--rz-text'),
      muted: v('--rz-text-muted'),
      primary: v('--rz-primary'),
    })
    return
  }
  srcdoc.value = ''
  // The bridge announces `ready` from inside the page, which can happen BEFORE
  // the iframe's load event — so "connected" is reset here, when a new src is
  // set, and never on load.
  connected.value = false
  try {
    // A fresh token per (re)load: it is scoped to this (type, slug) and expires
    // in 15 minutes, so it is never reused across documents or long sessions.
    const { token } = await previewToken(props.type, props.slug)
    src.value = fillPreviewUrl(props.settings.previewUrl, {
      type: props.type,
      slug: props.slug,
      token,
      locale: props.locale,
    })
  } catch (e) {
    error.value = `Kunne ikke hente preview-token: ${(e as Error).message}`
  }
}

function send(message: Record<string, unknown>) {
  const win = frame.value?.contentWindow
  if (!win || !connected.value || !peerOrigin.value) return
  const target = fallback.value ? '*' : props.settings.previewOrigin
  win.postMessage({ source: FROM_STUDIO, v: BRIDGE_VERSION, ...message }, target)
}

function pushEnvelope(env: Record<string, unknown> | null) {
  if (!env) return
  if (fallback.value) {
    send({ type: 'render', html: renderBody(toRaw(env), props.docFields, props.blockDefs) })
  } else {
    send({ type: 'update', envelope: toRaw(env) })
  }
}

function onMessage(e: MessageEvent) {
  const msg = parseBridgeMessage(e, peerOrigin.value, frame.value?.contentWindow)
  if (!msg) return
  if (msg.type === 'ready') {
    connected.value = true
    pushEnvelope(props.envelope)
    if (props.selected) send({ type: 'select', path: props.selected })
  } else if (msg.type === 'select') {
    emit('select', msg.path)
  }
}

watch(
  () => props.envelope,
  (env) => pushEnvelope(env),
)
watch(
  () => props.selected,
  (path) => send({ type: 'select', path }),
)
// A new document or preview URL needs a new token; a new locale does not — the
// live envelope carries it — but reloading keeps the server-rendered first
// paint in the same language.
watch(() => [props.type, props.slug, props.settings.previewUrl, props.locale, fallback.value], load)

onMounted(() => {
  window.addEventListener('message', onMessage)
  window.addEventListener('keydown', onKey)
  load()
})
onBeforeUnmount(() => {
  window.removeEventListener('message', onMessage)
  window.removeEventListener('keydown', onKey)
})
</script>

<template>
  <div class="pv">
    <div class="pv__top">
      <button type="button" class="pv__back" @click="emit('back')">‹ Editor</button>
      <span v-if="fallback" class="pv__badge pv__gbadge">⚠ Generisk preview</span>
      <span v-else class="pv__badge">Draft-preview</span>
      <span class="pv__status" :class="{ 'pv__status--on': connected }" :title="settings.previewOrigin">
        {{ connected ? 'Forbundet' : 'Venter på preview…' }}
      </span>
      <span class="pv__sp" />
      <!--
        Not in fallback: the generic cards only stack narrower — they have no
        responsive layout to show, so the selector would promise a simulation the
        fallback cannot deliver ([designer]). The real site keeps it.
      -->
      <span v-if="!fallback" class="pv__seg" role="group" aria-label="Visningsbredde">
        <button
          v-for="v in VIEWPORTS"
          :key="v.id"
          type="button"
          :class="{ on: viewport === v.id }"
          :aria-pressed="viewport === v.id"
          @click="viewport = v.id"
        >
          {{ v.label }}
        </button>
      </span>
      <span class="pv__seg" role="group" aria-label="Sprog">
        <button
          v-for="l in LOCALES"
          :key="l"
          type="button"
          :class="{ on: locale === l }"
          :aria-pressed="locale === l"
          @click="emit('update:locale', l)"
        >
          {{ l.toUpperCase() }}
        </button>
      </span>
      <button
        type="button"
        class="pv__reload"
        :title="fullscreen ? 'Tilbage til split (Esc)' : 'Fuldskærm'"
        :aria-pressed="fullscreen"
        @click="fullscreen = !fullscreen"
      >
        {{ fullscreen ? '⤡' : '⤢' }}
      </button>
      <button type="button" class="pv__reload" title="Genindlæs preview" @click="load">↻</button>
    </div>
    <p v-if="error" class="pv__error">{{ error }}</p>
    <div class="pv__grid" :class="{ 'pv__grid--full': fullscreen }">
      <div v-show="!fullscreen" class="pv__ed">
        <p class="pv__edt">Editor</p>
        <slot name="editor" />
      </div>
      <div class="pv__render" :class="{ 'pv__render--generic': fallback }">
        <!-- Outside the iframe: the sandboxed document may not navigate, the link must. -->
        <div v-if="fallback" class="pv__gbanner">
          ⚠ Generisk preview — I ser strukturen, ikke jeres rigtige site.
          <NuxtLink to="/settings">Tilføj preview-bro →</NuxtLink>
        </div>
        <!--
          sandbox: scripts and same-origin (the frontend needs both to render and
          to read its own cookies) — but NO allow-top-navigation, so the preview
          can never navigate the Studio away.
        -->
        <!--
          Fallback: sandbox="allow-scripts" only — opaque origin, no forms, no
          popups, no navigation of the Studio. Its CSP allows only its own
          nonce'd script, so markup that slipped past DOMPurify cannot run.
        -->
        <iframe
          v-if="fallback && srcdoc"
          ref="frame"
          :srcdoc="srcdoc"
          class="pv__frame pv__frame--generic"
          title="Generisk draft-preview"
          sandbox="allow-scripts"
        />
        <iframe
          v-else-if="src"
          ref="frame"
          :src="src"
          :style="{ width: frameWidth }"
          class="pv__frame"
          title="Live draft-preview"
          referrerpolicy="no-referrer"
          sandbox="allow-scripts allow-same-origin allow-forms allow-popups"
        />
      </div>
    </div>
    <div class="pv__foot">
      Live draft-render. <b>Klik en blok → fremhæv i preview · klik i preview → vælg blok i editor</b>
    </div>
  </div>
</template>

<style scoped>
/* Measurements from the forlæg (#4); colors are the Studio theme tokens. */
.pv {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
  border: 1px solid var(--rz-border);
  border-radius: 12px;
  overflow: hidden;
  background: var(--rz-bg-subtle);
}
.pv__grid {
  flex: 1;
  min-height: 0;
  display: grid;
  grid-template-columns: 290px 1fr;
}
.pv__grid--full {
  grid-template-columns: 1fr;
}
/* Fallback (forlæg #4, "Fallback" section): generic badge + banner. */
.pv__gbadge {
  background: color-mix(in srgb, #ffc107 30%, transparent);
  color: #7a5b00;
}
.pv__render--generic {
  flex-direction: column;
  align-items: center;
}
.pv__gbanner {
  display: flex;
  align-items: center;
  gap: 8px;
  align-self: stretch;
  padding: 9px 12px;
  border-radius: 8px;
  background: color-mix(in srgb, #ffc107 16%, transparent);
  border: 1px solid color-mix(in srgb, #ffc107 40%, transparent);
  font-size: 11.5px;
  color: #7a5b00;
  margin-bottom: 12px;
}
.pv__gbanner a {
  color: inherit;
  font-weight: 600;
  text-decoration: underline;
  margin-left: auto;
  white-space: nowrap;
}
.pv__ed {
  border-right: 1px solid var(--rz-border);
  background: color-mix(in srgb, var(--rz-bg-subtle) 97%, var(--rz-text));
  padding: 12px;
  overflow: auto;
  min-height: 0;
}
.pv__edt {
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--rz-text-muted);
  margin: 0 0 8px;
}
@media (max-width: 900px) {
  .pv__grid {
    grid-template-columns: 1fr;
    grid-template-rows: auto 70vh;
  }
  .pv__ed {
    border-right: 0;
    border-bottom: 1px solid var(--rz-border);
  }
}
.pv__top {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  border-bottom: 1px solid var(--rz-border);
  background: var(--rz-bg-subtle);
  font-size: 12px;
}
.pv__back {
  border: 0;
  background: none;
  padding: 0;
  font: inherit;
  color: var(--rz-text-muted);
  cursor: pointer;
}
.pv__badge {
  padding: 2px 8px;
  border-radius: 999px;
  font-size: 11px;
  font-weight: 500;
  background: color-mix(in srgb, #ffc107 22%, transparent);
  color: #8a6d00;
}
.pv__status {
  font-size: 11px;
  color: var(--rz-text-muted);
}
.pv__status--on {
  color: #198754;
}
.pv__sp {
  flex: 1;
}
.pv__seg {
  display: inline-flex;
  border: 1px solid var(--rz-border);
  border-radius: 7px;
  overflow: hidden;
}
.pv__seg button {
  padding: 4px 10px;
  border: none;
  background: var(--rz-bg);
  color: var(--rz-text-muted);
  font: inherit;
  font-size: 11px;
  font-weight: 500;
  cursor: pointer;
}
.pv__seg button.on {
  background: var(--rz-primary);
  color: var(--rz-primary-contrast);
}
.pv__reload {
  border: 1px solid var(--rz-border);
  border-radius: 7px;
  background: var(--rz-bg);
  color: var(--rz-text-muted);
  font-size: 12px;
  padding: 2px 8px;
  cursor: pointer;
}
.pv__error {
  margin: 0;
  padding: 8px 12px;
  font-size: 12px;
  color: var(--rz-danger);
}
.pv__render {
  min-height: 0;
  padding: 14px;
  display: flex;
  justify-content: center;
  overflow: auto;
}
.pv__frame {
  max-width: 100%;
  height: 100%;
  border: 1px solid var(--rz-border);
  border-radius: 8px;
  background: var(--rz-bg);
  transition: width 120ms ease-out;
}
.pv__frame--generic {
  flex: 1;
  min-height: 0;
}
.pv__foot {
  padding: 9px 12px;
  border-top: 1px solid var(--rz-border);
  background: var(--rz-bg-subtle);
  font-size: 11px;
  color: var(--rz-text-muted);
}
.pv__foot b {
  color: var(--rz-text);
  font-weight: 500;
}
</style>
