<script setup lang="ts">
import type { BlockType, Field } from '~/composables/useResonans'

// SchemaField renders ONE schema field into an input, driven entirely by the
// field definition (type + i18n). This is the core of the schema-driven editor:
// no content type is hardcoded. Scalars + i18n render as inputs; `richtext` uses
// the rich-text editor; `blocks` uses the visual BlockEditor (Fase 4). `array`
// still falls back to a validated JSON editor (next increment). blockDefs is the
// schema block-registry, threaded down so BlockEditor can render nested blocks.
// `path` is the field's location in the document body ("seoTitle",
// "body.2.heading"). Visual editing uses it to jump from a click in the preview
// to this field; it is optional so the field still works outside the editor.
const props = defineProps<{ field: Field; modelValue: unknown; blockDefs?: BlockType[]; path?: string }>()
const emit = defineEmits<{ 'update:modelValue': [unknown] }>()

// Active locales — v1 hardcoded; later resolved from the tenant config.
const LOCALES = ['da', 'en'] as const

// i18n string/text render as per-locale inputs; i18n richtext gets its own
// per-locale rich-text editors (handled separately below).
const isI18nScalar = computed(
  () => !!props.field.i18n && ['string', 'text'].includes(props.field.type),
)
const isI18nRich = computed(() => !!props.field.i18n && props.field.type === 'richtext')

function set(v: unknown) {
  emit('update:modelValue', v)
}

// i18n value is a locale-keyed object; read/write one locale.
function i18nVal(loc: string): string {
  const o = props.modelValue as Record<string, string> | undefined
  return (o && typeof o === 'object' ? o[loc] : '') ?? ''
}
function setI18n(loc: string, v: string) {
  const o = { ...((props.modelValue as Record<string, string>) || {}) }
  o[loc] = v
  set(o)
}

// `array` (non-block, e.g. feature_grid.features) still uses a JSON editor.
const jsonText = ref('')
watchEffect(() => {
  if (props.field.type === 'array') {
    jsonText.value = JSON.stringify(props.modelValue ?? [], null, 2)
  }
})
const jsonError = ref('')
function commitJson() {
  try {
    set(JSON.parse(jsonText.value || '[]'))
    jsonError.value = ''
  } catch (e) {
    jsonError.value = (e as Error).message
  }
}

const scalar = computed(() => String(props.modelValue ?? ''))
</script>

<template>
  <div class="rz-field" :data-field-path="path">
    <label class="rz-field__label">
      {{ field.name }}
      <span v-if="field.required" style="color: var(--rz-danger)">*</span>
      <span class="rz-chip" v-if="field.i18n">i18n</span>
      <span class="rz-chip">{{ field.type }}</span>
    </label>

    <!-- i18n scalar: one input per locale -->
    <template v-if="isI18nScalar">
      <div v-for="loc in LOCALES" :key="loc" style="margin-bottom: 6px">
        <textarea
          v-if="field.type !== 'string'"
          class="rz-textarea"
          :placeholder="loc"
          :value="i18nVal(loc)"
          @input="setI18n(loc, ($event.target as HTMLTextAreaElement).value)"
        />
        <input
          v-else
          class="rz-input"
          :placeholder="loc"
          :value="i18nVal(loc)"
          @input="setI18n(loc, ($event.target as HTMLInputElement).value)"
        />
      </div>
    </template>

    <!-- i18n richtext: one rich-text editor per locale -->
    <template v-else-if="isI18nRich">
      <div v-for="loc in LOCALES" :key="loc" style="margin-bottom: 8px">
        <div class="rz-field__hint" style="margin-bottom: 3px">{{ loc }}</div>
        <RichTextField :model-value="i18nVal(loc)" @update:model-value="(v) => setI18n(loc, v)" />
      </div>
    </template>

    <!-- non-i18n richtext -->
    <RichTextField
      v-else-if="field.type === 'richtext'"
      :model-value="scalar"
      @update:model-value="(v) => set(v)"
    />

    <!-- non-i18n scalars -->
    <input
      v-else-if="field.type === 'string'"
      class="rz-input"
      :value="scalar"
      @input="set(($event.target as HTMLInputElement).value)"
    />
    <textarea
      v-else-if="field.type === 'text'"
      class="rz-textarea"
      :value="scalar"
      @input="set(($event.target as HTMLTextAreaElement).value)"
    />
    <input
      v-else-if="field.type === 'number'"
      class="rz-input"
      type="number"
      :value="scalar"
      @input="set(Number(($event.target as HTMLInputElement).value))"
    />
    <label v-else-if="field.type === 'boolean'" style="display: flex; gap: 8px; align-items: center">
      <input
        type="checkbox"
        :checked="!!modelValue"
        @change="set(($event.target as HTMLInputElement).checked)"
      />
      <span>{{ field.name }}</span>
    </label>
    <input
      v-else-if="field.type === 'asset' || field.type === 'reference'"
      class="rz-input"
      :placeholder="field.type === 'reference' ? `${field.ref} id` : 'asset id'"
      :value="scalar"
      @input="set(($event.target as HTMLInputElement).value)"
    />

    <!-- blocks: visual block editor (Fase 4) -->
    <BlockEditor
      v-else-if="field.type === 'blocks'"
      :model-value="modelValue"
      :allow="field.allow"
      :block-defs="blockDefs || []"
      :path="path"
      @update:model-value="(v) => set(v)"
    />

    <!-- array (non-block): JSON fallback until a repeater UI lands -->
    <template v-else-if="field.type === 'array'">
      <textarea class="rz-textarea" style="min-height: 160px; font-family: ui-monospace, monospace" v-model="jsonText" @blur="commitJson" />
      <p v-if="jsonError" class="rz-field__hint" style="color: var(--rz-danger)">{{ jsonError }}</p>
      <p class="rz-field__hint">Array-felt — repeater-UI sekvenseres som følge-increment.</p>
    </template>

    <p v-else class="rz-field__hint">Ukendt felt-type: {{ field.type }}</p>
  </div>
</template>
