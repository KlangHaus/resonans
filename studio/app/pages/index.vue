<script setup lang="ts">
// Landing: send the editor to the first doc-type's list.
const { schema } = useResonans()
const { data: sch } = await useAsyncData('schema-index', () => schema())
const first = computed(() => sch.value?.docTypes?.[0]?.type)
watchEffect(() => {
  if (first.value) navigateTo(`/${first.value}`)
})
</script>

<template>
  <div>
    <h1>Resonans Studio</h1>
    <p class="rz-field__hint">Vælg en content-type i menuen.</p>
  </div>
</template>
