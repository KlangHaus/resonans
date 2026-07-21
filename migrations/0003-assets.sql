-- Media assets (Fase 3) — tenant-scoped, RLS fail-closed (same discipline as
-- documents). An asset is a stored file (image or generic file); the bytes live
-- in object storage (Bunny Storage), this row holds the metadata + storage key.
-- Content references an asset by its id (schema `asset` field type). Image
-- hotspot/crop lives here; delivery builds a Bunny-CDN transform URL from it.

create table if not exists resonans_app.assets (
  id           uuid        primary key default gen_random_uuid(),
  tenant_id    uuid        not null,
  kind         text        not null check (kind in ('image', 'file')),
  storage_key  text        not null,
  filename     text        not null,
  mime         text        not null,
  size_bytes   bigint      not null default 0,
  width        integer,
  height       integer,
  hotspot      jsonb       not null default '{}'::jsonb,
  created_at   timestamptz not null default now()
);

create index if not exists assets_tenant_idx
  on resonans_app.assets (tenant_id, created_at desc);

alter table resonans_app.assets enable row level security;
alter table resonans_app.assets force row level security;

create policy assets_tenant_isolation on resonans_app.assets
  for all
  using      (tenant_id = current_setting('app.tenant_id', false)::uuid)
  with check (tenant_id = current_setting('app.tenant_id', false)::uuid);
