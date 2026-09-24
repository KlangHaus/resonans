-- Assets get the same bin documents got in 0008: an editor who uploaded the
-- wrong file could not remove it at all, which made every upload permanent.
--
-- Simpler than 0008 in one respect and that is worth stating: assets carry NO
-- unique constraint, so there is no partial-index dance here. A deleted asset
-- cannot hold anything hostage the way a deleted page's slug could.
--
-- The BYTES are deliberately left alone. Deleting metadata while the object
-- stays in storage is what makes the undo real — a bin that had already thrown
-- the file away would be a delete with extra steps. Retracting bytes from a CDN
-- and hard delete for an actual erasure request are separate, named non-goals.

alter table resonans_app.assets
  add column if not exists deleted_at timestamptz;

-- Every read path filters on `deleted_at is null`, and the existing
-- assets_tenant_idx does not cover it. This one serves the listing the editor
-- actually loads.
create index if not exists assets_tenant_live_idx
  on resonans_app.assets (tenant_id, created_at desc)
  where deleted_at is null;

-- And the bin's own listing, which is small but is a separate query.
create index if not exists assets_tenant_deleted_idx
  on resonans_app.assets (tenant_id, deleted_at desc)
  where deleted_at is not null;
