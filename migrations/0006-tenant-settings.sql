-- Per-tenant content profile (Allan 2026-07-28): which doc-types a tenant has
-- enabled. Empty/absent row = ALL types enabled (backwards compatible). First
-- step toward per-tenant content profiles (open-core product direction).
-- Tenant-scoped + FORCE RLS like every other table (fail-closed GUC).
create table if not exists resonans_app.tenant_settings (
  tenant_id     uuid        primary key,
  enabled_types jsonb       not null default '[]'::jsonb,
  updated_at    timestamptz not null default now()
);

alter table resonans_app.tenant_settings enable row level security;
alter table resonans_app.tenant_settings force row level security;

create policy tenant_settings_isolation on resonans_app.tenant_settings
  for all
  using      (tenant_id = current_setting('app.tenant_id', false)::uuid)
  with check (tenant_id = current_setting('app.tenant_id', false)::uuid);
