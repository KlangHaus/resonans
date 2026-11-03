<script setup lang="ts">
// Datasources, a planned feature: shared key/value lists (categories, authors, tags)
// that fields can reference across documents. Not built yet; this placeholder keeps
// the navigation honest about the roadmap.
const { t } = useT()
</script>

<template>
  <div style="max-width: 620px">
    <h1 style="margin: 0 0 6px; font-size: 26px; font-weight: 800">{{ t('Datasources') }}</h1>
    <p class="rz-field__hint" style="margin: 0 0 20px">
      {{ t('Shared key/value lists to reuse across content.') }}
    </p>
    <div class="ds__placeholder">
      <div style="font-size: 30px; margin-bottom: 10px">🗂</div>
      <strong>{{ t('On the roadmap') }}</strong>
      <p class="rz-field__hint" style="max-width: 42ch; margin: 6px auto 0">
        {{ t('Datasources (such as categories, author lists, glossaries) will come as part of the product hardening. Until then, fixed values live as fields on the documents.') }}
      </p>
    </div>
  </div>
</template>

<style scoped>
.ds__placeholder {
  text-align: center;
  border: 1px dashed var(--rz-border, #e6e2da);
  border-radius: 12px;
  padding: 44px 20px;
  background: var(--rz-surface, #f4f2ee);
}
</style>
