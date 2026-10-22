<script setup lang="ts">
import { SITE } from '~/site'

const year = new Date().getFullYear()
</script>

<template>
  <footer class="kh-footer">
    <div class="kh-footer__in">
      <span>© {{ year }} {{ SITE.name }}</span>
      <span>
        <a :href="`mailto:${SITE.mail}`">{{ SITE.mail }}</a> ·
        <NuxtLink to="/privatlivspolitik">Privatlivspolitik</NuxtLink> ·
        <NuxtLink to="/blog">Blog</NuxtLink>
      </span>
      <span class="kh-footer__legal">{{ SITE.legal }}</span>
    </div>
  </footer>
</template>
