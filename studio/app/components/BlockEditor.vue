<script setup lang="ts">
import type { BlockType, Field } from '~/composables/useResonans'

// BlockEditor — the visual block editor (Fase 4 core). Renders a `blocks` field:
// an ordered, editable list of blocks. Fully schema-driven — each block's fields
// come from the schema block-registry (blockDefs), so NO block type is hardcoded
// here; a new block in the YAML registry just works. Supports add (from the
// doc-type's `allow`-list), reorder, delete, and nesting (a block whose field is
// itself `blocks`, e.g. columns → SchemaField → BlockEditor recursively).
type Block = Record<string, unknown> & { _type: string }

const props = defineProps<{
  modelValue: unknown
  allow?: string[]
  blockDefs: BlockType[]
  path?: string // body path of this blocks field, e.g. "body"
}>()
const emit = defineEmits<{ 'update:modelValue': [Block[]] }>()

const blocks = computed<Block[]>(() =>
  Array.isArray(props.modelValue) ? (props.modelValue as Block[]) : [],
)

function defFor(type: string): BlockType | undefined {
  return props.blockDefs.find((b) => b.name === type)
}

// Only offer blocks this field allows; fall back to the whole registry if the
// field declared no allow-list.
const palette = computed<string[]>(() =>
  props.allow && props.allow.length ? props.allow : props.blockDefs.map((b) => b.name),
)
const showPalette = ref(false)

function commit(next: Block[]) {
  emit('update:modelValue', next)
}

function addBlock(type: string) {
  commit([...blocks.value, { _type: type }])
  showPalette.value = false
}
function removeAt(i: number) {
  const next = [...blocks.value]
  next.splice(i, 1)
  commit(next)
}
function move(i: number, dir: -1 | 1) {
  const j = i + dir
  if (j < 0 || j >= blocks.value.length) return
  const next = [...blocks.value]
  ;[next[i], next[j]] = [next[j] as Block, next[i] as Block]
  commit(next)
}
function setField(i: number, name: string, value: unknown) {
  const next = blocks.value.map((b, idx) => (idx === i ? { ...b, [name]: value } : b))
  commit(next)
}

// Fields to render for a block = its schema def's fields (the `_type` key is the
// discriminator, not an editable field).
function fieldsOf(block: Block): Field[] {
  return defFor(block._type)?.fields ?? []
}
</script>

<template>
  <div class="rz-be">
    <div v-if="!blocks.length" class="rz-be__empty">Ingen blokke endnu.</div>

    <div
      v-for="(block, i) in blocks"
      :key="i"
      class="rz-be__block"
      :data-field-path="path ? `${path}.${i}` : undefined"
    >
      <div class="rz-be__head">
        <span class="rz-be__type">{{ block._type }}</span>
        <span v-if="!defFor(block._type)" class="rz-be__unknown">ukendt blok — ikke i schemaet</span>
        <div class="rz-be__ctrls">
          <button type="button" class="rz-be__ctrl" title="Op" :disabled="i === 0" @click="move(i, -1)">↑</button>
          <button type="button" class="rz-be__ctrl" title="Ned" :disabled="i === blocks.length - 1" @click="move(i, 1)">↓</button>
          <button type="button" class="rz-be__ctrl rz-be__ctrl--del" title="Slet" @click="removeAt(i)">✕</button>
        </div>
      </div>
      <div class="rz-be__body">
        <SchemaField
          v-for="f in fieldsOf(block)"
          :key="f.name"
          :field="f"
          :block-defs="blockDefs"
          :path="path ? `${path}.${i}.${f.name}` : undefined"
          :model-value="block[f.name]"
          @update:model-value="(v) => setField(i, f.name, v)"
        />
      </div>
    </div>

    <div class="rz-be__add">
      <button type="button" class="rz-btn" @click="showPalette = !showPalette">+ Tilføj blok</button>
      <div v-if="showPalette" class="rz-be__palette">
        <button v-for="name in palette" :key="name" type="button" class="rz-be__pill" @click="addBlock(name)">
          {{ name }}
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.rz-be {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.rz-be__empty {
  color: var(--rz-text-muted, #6b6b78);
  font-size: 13px;
  padding: 6px 0;
}
.rz-be__block {
  border: 1px solid var(--rz-border, #e6e2da);
  border-radius: var(--rz-radius-md, 9px);
  background: var(--rz-surface, #f4f2ee);
}
.rz-be__head {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 7px 10px;
  border-bottom: 1px solid var(--rz-border, #e6e2da);
}
.rz-be__type {
  font-family: ui-monospace, monospace;
  font-size: 11.5px;
  font-weight: 600;
  color: var(--rz-primary, #e94560);
  text-transform: uppercase;
  letter-spacing: 0.03em;
}
.rz-be__unknown {
  font-size: 11px;
  color: var(--rz-danger, #dc3545);
}
.rz-be__ctrls {
  margin-left: auto;
  display: flex;
  gap: 3px;
}
.rz-be__ctrl {
  width: 26px;
  height: 26px;
  border: 1px solid var(--rz-border, #e6e2da);
  border-radius: 5px;
  background: var(--rz-raised, #fff);
  cursor: pointer;
  color: var(--rz-text, #1a1a2e);
}
.rz-be__ctrl:disabled {
  opacity: 0.4;
  cursor: default;
}
.rz-be__ctrl--del:hover {
  color: var(--rz-danger, #dc3545);
  border-color: var(--rz-danger, #dc3545);
}
.rz-be__body {
  padding: 10px 12px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.rz-be__add {
  position: relative;
}
.rz-be__palette {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 8px;
  padding: 10px;
  border: 1px dashed var(--rz-border, #e6e2da);
  border-radius: var(--rz-radius-md, 9px);
}
.rz-be__pill {
  font-family: ui-monospace, monospace;
  font-size: 12px;
  padding: 5px 11px;
  border: 1px solid var(--rz-border, #e6e2da);
  border-radius: 999px;
  background: var(--rz-raised, #fff);
  color: var(--rz-text, #1a1a2e);
  cursor: pointer;
}
.rz-be__pill:hover {
  border-color: var(--rz-primary, #e94560);
  color: var(--rz-primary, #e94560);
}
</style>
