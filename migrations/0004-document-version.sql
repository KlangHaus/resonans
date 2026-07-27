-- Monotonic revision counter per document — bumped on every update and publish.
-- The delivery envelope exposes it as `version`; consumers (Etude Nitro) use it
-- as the cache key / ETag so they can invalidate per tenant/doc/locale/version.
alter table resonans_app.documents
  add column version integer not null default 1;
