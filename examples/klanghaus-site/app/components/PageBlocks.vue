<script setup lang="ts">
// Renders a Resonans page as klanghaus.dk's landing-page sections. The markup and
// class names are klanghaus.dk's own, so its stylesheet applies unchanged; which
// layout a block gets is decided by the block's `variant` (feature_grid,
// section_header) — a hint Resonans stores and never interprets.
import { GTButton } from '@grundtone/vue'
import {
  toSections, str, assetUrl, headingHtml, features, columnImages, isExternal, useEditable,
  type Block, type Section,
} from '~/composables/useSections'

const props = defineProps<{ components: Block[] }>()
// Editable paths, on the preview route only (useEditable returns {} elsewhere).
const ed = useEditable()
const sections = computed(() => toSections(props.components))

const hero = (s: Section) => s.blocks.find((b) => b._type === 'hero')
const facts = (s: Section) => s.blocks.find((b) => b._type === 'feature_grid' && b.variant === 'facts')
const variant = (s: Section) => str(s.header?.variant)

// A quote directly followed by a checklist is one two-column "split" row.
const pairsQuote = (blocks: Block[], i: number) =>
  blocks[i]?._type === 'quote' && blocks[i + 1]?._type === 'feature_grid' && blocks[i + 1]?.variant === 'checklist'
const pairedChecklist = (blocks: Block[], i: number) => i > 0 && pairsQuote(blocks, i - 1)
const ctaAttrs = (href: string) => (isExternal(href) ? { target: '_blank', rel: 'noopener' } : {})
const contactCard = (s: Section) => s.blocks.filter((b) => b._type === 'rich_text')
</script>

