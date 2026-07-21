-- Row-level security for documents (Beslutning K — tenant isolation).
--
-- Fail-closed variant (mirrors etude/packages/db/drizzle/0002_demo_courses_rls.sql,
-- stricter than the permissions PDP's NULL-tolerant policy): CMS content is
-- ALWAYS tenant-owned, so the GUC MUST be set. `current_setting('app.tenant_id',
-- false)` raises if the GUC is unset (no accidental unscoped read), and the
-- ::uuid cast matches documents.tenant_id.
--
-- ENABLE + FORCE so even the table owner / app role is subject to RLS; only a
-- BYPASSRLS role (the migration runner + ops backup) escapes it. The app must
-- connect as a normal (non-BYPASSRLS) role and set the GUC per transaction via
-- set_config('app.tenant_id', <uuid>, true).

alter table resonans_app.documents enable row level security;
alter table resonans_app.documents force row level security;

create policy documents_tenant_isolation on resonans_app.documents
  for all
  using      (tenant_id = current_setting('app.tenant_id', false)::uuid)
  with check (tenant_id = current_setting('app.tenant_id', false)::uuid);
