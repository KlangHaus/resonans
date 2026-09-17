-- Visual editing: the tenant's frontend preview URL template, e.g.
-- https://etude.example/preview/{type}/{slug}?token={token}&locale={locale}.
-- The Studio loads it in an iframe and derives the only origin it will talk to
-- over postMessage from it. Empty = visual editing off for the tenant.
-- Same table, so the existing FORCE RLS policy scopes the column too.
alter table resonans_app.tenant_settings
  add column if not exists preview_url text not null default '';
