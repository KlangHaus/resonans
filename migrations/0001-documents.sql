-- Resonans content model — multi-tenant documents (Fase 0 foundation).
--
-- Resonans is a broad, content-type-agnostic headless CMS: a `document` holds
-- any content type (course, module, lesson, page, post, resource, ...). The
-- concrete per-type field/block schema is validated by the schema-registry
-- (Fase 1); at the storage layer a document is a typed, tenant-scoped JSONB
-- body. Beslutning K: every row is tenant-scoped (tenant_id ≡ org_id) and
-- isolated by RLS (see 0002).
--
-- `dataset` is the Sanity-style content space within a tenant (default
-- 'production'); it maps to tenant_registry.cms_dataset on the Etude side.

create table if not exists resonans_app.documents (
  id            uuid        primary key default gen_random_uuid(),
  tenant_id     uuid        not null,
  dataset       text        not null default 'production',
  -- content type: 'course' | 'module' | 'lesson' | 'page' | 'post' |
  -- 'resource' | ... — open-ended (schema-registry defines the valid set per
  -- space), so a new type never needs a migration.
  type          text        not null,
  slug          text        not null,
  title         text        not null,
  -- Typed content body (fields + block-array) as JSON; validated against the
  -- schema-registry in the application layer (Fase 1).
  body          jsonb       not null default '{}'::jsonb,
  status        text        not null default 'draft'
                            check (status in ('draft', 'published')),
  published_at  timestamptz,
  created_at    timestamptz not null default now(),
  updated_at    timestamptz not null default now()
);

-- One slug per (tenant, dataset, type).
create unique index if not exists documents_tenant_dataset_type_slug_uniq
  on resonans_app.documents (tenant_id, dataset, type, slug);

-- Delivery read path: published documents of a type within a tenant/dataset.
create index if not exists documents_tenant_type_status_idx
  on resonans_app.documents (tenant_id, dataset, type, status);
