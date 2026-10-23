<script setup lang="ts">
// The editor's preview: Resonans' visual editing (examples/visual-editing in the
// Resonans repo is the reference). The editor frames this route with a
// short-lived token, so the first paint is the stored DRAFT, rendered here on
// the server. Then the bridge takes over: every keystroke in the editor arrives
// as a new envelope and the page re-renders from it — unsaved, no reload. The
// renderer is the same one the public pages use; only this route marks values
// with their paths (provide PREVIEW_KEY) and loads the bridge.
import type { Envelope } from '~~/server/utils/resonans'
import { PREVIEW_KEY, str, withHeadingIds, type Block } from '~/composables/useSections'

const route = useRoute()
const type = computed(() => String(route.params.type))
const slug = computed(() => String(route.params.slug))
const cfg = useRuntimeConfig().public

const { data, error } = await useFetch<Envelope>(() => `/api/preview/${type.value}/${slug.value}`, {
  query: { token: route.query.token, locale: route.query.locale },
  key: `preview-${type.value}-${slug.value}`,
})
if (error.value || !data.value) {
  throw createError({ statusCode: error.value?.statusCode ?? 404, statusMessage: 'Forhåndsvisningen kunne ikke hentes — tokenet mangler, er forkert eller udløbet.', fatal: true })
}

const envelope = shallowRef<Envelope>(data.value)
provide(PREVIEW_KEY, true)
// The front page's menu is built from its section headers; while the front page
// itself is being previewed, it should follow the draft, not the published page.
const draftHome = useState<Block[] | null>('preview-home', () => null)
watchEffect(() => {
  draftHome.value = type.value === 'page' && slug.value === 'forside' ? (envelope.value.components ?? []) : null
})
onBeforeUnmount(() => (draftHome.value = null))

// Live updates arrive already sanitized: Resonans cleans the unsaved body with
// the same server-side policy it applies on save (POST /api/v1/preview/envelope).
// This site runs no sanitizer of its own.
onMounted(() => {
  const base = String(cfg.resonansPublicUrl).replace(/\/$/, '')
  const script = document.createElement('script')
  script.src = `${base}/bridge/v1/resonans-bridge.js`
  script.onload = () => {
    const bridge = (window as unknown as { ResonansBridge?: { init: (o: unknown) => void } }).ResonansBridge
    bridge?.init({
      studioOrigin: String(cfg.studioOrigin),
      onUpdate: (next: Envelope) => {
        envelope.value = next
      },
      accent: '#c41e3a',
    })
  }
  document.head.appendChild(script)
})

const components = computed(() => envelope.value.components ?? [])
const isLanding = computed(() => type.value === 'page' && components.value[0]?._type === 'hero')
const heading = computed(() => str(envelope.value.data.seoTitle).split(' — ')[0] || slug.value)
useHead({ title: `Forhåndsvisning — ${heading.value}`, meta: [{ name: 'robots', content: 'noindex, nofollow' }] })
</script>

<template>
  <main v-if="isLanding" id="top">
    <PageBlocks :components="components" />
  </main>
  <main v-else class="kh-legal" :class="{ 'kh-post': type === 'post' }">
    <p class="kh-sec__eyebrow">Forhåndsvisning · {{ type }}</p>
    <h1 class="kh-sec__title" data-resonans-path="data.seoTitle">{{ heading }}</h1>
    <p v-if="str(envelope.data.excerpt)" class="kh-sec__lead" data-resonans-path="data.excerpt">{{ str(envelope.data.excerpt) }}</p>
    <template v-for="(b, i) in components" :key="i">
      <!-- eslint-disable-next-line vue/no-v-html -- Resonans sanitizes rich text on save and in the live envelope -->
      <div v-if="type === 'page' && b._type === 'rich_text'" class="kh-prose" :data-resonans-path="`components.${i}.value`" v-html="withHeadingIds(str(b.value))" />
      <PostBlock v-else :block="b" :index="i" />
    </template>
  </main>
</template>
