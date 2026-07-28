<script setup lang="ts">
// RichTextField — INTERIM rich-text editor (bold/italic/H2/H3/lists/link),
// HTML output. Placeholder for grundtone's GTRichText organism (grundtone PR
// #96: ProseMirror-JSON v-model + HTML-getter + SSR-static render). When that
// publishes we swap this component for <GTRichText> and store portable-text
// JSON instead of HTML — the delivery contract already carries `format` so the
// swap is renderer-local. Until then: a constrained contenteditable that emits
// the same limited HTML the blog renderer expects (§F rich_text format:"html").
//
// Client-only (contenteditable needs the DOM); SSR renders a static fallback.
const props = defineProps<{ modelValue: string; placeholder?: string }>()
const emit = defineEmits<{ 'update:modelValue': [string] }>()

const el = ref<HTMLElement | null>(null)

// Sync prop → DOM only when it diverges AND the field isn't being edited, so we
// never clobber the caret mid-type.
watch(
  () => props.modelValue,
  (v) => {
    if (el.value && document.activeElement !== el.value && el.value.innerHTML !== (v || '')) {
      el.value.innerHTML = v || ''
    }
  },
)
onMounted(() => {
  if (el.value) el.value.innerHTML = props.modelValue || ''
})

function emitHtml() {
  if (el.value) emit('update:modelValue', el.value.innerHTML)
}

// Constrained command set — mirrors the blog renderer's allow-list so authors
// can't produce blocks the delivery/render path won't honor.
function cmd(command: string, value?: string) {
  el.value?.focus()
  document.execCommand(command, false, value)
  emitHtml()
}
function block(tag: string) {
  cmd('formatBlock', tag)
}
function link() {
  const url = window.prompt('Link-URL:')
  if (url) cmd('createLink', url)
}

const tools = [
  { label: 'B', title: 'Fed', style: 'font-weight:700', act: () => cmd('bold') },
  { label: 'I', title: 'Kursiv', style: 'font-style:italic', act: () => cmd('italic') },
  { label: 'H2', title: 'Overskrift 2', style: '', act: () => block('h2') },
  { label: 'H3', title: 'Overskrift 3', style: '', act: () => block('h3') },
  { label: '¶', title: 'Afsnit', style: '', act: () => block('p') },
  { label: '“ ”', title: 'Citat', style: '', act: () => block('blockquote') },
  { label: '• Liste', title: 'Punktliste', style: '', act: () => cmd('insertUnorderedList') },
  { label: '1. Liste', title: 'Nummereret', style: '', act: () => cmd('insertOrderedList') },
  { label: '🔗', title: 'Link', style: '', act: link },
  { label: '⌫', title: 'Ryd format', style: '', act: () => cmd('removeFormat') },
]
</script>

<template>
  <div class="rz-rte">
    <div class="rz-rte__bar">
      <button
        v-for="t in tools"
        :key="t.label"
        type="button"
        class="rz-rte__btn"
        :title="t.title"
        :style="t.style"
        @mousedown.prevent="t.act()"
      >
        {{ t.label }}
      </button>
      <span class="rz-rte__tag">interim · GTRichText swap pending</span>
    </div>
    <ClientOnly>
      <div
        ref="el"
        class="rz-rte__area"
        contenteditable="true"
        :data-placeholder="placeholder || 'Skriv…'"
        @input="emitHtml"
        @blur="emitHtml"
      />
      <template #fallback>
        <div class="rz-rte__area" v-html="modelValue || ''" />
      </template>
    </ClientOnly>
  </div>
</template>

<style scoped>
.rz-rte {
  border: 1px solid var(--rz-border, #e6e2da);
  border-radius: var(--rz-radius-md, 9px);
  overflow: hidden;
  background: var(--rz-raised, #fff);
}
.rz-rte__bar {
  display: flex;
  flex-wrap: wrap;
  gap: 2px;
  align-items: center;
  padding: 5px 6px;
  border-bottom: 1px solid var(--rz-border, #e6e2da);
  background: var(--rz-surface, #f4f2ee);
}
.rz-rte__btn {
  min-width: 26px;
  height: 26px;
  padding: 0 7px;
  border: 1px solid transparent;
  border-radius: 5px;
  background: transparent;
  color: var(--rz-text, #1a1a2e);
  font-size: 12.5px;
  cursor: pointer;
}
.rz-rte__btn:hover {
  background: var(--rz-surface-2, #ece9e2);
  border-color: var(--rz-border, #e6e2da);
}
.rz-rte__tag {
  margin-left: auto;
  font-size: 10px;
  color: var(--rz-text-muted, #6b6b78);
  font-family: ui-monospace, monospace;
}
.rz-rte__area {
  min-height: 120px;
  padding: 12px 14px;
  font-size: 15px;
  line-height: 1.6;
  color: var(--rz-text, #1a1a2e);
  outline: none;
}
.rz-rte__area:empty::before {
  content: attr(data-placeholder);
  color: var(--rz-text-muted, #9a9ab0);
}
.rz-rte__area :deep(h2) {
  font-size: 20px;
  font-weight: 700;
  margin: 12px 0 6px;
}
.rz-rte__area :deep(h3) {
  font-size: 17px;
  font-weight: 700;
  margin: 10px 0 5px;
}
.rz-rte__area :deep(blockquote) {
  border-left: 3px solid var(--rz-primary, #e94560);
  margin: 10px 0;
  padding-left: 12px;
  color: var(--rz-text-muted, #6b6b78);
}
.rz-rte__area :deep(a) {
  color: var(--rz-primary, #e94560);
}
</style>
