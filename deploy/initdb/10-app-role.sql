-- Create the role the Resonans server connects as.
--
-- This runs once, on the database cluster's FIRST start, before any migration.
-- The split is not ceremony. Resonans isolates tenants with Postgres row level
-- security, and RLS is bypassed by any role that is SUPERUSER or has BYPASSRLS.
-- The migrator owns the schema and needs DDL; the server needs rows and nothing
-- more. Run the server as the owner and every tenant's content becomes readable
-- to every other tenant the moment one policy is mistyped, with nothing to
-- notice it.
--
-- `force row level security` in the migrations binds the table OWNER too. Be
-- precise about what that buys HERE, because it is easy to over-read: in this
-- compose file the owner IS the cluster superuser, and a superuser bypasses RLS
-- whatever `force` says. Measured: the migrator reads every tenant's rows with
-- no tenant GUC set. So `force` adds nothing against THIS migrator connection —
-- its value is conditional and future, for the day the schema owner is not a
-- superuser. The migrator connection is not fenced; treat its DSN accordingly.
--
-- What does fence the server is this role: nosuperuser + nobypassrls.
--
-- One thing this role model CANNOT see: a table that carries a tenant_id and
-- forgot `enable`/`force row level security` is fully visible to it. The role
-- stays correct while the data leaks, so the coverage assertion in
-- deploy/assert-rls-coverage.sql is part of the model, not an extra.

\set ON_ERROR_STOP on
\set app_password `echo "$RESONANS_APP_PASSWORD"`

-- Refuse to continue on an empty password. Postgres would otherwise emit only
-- `NOTICE: empty string is not a valid password, clearing password` and create
-- the role WITHOUT one — and initdb notices scroll past. compose.yml already
-- guards this with ${VAR:?}, but after publication strangers will run this file
-- without that compose file, and a failure should name itself at init rather
-- than as a connection error later.
--
-- Measured fail direction, for the record: a passwordless role still cannot
-- authenticate from another host, where pg_hba requires scram-sha-256. It only
-- looked permissive over the unix socket and loopback, where the image's own
-- `trust` rule accepts any password from any role. So this check hardens a
-- failure that was already closed to network clients — it is about a legible
-- error, not about a hole.
select set_config('resonans.app_password', :'app_password', false);
do $guard$
begin
  if coalesce(current_setting('resonans.app_password', true), '') = '' then
    raise exception 'RESONANS_APP_PASSWORD is empty or unset — refusing to create a passwordless resonans_app role';
  end if;
end
$guard$;

do $$
begin
  if current_setting('server_version_num')::int < 140000 then
    raise exception 'Resonans needs PostgreSQL 14 or newer, found %', current_setting('server_version');
  end if;
end
$$;

create role resonans_app with
  login password :'app_password'
  nosuperuser nocreatedb nocreaterole noinherit nobypassrls;

-- Resonans keeps its tables in the `resonans_app` SCHEMA, not in `public`.
-- Create it here so the grants below have something to attach to; the migrator's
-- own `create schema if not exists` then finds it already present. Getting this
-- wrong is not a subtle failure: granting on `public` produced an app role that
-- could not read its own tables at all, and every request answered
-- `permission denied for schema resonans_app`.
create schema if not exists resonans_app authorization resonans_migrator;

grant usage on schema resonans_app to resonans_app;

-- No table exists yet, so grant on what the migrator WILL create. Default
-- privileges attach to the GRANTING role, which is why this has to run as the
-- same role the migrations run as — and they are per-SCHEMA, so the schema has
-- to be named, not inherited.
alter default privileges in schema resonans_app
  grant select, insert, update, delete on tables to resonans_app;
alter default privileges in schema resonans_app
  grant usage, select on sequences to resonans_app;
