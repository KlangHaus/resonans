<script setup lang="ts">
// One blog post, rendered block by block (PostBlock).
import type { Envelope } from '~~/server/utils/resonans'

type Post = Envelope & { title: string }
import { str } from '~/composables/useSections'

const route = useRoute()
const slug = computed(() => String(route.params.slug))
const { data: post } = await useFetch<Post>(() => `/api/posts/${slug.value}`, { key: `post-${slug.value}` })
if (!post.value) throw createError({ statusCode: 404, statusMessage: 'Indlægget findes ikke', fatal: true })

const title = computed(() => post.value?.title || slug.value)
const meta = computed(() => [str(post.value?.data.author), str(post.value?.data.category)].filter(Boolean).join(' · '))
const date = computed(() =>
  post.value?.publishedAt
    ? new Date(post.value.publishedAt).toLocaleDateString('da-DK', { day: 'numeric', month: 'long', year: 'numeric' })
    : '',
)
useHead({ title: `${title.value} — KlangHaus`, meta: [{ name: 'description', content: str(post.value.data.excerpt) }] })
</script>

<template>
  <main class="kh-legal kh-post">
    <p class="kh-sec__eyebrow"><NuxtLink to="/blog">Blog</NuxtLink> · {{ date }}</p>
    <h1 class="kh-sec__title">{{ title }}</h1>
    <p v-if="meta" class="kh-legal__meta">{{ meta }}</p>
    <p v-if="str(post?.data.excerpt)" class="kh-sec__lead">{{ str(post?.data.excerpt) }}</p>
    <PostBlock v-for="(b, i) in post?.components ?? []" :key="i" :block="b" />
  </main>
</template>
