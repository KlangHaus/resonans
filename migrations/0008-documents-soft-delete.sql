-- Soft delete for documents (Allan 2026-09-20: "soft delete, så det kan fortrydes").
--
-- An editor could create, edit and publish, but never remove: there was no
-- DELETE route anywhere in the API (measured on de989fd: 21 routes, 0 DELETE).
-- A page created by mistake stayed forever.
--
-- 🔴 SOFT, NOT HARD, AND THE DECISION IS THE POINT.
--    `deleted_at` null = live. Deleting sets the timestamp; restoring clears
--    it. The row, its versions and its history stay, so a mistaken delete is a
--    mistake and not a loss. Hard delete (for a real erasure request) is a
--    separate, deliberate operation and is NOT this migration.
--
-- 🔴 THE UNIQUE INDEX BECOMES PARTIAL, AND THAT IS A BEHAVIOUR CHANGE.
--    The old index reserved (tenant, dataset, type, slug) for every row,
--    deleted or not — so a deleted "forside" would block creating a new
--    "forside" forever, which is the opposite of what an editor expects from a
--    bin. The new index only covers live rows.
--    The cost is named rather than hidden: if the slug is taken again, the
--    deleted document CANNOT be restored as-is; the restore fails with a slug
--    conflict instead of silently violating the constraint or stealing the
--    slug back from the live document.
alter table resonans_app.documents
  add column if not exists deleted_at timestamptz;

drop index if exists resonans_app.documents_tenant_dataset_type_slug_uniq;

create unique index if not exists documents_tenant_dataset_type_slug_live_uniq
  on resonans_app.documents (tenant_id, dataset, type, slug)
  where deleted_at is null;

-- The delivery/preview read paths filter on deleted_at, so give them an index
-- that matches the queries they actually run.
create index if not exists documents_tenant_type_status_live_idx
  on resonans_app.documents (tenant_id, dataset, type, status)
  where deleted_at is null;
