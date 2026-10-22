<script setup lang="ts">
// The blog: every published `post` in Resonans, newest first.
import type { PostSummary } from '~~/server/utils/resonans'

const { data: posts } = await useFetch<PostSummary[]>('/api/posts', { key: 'posts' })
useHead({ title: 'Blog — KlangHaus' })

const date = (iso?: string) =>
  iso ? new Date(iso).toLocaleDateString('da-DK', { day: 'numeric', month: 'long', year: 'numeric' }) : ''
</script>

<template>
  <main class="kh-sec kh-blog">
    <p class="kh-sec__eyebrow">Blog</p>
    <h1 class="kh-sec__title">Fra huset</h1>
    <p v-if="!posts?.length" class="kh-sec__lead">Der er ingen indlæg endnu.</p>
    <ul v-else class="kh-blog__list">
      <li v-for="p in posts" :key="p.id" class="kh-blog__item">
        <NuxtLink :to="`/blog/${p.slug}`" class="kh-card kh-blog__card">
          <img v-if="p.thumbnail?.url" :src="p.thumbnail.url" :alt="p.thumbnail.alt ?? ''" class="kh-blog__thumb" loading="lazy" />
          <p class="kh-blog__date">{{ date(p.publishedAt) }}</p>
          <h2 class="kh-card__title">{{ p.title }}</h2>
          <p v-if="p.summary" class="kh-card__text">{{ p.summary }}</p>
        </NuxtLink>
      </li>
    </ul>
  </main>
</template>
