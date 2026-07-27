-- Append-only version history (Fase 5 workflow). A snapshot is written on every
-- publish so editors can review history and roll back. Tenant-scoped + RLS like
-- documents (fail-closed: the app sets app.tenant_id per transaction).
create table if not exists resonans_app.document_versions (
  id           uuid        primary key default gen_random_uuid(),
  tenant_id    uuid        not null,
  document_id  uuid        not null,
  version      integer     not null,
  title        text        not null,
  body         jsonb       not null,
  status       text        not null,
  created_at   timestamptz not null default now()
);

create index if not exists document_versions_doc_idx
  on resonans_app.document_versions (tenant_id, document_id, version desc);

alter table resonans_app.document_versions enable row level security;
alter table resonans_app.document_versions force row level security;

create policy document_versions_tenant_isolation on resonans_app.document_versions
  for all
  using      (tenant_id = current_setting('app.tenant_id', false)::uuid)
  with check (tenant_id = current_setting('app.tenant_id', false)::uuid);
