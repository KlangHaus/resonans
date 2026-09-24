-- Every table that carries a tenant_id must have row level security ENABLED and
-- FORCED. Run this against your database after migrating.
--
-- Why it is not optional. Resonans' tenant isolation is two halves: the server
-- connects as a role that cannot bypass RLS, and every tenant table carries a
-- policy. The role half is verifiable once and then stays true. The table half
-- is re-decided by every migration, and a migration that forgets is INVISIBLE
-- to the role half — the role is still correct, and the data is still readable
-- across tenants. This query is the half that can regress.
--
--   psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f deploy/assert-rls-coverage.sql
--
-- It exits non-zero and names the offending tables, so it can be a deploy gate.

\set ON_ERROR_STOP on

do $check$
declare
  unguarded text;
  total int;
begin
  -- The denominator first: a query that finds nothing because it is looking in
  -- the wrong place would otherwise read exactly like a pass.
  -- Every non-system schema, deliberately. An earlier version hardcoded
  -- `public`, and Resonans keeps its tables in `resonans_app` — so against a
  -- real database it found nothing. Naming no schema is the version that cannot
  -- be pointed at the wrong one.
  select count(*) into total
  from pg_class c
  join pg_attribute a on a.attrelid = c.oid
  join pg_namespace n on n.oid = c.relnamespace
  where c.relkind = 'r' and a.attname = 'tenant_id' and not a.attisdropped
    and n.nspname not in ('pg_catalog', 'information_schema')
    and n.nspname not like 'pg_toast%' and n.nspname not like 'pg_temp%';

  if total = 0 then
    raise exception 'found no table with a tenant_id column in any user schema — this assertion is looking at the wrong database, which is not the same as passing';
  end if;

  select string_agg(n.nspname || '.' || c.relname, ', ' order by n.nspname, c.relname) into unguarded
  from pg_class c
  join pg_attribute a on a.attrelid = c.oid
  join pg_namespace n on n.oid = c.relnamespace
  where c.relkind = 'r' and a.attname = 'tenant_id' and not a.attisdropped
    and n.nspname not in ('pg_catalog', 'information_schema')
    and n.nspname not like 'pg_toast%' and n.nspname not like 'pg_temp%'
    and not (c.relrowsecurity and c.relforcerowsecurity);

  if unguarded is not null then
    raise exception 'tenant tables without enable+force row level security: %', unguarded;
  end if;

  raise notice 'row level security enabled and forced on all % tenant tables', total;
end
$check$;
