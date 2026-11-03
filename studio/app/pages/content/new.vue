<script setup lang="ts">
// "Create new", in two steps: the content TYPE is chosen HERE (step 1, cards from the
// active profile of the space, not in the nav), then title + auto slug (step 2) →
// the editor.
const { t } = useT()
const { schema, createDocument } = useResonans()
const { activeId } = useSpaces()
const cfgPub = useRuntimeConfig().public
// With no space context (no active space and no dev tenant) the API would answer
// 403, so send the user to the Spaces landing page instead.
if (!activeId.value && !(cfgPub.devTenant as string)) {
  navigateTo('/')
}

const { data: sch } = await useAsyncData('content-new-schema', () => schema())

const TYPE_META: Record<string, { label: string; desc: string; ic: string }> = {
  page: { label: t('Page'), desc: t('A standalone page: marketing, about us, front page sections.'), ic: '▢' },
  post: { label: t('Post'), desc: t('A blog post: author, category, date.'), ic: '✎' },
  course: { label: t('Course'), desc: t('A course: a container for modules.'), ic: '🎓' },
  module: { label: t('Module'), desc: t('A chapter or sequence inside a course.'), ic: '▤' },
  lesson: { label: t('Lesson'), desc: t('One lesson: the teaching content itself.'), ic: '▶' },
  quiz: { label: t('Quiz'), desc: t('Questions with answer options, referenced from lessons.'), ic: '?' },
  resource: { label: t('Resource'), desc: t('Download or file material for lessons.'), ic: '⇩' },
}

const step = ref<1 | 2>(1)
const chosen = ref('')
const title = ref('')
const slug = ref('')
const slugTouched = ref(false)
const busy = ref(false)
const err = ref('')

function choose(t: string) {
  chosen.value = t
  step.value = 2
}

function slugify(s: string): string {
  return s
    .toLowerCase()
    // Danish letters transliterate, so a Danish title keeps a readable slug.
    .replaceAll('\u00e6', 'ae')
    .replaceAll('\u00f8', 'oe')
    .replaceAll('\u00e5', 'aa')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    // The API rejects slugs over 200 chars (content.ValidSlug): cut at 200 and
    // drop a trailing hyphen the cut may leave, so a long title still creates.
    .slice(0, 200)
    .replace(/-+$/, '')
}
watch(title, (t) => {
  if (!slugTouched.value) slug.value = slugify(t)
})

async function submit() {
  if (!title.value.trim() || !slug.value.trim() || busy.value) return
  busy.value = true
  err.value = ''
  try {
    const d = await createDocument({ type: chosen.value, slug: slug.value, title: title.value.trim() })
    navigateTo(`/${d.type}/${d.id}`)
  } catch (e) {
    err.value = t('Could not create: {message}', { message: (e as Error).message })
    busy.value = false
  }
}
</script>

<template>
  <div style="max-width: 640px">
    <NuxtLink to="/content" class="cn__back">← {{ t('Back to Content') }}</NuxtLink>
    <h1 style="margin: 10px 0 4px; font-size: 26px; font-weight: 800">{{ t('Create new') }}</h1>

    <!-- Step 1: choose a content type -->
    <template v-if="step === 1">
      <p class="rz-field__hint" style="margin: 0 0 18px">{{ t('Choose which type of content you want to create.') }}</p>
      <div class="cn__grid">
        <button
          v-for="dt in sch?.docTypes || []"
          :key="dt.type"
          type="button"
          class="cn__card"
          @click="choose(dt.type)"
        >
          <span class="cn__ic">{{ TYPE_META[dt.type]?.ic || '▢' }}</span>
          <strong>{{ TYPE_META[dt.type]?.label || dt.type }}</strong>
          <span class="cn__desc">{{ TYPE_META[dt.type]?.desc || t('{n} fields', { n: dt.fields.length }) }}</span>
        </button>
      </div>
    </template>

    <!-- Step 2: title + slug -->
    <template v-else>
      <p class="rz-field__hint" style="margin: 0 0 18px">
        {{ t('New') }} <strong>{{ TYPE_META[chosen]?.label || chosen }}</strong> —
        <button type="button" class="cn__switch" @click="step = 1">{{ t('change type') }}</button>
      </p>
      <label class="rz-field__label" for="cn-title">{{ t('Title') }} <span style="color: var(--rz-danger)">*</span></label>
      <input id="cn-title" v-model="title" class="rz-input" :placeholder="t('My new title')" @keydown.enter="submit" />
      <label class="rz-field__label" for="cn-slug" style="margin-top: 12px">{{ t('Slug') }}</label>
      <input
        id="cn-slug"
        v-model="slug"
        class="rz-input"
        style="font-family: var(--rz-font-mono, monospace)"
        @input="slugTouched = true"
        @keydown.enter="submit"
      />
      <p class="rz-field__hint" style="margin-top: 4px">{{ t('The URL name, generated automatically from the title.') }}</p>
      <button class="rz-btn rz-btn--primary" style="width: 100%; margin-top: 18px; padding: 11px" :disabled="!title.trim() || !slug.trim() || busy" @click="submit">
        {{ busy ? t('Creating…') : t('Create and open editor') }}
      </button>
      <p v-if="err" class="rz-field__hint" style="color: var(--rz-danger); margin-top: 8px">{{ err }}</p>
    </template>
  </div>
</template>

<style scoped>
.cn__back {
  font-family: var(--rz-font-mono, ui-monospace, monospace);
  font-size: 11.5px;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--rz-text-muted, #6b6b78);
}
.cn__grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(190px, 1fr));
  gap: 12px;
}
.cn__card {
  display: flex;
  flex-direction: column;
  gap: 6px;
  align-items: flex-start;
  text-align: left;
  background: var(--rz-raised, #fff);
  border: 1px solid var(--rz-border, #e6e2da);
  border-radius: 10px;
  padding: 14px 15px;
  cursor: pointer;
  font-family: inherit;
  color: inherit;
}
.cn__card:hover {
  border-color: var(--rz-primary, #e94560);
  box-shadow: var(--rz-shadow-sm, 0 1px 2px rgba(26, 26, 46, 0.06));
}
.cn__ic {
  font-size: 18px;
}
.cn__card strong {
  font-size: 14px;
}
.cn__desc {
  font-size: 12px;
  color: var(--rz-text-muted, #6b6b78);
  line-height: 1.45;
}
.cn__switch {
  background: none;
  border: 0;
  color: var(--rz-primary, #e94560);
  cursor: pointer;
  font-family: inherit;
  font-size: inherit;
  padding: 0;
}
.rz-field__label {
  display: block;
  margin-bottom: 5px;
}
</style>
