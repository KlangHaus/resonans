<script setup lang="ts">
// The navigation is the front page's own sections: every section_header with an
// anchor becomes a link, labelled by its navLabel (or else its eyebrow). Add a section in Resonans and
// it appears here. On other pages the links point back to the front page.
import { GTButton } from '@grundtone/vue'
import { SITE } from '~/site'
import { str, type Block } from '~/composables/useSections'
import type { Envelope } from '~~/server/utils/resonans'

const route = useRoute()
const onHome = computed(() => route.path === '/')
const { data: home } = await useFetch<Envelope>('/api/pages/forside', { key: 'page-forside' })
// While the front page is open in the editor's preview, its draft drives the menu.
const draftHome = useState<Block[] | null>('preview-home', () => null)
const homeBlocks = computed<Block[]>(() => draftHome.value ?? ((home.value?.components ?? []) as Block[]))

const links = computed(() =>
  homeBlocks.value
    .map((c) => ({ c, label: str(c.navLabel) || str(c.eyebrow) }))
    .filter(({ c, label }) => c._type === 'section_header' && str(c.anchor) && label)
    .map(({ c, label }) => ({ href: `${onHome.value ? '' : '/'}#${str(c.anchor)}`, label })),
)
const hero = computed(() => homeBlocks.value.find((c) => c._type === 'hero'))
// The header button repeats the hero's main call to action, without its arrow.
const cta = computed(() => ({
  label: str(hero.value?.ctaLabel).replace(/\s*[→↗]\s*$/, ''),
  href: `${onHome.value ? '' : '/'}${str(hero.value?.ctaHref)}`,
}))
</script>

<template>
  <header class="kh-header">
    <div class="kh-header__in">
      <a class="kh-header__brand" :href="onHome ? '#top' : '/'">
        <img :src="SITE.logo" :alt="SITE.name" width="34" height="34" />
        {{ SITE.name }}
      </a>
      <nav class="kh-header__nav" aria-label="Primær">
        <a v-for="l in links" :key="l.href" :href="l.href">{{ l.label }}</a>
        <NuxtLink to="/blog">Blog</NuxtLink>
      </nav>
      <GTButton v-if="cta.label" as="a" :href="cta.href" variant="primary" size="sm" class="kh-header__cta">
        {{ cta.label }}
      </GTButton>
    </div>
  </header>
</template>
