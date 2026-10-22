<script setup lang="ts">
// Every other Resonans page: a landing page when it opens with a hero (e.g. the
// bunker rental page), otherwise a text page in klanghaus.dk's legal-page layout
// (e.g. the privacy policy).
import type { Envelope } from '~~/server/utils/resonans'
import { str, withHeadingIds } from '~/composables/useSections'

const route = useRoute()
const slug = computed(() => String(route.params.slug))
const { data: page } = await useFetch<Envelope>(() => `/api/pages/${slug.value}`, { key: `page-${slug.value}` })
if (!page.value) throw createError({ statusCode: 404, statusMessage: 'Siden findes ikke', fatal: true })

const isLanding = computed(() => page.value?.components?.[0]?._type === 'hero')
// The delivery envelope carries the page's fields, not its document title, so a
// text page takes its heading from the SEO title: "Privatlivspolitik — KlangHaus".
const seoTitle = computed(() => str(page.value?.data.seoTitle))
const heading = computed(() => seoTitle.value.split(' — ')[0] || slug.value)
useHead({
  title: seoTitle.value || 'KlangHaus',
  meta: [{ name: 'description', content: str(page.value.data.seoDescription) }],
})
</script>

<template>
  <main v-if="isLanding" id="top">
    <PageBlocks :components="page?.components ?? []" />
  </main>
  <main v-else class="kh-legal">
    <p class="kh-sec__eyebrow">klanghaus.dk</p>
    <h1 class="kh-sec__title">{{ heading }}</h1>
    <template v-for="(b, i) in page?.components ?? []" :key="i">
      <div v-if="b._type === 'callout'" class="kh-callout" :class="`kh-callout--${str(b.variant) || 'info'}`" role="note">
        <!-- eslint-disable-next-line vue/no-v-html -- sanitized on save -->
        <div v-html="str(b.value)" />
      </div>
      <!-- eslint-disable-next-line vue/no-v-html -- sanitized on save -->
      <div v-else-if="b._type === 'rich_text'" class="kh-prose" v-html="withHeadingIds(str(b.value))" />
    </template>
  </main>
</template>
