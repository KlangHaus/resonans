<script setup lang="ts">
// The studio overview from klanghaus.dk, driven by a Resonans feature_grid
// (variant "plan"): one feature per studio.
//   title  "Studie 3"       → the number
//   body   "15 m²"          → the size
//   icon   "2026-11-01"     → free from that date; empty = not free
// On klanghaus.dk this lived in studios.ts and changed only with a deploy; here
// an editor changes it in Resonans and the next request shows it.
import { GTButton } from '@grundtone/vue'
import { SITE, PLAN_TOP, PLAN_BOTTOM } from '~/site'
import type { Feature } from '~/composables/useSections'

const props = defineProps<{ studios: Feature[] }>()

const MONTHS = ['januar', 'februar', 'marts', 'april', 'maj', 'juni', 'juli', 'august', 'september', 'oktober', 'november', 'december']
const dateLabel = (iso: string) => {
  const [, m, d] = iso.split('-').map(Number)
  return `${d}. ${MONTHS[m! - 1]}`
}
const dateShort = (iso: string) => {
  const [, m, d] = iso.split('-').map(Number)
  return `${d}. ${MONTHS[m! - 1]!.slice(0, 3)}`
}

const studios = computed(() =>
  props.studios
    .map((f) => ({
      nr: Number.parseInt(String(f.title ?? '').replace(/\D+/g, ''), 10),
      m2: Number.parseInt(String(f.body ?? ''), 10),
      availableFrom: /^\d{4}-\d{2}-\d{2}$/.test(String(f.icon ?? '')) ? String(f.icon) : undefined,
    }))
    .filter((s) => Number.isFinite(s.nr) && Number.isFinite(s.m2)),
)
// Rendered per request, so "fra 1. november" turns into "nu" on the day itself.
const today = new Date().toISOString().slice(0, 10)
const room = (nr: number) => studios.value.find((s) => s.nr === nr)
const sizes = computed(() => {
  const c = new Map<number, number>()
  for (const s of studios.value) c.set(s.m2, (c.get(s.m2) ?? 0) + 1)
  return [...c.entries()].sort((a, b) => a[0] - b[0])
})
const free = computed(() =>
  studios.value
    .filter((s) => s.availableFrom)
    .map((s) => ({
      ...s,
      label: s.availableFrom! > today ? `fra ${dateLabel(s.availableFrom!)}` : 'nu',
      short: s.availableFrom! > today ? dateShort(s.availableFrom!) : 'nu',
    })),
)
const freeById = computed(() => new Map(free.value.map((s) => [s.nr, s])))
const lead = computed(() => {
  const all = studios.value.map((s) => s.m2)
  const range = all.length ? `Fra ${Math.min(...all)} til ${Math.max(...all)} m².` : ''
  const f = free.value
  const status =
    f.length === 0
      ? 'Der er ikke noget ledigt lige nu — skriv dig på ventelisten.'
      : f.length === 1
        ? `Studie ${f[0]!.nr} er ledigt ${f[0]!.label}.`
        : `Ledigt: ${f.map((s) => `studie ${s.nr} (${s.label})`).join(', ')}.`
  return { range, status }
})
</script>

<template>
  <div>
    <p class="kh-sec__lead">
      {{ lead.range }} <strong>{{ lead.status }}</strong>
    </p>
    <ul class="kh-sizes" aria-label="Fordeling efter størrelse">
      <li v-for="[m2, n] in sizes" :key="m2"><b>{{ n }} ×</b>{{ m2 }} m²</li>
    </ul>
    <div class="kh-plan" role="group" aria-label="Skitse af huset med studie 1 til 10, te-køkken og toilet (ikke i målestok)">
      <ol class="kh-plan__row" aria-label="Studie 5, 4, 3, 2 og 1">
        <template v-for="nr in PLAN_TOP" :key="nr">
          <li v-if="room(nr)" class="kh-room" :class="{ 'kh-room--free': freeById.has(nr) }" :style="{ '--m': room(nr)!.m2 }">
            <span class="kh-room__n">{{ nr }}</span>
            <span class="kh-room__m">{{ room(nr)!.m2 }} m²</span>
            <span v-if="freeById.has(nr)" class="kh-room__s">Ledig {{ freeById.get(nr)!.short }}</span>
          </li>
        </template>
      </ol>
      <div class="kh-plan__gang" aria-hidden="true">Gang</div>
      <ol class="kh-plan__row" aria-label="Studie 9, 8, 7, 6 og 10, te-køkken og toilet">
        <template v-for="nr in PLAN_BOTTOM" :key="nr">
          <li v-if="room(nr)" class="kh-room" :class="{ 'kh-room--free': freeById.has(nr) }" :style="{ '--m': room(nr)!.m2 }">
            <span class="kh-room__n">{{ nr }}</span>
            <span class="kh-room__m">{{ room(nr)!.m2 }} m²</span>
            <span v-if="freeById.has(nr)" class="kh-room__s">Ledig {{ freeById.get(nr)!.short }}</span>
          </li>
        </template>
        <li class="kh-room kh-room--fac" style="--m: 6">Te-køkken</li>
        <li class="kh-room kh-room--fac" style="--m: 5">WC</li>
      </ol>
    </div>
    <p class="kh-plan__note">Skitse — ikke i målestok.</p>
    <ul class="kh-legend">
      <li><i class="kh-legend__free"></i>Ledig</li>
      <li><i class="kh-legend__no"></i>Ikke ledig lige nu</li>
    </ul>
    <div v-for="s in free" :key="s.nr" class="kh-avail">
      <div>
        <h3>Studie {{ s.nr }} <span>· {{ s.m2 }} m²</span></h3>
        <p>Ledigt til overtagelse {{ s.label }}. Skriv til os, så fortæller vi mere og viser det.</p>
      </div>
      <div class="kh-avail__act">
        <GTButton as="a" :href="`mailto:${SITE.mail}?subject=Studie%20${s.nr}`" variant="primary" class="kh-btn">
          Hør om studie {{ s.nr }} →
        </GTButton>
        <span class="kh-avail__alt">Ikke det rette? <a href="#venteliste">Skriv dig på ventelisten</a></span>
      </div>
    </div>
  </div>
</template>