<template>
  <template v-for="(s, si) in sections" :key="si">
    <!-- OPENING: hero + facts -->
    <section v-if="!s.header && hero(s)" class="kh-hero" aria-labelledby="hero-title">
      <div class="kh-hero__in">
        <div>
          <p v-if="str(hero(s)!.eyebrow)" class="kh-hero__eyebrow" v-bind="ed(hero(s)!._i, 'eyebrow')">{{ str(hero(s)!.eyebrow) }}</p>
          <!-- eslint-disable-next-line vue/no-v-html -- escaped in headingHtml; only <br> and <em> are produced -->
          <h1 id="hero-title" class="kh-hero__title" v-bind="ed(hero(s)!._i, 'heading')" v-html="headingHtml(str(hero(s)!.heading))" />
          <p v-if="str(hero(s)!.subheading)" class="kh-hero__sub" v-bind="ed(hero(s)!._i, 'subheading')">{{ str(hero(s)!.subheading) }}</p>
          <div class="kh-hero__actions">
            <GTButton v-if="str(hero(s)!.ctaLabel)" as="a" :href="str(hero(s)!.ctaHref)" variant="primary" class="kh-btn" v-bind="ed(hero(s)!._i, 'ctaLabel')">
              {{ str(hero(s)!.ctaLabel) }}
            </GTButton>
            <GTButton
              v-if="str(hero(s)!.secondaryCtaLabel)"
              as="a"
              :href="str(hero(s)!.secondaryCtaHref)"
              variant="unstyled"
              class="kh-btn kh-btn--ghost"
              v-bind="ed(hero(s)!._i, 'secondaryCtaLabel')"
            >
              {{ str(hero(s)!.secondaryCtaLabel) }}
            </GTButton>
          </div>
        </div>
        <div v-if="assetUrl(hero(s)!.image)" class="kh-hero__visual" v-bind="ed(hero(s)!._i, 'image')">
          <img :src="assetUrl(hero(s)!.image)" :alt="str(hero(s)!.imageAlt)" width="1000" height="750" loading="eager" fetchpriority="high" decoding="async" />
        </div>
      </div>
      <div v-if="facts(s)" class="kh-facts">
        <div class="kh-facts__in">
          <div v-for="(f, i) in features(facts(s)!)" :key="i">
            <div class="kh-facts__n" v-bind="ed(facts(s)!._i, 'features', i, 'title')">{{ f.title }}</div>
            <div class="kh-facts__l" v-bind="ed(facts(s)!._i, 'features', i, 'body')">{{ f.body }}</div>
          </div>
        </div>
      </div>
    </section>

    <!-- CONTACT -->
    <section v-else-if="variant(s) === 'contact'" class="kh-contact" :id="str(s.header!.anchor) || undefined" :aria-labelledby="`${str(s.header!.anchor)}-title`">
      <div>
        <h2 :id="`${str(s.header!.anchor)}-title`" class="kh-contact__title" v-bind="ed(s.header!._i, 'heading')">{{ str(s.header!.heading) }}</h2>
        <p v-if="str(s.header!.lead)" class="kh-contact__text" v-bind="ed(s.header!._i, 'lead')">{{ str(s.header!.lead) }}</p>
        <template v-for="(b, bi) in s.blocks" :key="bi">
          <GTButton v-if="b._type === 'cta'" as="a" :href="str(b.href)" v-bind="{ ...ctaAttrs(str(b.href)), ...ed(b._i, 'label') }" variant="primary" class="kh-btn">
            {{ str(b.label) }}
          </GTButton>
        </template>
      </div>
      <!-- eslint-disable-next-line vue/no-v-html -- Resonans sanitizes rich text on save -->
      <div v-for="(b, bi) in contactCard(s)" :key="`c${bi}`" class="kh-contact__card" v-bind="ed(b._i, 'value')" v-html="str(b.value)" />
    </section>

    <!-- ORDINARY SECTION (optionally on a band). The stylesheet has no child
         selectors, so the plain wrapper div on non-band sections changes nothing. -->
    <div v-else-if="s.header" :class="variant(s) === 'band' ? 'kh-band' : undefined">
      <section
        class="kh-sec"
        :class="{ 'kh-waitlist': variant(s) === 'waitlist' }"
        :id="str(s.header.anchor) || undefined"
        :aria-labelledby="`${str(s.header.anchor) || si}-title`"
      >
        <p v-if="str(s.header.eyebrow)" class="kh-sec__eyebrow" v-bind="ed(s.header._i, 'eyebrow')">{{ str(s.header.eyebrow) }}</p>
        <h2 :id="`${str(s.header.anchor) || si}-title`" class="kh-sec__title" v-bind="ed(s.header._i, 'heading')">{{ str(s.header.heading) }}</h2>
        <p v-if="str(s.header.lead)" class="kh-sec__lead" v-bind="ed(s.header._i, 'lead')">{{ str(s.header.lead) }}</p>

        <!-- The waitlist keeps its button and note together in one action row. -->
        <div v-if="variant(s) === 'waitlist'" class="kh-waitlist__action">
          <template v-for="(b, bi) in s.blocks" :key="bi">
            <GTButton v-if="b._type === 'cta'" as="a" :href="str(b.href)" v-bind="{ ...ctaAttrs(str(b.href)), ...ed(b._i, 'label') }" variant="primary" class="kh-btn">
              {{ str(b.label) }}
            </GTButton>
            <!-- eslint-disable-next-line vue/no-v-html -- sanitized on save -->
            <p v-else-if="b._type === 'rich_text'" class="kh-waitlist__note" v-bind="ed(b._i, 'value')" v-html="str(b.value)" />
          </template>
        </div>

        <template v-else>
          <template v-for="(b, bi) in s.blocks" :key="bi">
            <div v-if="b._type === 'feature_grid' && b.variant === 'cards'" class="kh-cards">
              <article v-for="(f, i) in features(b)" :key="i" class="kh-card" v-bind="ed(b._i, 'features', i)">
                <div v-if="f.icon" class="kh-card__icon" aria-hidden="true">{{ f.icon }}</div>
                <h3 class="kh-card__title" v-bind="ed(b._i, 'features', i, 'title')">{{ f.title }}</h3>
                <p class="kh-card__text" v-bind="ed(b._i, 'features', i, 'body')">{{ f.body }}</p>
              </article>
            </div>

            <StudioPlan v-else-if="b._type === 'feature_grid' && b.variant === 'plan'" :studios="features(b)" :block-index="b._i" />

            <ol v-else-if="b._type === 'feature_grid' && b.variant === 'steps'" class="kh-steps">
              <li v-for="(f, i) in features(b)" :key="i" class="kh-step" v-bind="ed(b._i, 'features', i)">
                <p class="kh-step__n">{{ String(i + 1).padStart(2, '0') }}</p>
                <p class="kh-step__title" v-bind="ed(b._i, 'features', i, 'title')">{{ f.title }}</p>
                <p class="kh-step__text" v-bind="ed(b._i, 'features', i, 'body')">{{ f.body }}</p>
              </li>
            </ol>

            <div v-else-if="pairsQuote(s.blocks, bi)" class="kh-split">
              <blockquote class="kh-split__quote" v-bind="ed(b._i, 'quote')">
                <p>"{{ str(b.quote) }}"</p>
                <footer v-if="str(b.author)">{{ str(b.author) }}</footer>
              </blockquote>
              <ul class="kh-split__list">
                <li v-for="(f, i) in features(s.blocks[bi + 1]!)" :key="i" v-bind="ed(s.blocks[bi + 1]!._i, 'features', i)">
                  <span class="kh-split__check" aria-hidden="true">✓</span>
                  <span><b>{{ f.title }}</b> {{ f.body }}</span>
                </li>
              </ul>
            </div>
            <template v-else-if="pairedChecklist(s.blocks, bi)" />

            <div v-else-if="b._type === 'columns' && columnImages(b).length" :class="variant(s) === 'band' ? 'kh-photo-band' : 'kh-gallery'" v-bind="ed(b._i)">
              <picture
                v-for="(img, i) in columnImages(b)"
                :key="i"
                :class="variant(s) === 'band' ? 'kh-photo-band__item' : 'kh-gallery__item'"
              >
                <img :src="img.url" :alt="img.alt" loading="lazy" decoding="async" />
              </picture>
            </div>

            <div v-else-if="b._type === 'cta'" class="kh-sec__cta">
              <p v-if="str(b.heading)" class="kh-sec__lead">{{ str(b.heading) }}</p>
              <GTButton as="a" :href="str(b.href)" v-bind="{ ...ctaAttrs(str(b.href)), ...ed(b._i, 'label') }" variant="primary" class="kh-btn">{{ str(b.label) }}</GTButton>
            </div>

            <div v-else-if="b._type === 'callout'" class="kh-callout" :class="`kh-callout--${str(b.variant) || 'info'}`" role="note" v-bind="ed(b._i, 'value')">
              <!-- eslint-disable-next-line vue/no-v-html -- sanitized on save -->
              <div v-html="str(b.value)" />
            </div>

            <!-- eslint-disable-next-line vue/no-v-html -- sanitized on save -->
            <div v-else-if="b._type === 'rich_text'" class="kh-prose" v-bind="ed(b._i, 'value')" v-html="str(b.value)" />

            <blockquote v-else-if="b._type === 'quote'" class="kh-split__quote" v-bind="ed(b._i, 'quote')">
              <p>"{{ str(b.quote) }}"</p>
              <footer v-if="str(b.author)">{{ str(b.author) }}</footer>
            </blockquote>

            <figure v-else-if="b._type === 'image' && assetUrl(b.asset)" class="kh-figure" v-bind="ed(b._i, 'asset')">
              <img :src="assetUrl(b.asset)" :alt="str((b.asset as { alt?: unknown }).alt)" loading="lazy" decoding="async" />
              <figcaption v-if="str(b.caption)">{{ str(b.caption) }}</figcaption>
            </figure>
          </template>
        </template>
      </section>
    </div>
  </template>
</template>
