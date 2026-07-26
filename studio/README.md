# Resonans Studio

The Resonans editor — a **schema-driven**, **theme-agnostic** Nuxt 4 SPA. It has
**no hardcoded content types**: the nav, document lists and forms are all
rendered from the backend registry (`GET /api/v1/schema`), so the same editor
serves Etude course/lesson content, blog posts, marketing pages and email — one
editor, many domains.

## Design principles

- **Schema-driven.** `SchemaField.vue` maps a schema field (type + i18n) to an
  input. Add a content type in the backend YAML → it appears here, no code change.
- **Swappable theming** (product-grade / external-CMS track). Everything is
  styled via CSS custom properties in `app/assets/theme.css`. grundtone — or any
  external org's brand — maps onto these variables; components never import a
  design system directly.

## Status (Fase 4 — editor-shell)

GT-independent core: shell + schema-driven nav + document list + generic
document editor (scalars + field-level da/en). **Sequenced next** (waiting on
[grundtone] GTRichText organism + [designer] product-grade design):

- Rich-text block editor (GTRichText) — currently a plain textarea.
- Visual block editor + palette (blocks/array fields currently use a JSON editor).
- grundtone theme mapped onto the theming variables.
- Asset picker (asset/reference fields currently take an id).
- Email doc-type authoring (Beslutning R amendment).

## Dev

```bash
pnpm install
# point at a running Resonans backend + a dev tenant:
NUXT_PUBLIC_API_BASE=http://localhost:3000 NUXT_PUBLIC_DEV_TENANT=<uuid> pnpm dev
```

The backend must run with auth disabled (dev) so `X-Dev-Tenant` is honoured, or
behind the real BetterAuth session in prod.
