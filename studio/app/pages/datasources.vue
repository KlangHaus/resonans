<script setup lang="ts">
// Datasources — planlagt feature (Storyblok-paritet, open-core-produktsporet):
// delte nøgle/værdi-lister (fx kategorier, forfattere, tags) der kan refereres
// fra felter på tværs af dokumenter. Ikke bygget endnu — pladsholderen holder
// navigationen ærlig om roadmappet.
</script>

<template>
  <div style="max-width: 620px">
    <h1 style="margin: 0 0 6px; font-size: 26px; font-weight: 800">Datasources</h1>
    <p class="rz-field__hint" style="margin: 0 0 20px">
      Delte nøgle/værdi-lister til genbrug på tværs af indhold.
    </p>
    <div class="ds__placeholder">
      <div style="font-size: 30px; margin-bottom: 10px">🗂</div>
      <strong>På roadmappet</strong>
      <p class="rz-field__hint" style="max-width: 42ch; margin: 6px auto 0">
        Datasources (fx kategorier, forfatter-lister, gloser) kommer som del af
        produkt-hærdningen. Indtil da bor faste værdier som felter på dokumenterne.
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
