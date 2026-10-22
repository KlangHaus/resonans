<script setup lang="ts">
// The front page is the Resonans page with slug "forside".
import type { Envelope } from '~~/server/utils/resonans'

const { data: page, error } = await useFetch<Envelope>('/api/pages/forside', { key: 'page-forside' })
if (error.value || !page.value) {
  throw createError({ statusCode: 404, statusMessage: 'Forsiden findes ikke i Resonans endnu — kør seed (se README).', fatal: true })
}
useHead({
  title: String(page.value.data.seoTitle ?? 'KlangHaus'),
  meta: [{ name: 'description', content: String(page.value.data.seoDescription ?? '') }],
})
</script>

<template>
  <main id="top">
    <PageBlocks :components="page?.components ?? []" />
  </main>
</template>
