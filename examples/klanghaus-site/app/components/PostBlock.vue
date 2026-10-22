<script setup lang="ts">
// The blocks a blog post body uses: rich text, callout, code, a call to action,
// and columns of those.
import { GTButton } from '@grundtone/vue'
import { str, isExternal, type Block } from '~/composables/useSections'

defineProps<{ block: Block }>()
const cells = (b: Block) => (Array.isArray(b.columns) ? (b.columns as Array<{ content?: Block[] }>) : [])
</script>

<template>
  <!-- eslint-disable-next-line vue/no-v-html -- Resonans sanitizes rich text on save -->
  <div v-if="block._type === 'rich_text'" class="kh-prose" v-html="str(block.value)" />
  <div v-else-if="block._type === 'callout'" class="kh-callout" :class="`kh-callout--${str(block.variant) || 'info'}`" role="note">
    <!-- eslint-disable-next-line vue/no-v-html -- sanitized on save -->
    <div v-html="str(block.value)" />
  </div>
  <figure v-else-if="block._type === 'code'" class="kh-code">
    <figcaption v-if="str(block.filename)">{{ str(block.filename) }}</figcaption>
    <pre><code>{{ str(block.value) }}</code></pre>
  </figure>
  <div v-else-if="block._type === 'cta'" class="kh-sec__cta">
    <p v-if="str(block.heading)" class="kh-sec__lead">{{ str(block.heading) }}</p>
    <GTButton
      as="a"
      :href="str(block.href)"
      v-bind="isExternal(str(block.href)) ? { target: '_blank', rel: 'noopener' } : {}"
      variant="primary"
      class="kh-btn"
    >
      {{ str(block.label) }}
    </GTButton>
  </div>
  <div v-else-if="block._type === 'columns'" class="kh-columns">
    <div v-for="(c, i) in cells(block)" :key="i">
      <PostBlock v-for="(inner, j) in c.content ?? []" :key="j" :block="inner" />
    </div>
  </div>
</template>
