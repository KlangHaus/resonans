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
-- `force row level security` in the migrations binds the table OWNER too, so
-- the one remaining hole is a superuser connection. That is exactly what this
-- role is not.
--
-- RESONANS_APP_PASSWORD is required by compose.yml (`${VAR:?}`), so the stack
-- refuses to start without it rather than creating a passwordless role here.

\set ON_ERROR_STOP on
\set app_password `echo "$RESONANS_APP_PASSWORD"`

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

grant usage on schema public to resonans_app;

-- No table exists yet, so grant on what the migrator WILL create. Default
-- privileges attach to the GRANTING role, which is why this has to run as the
-- same role the migrations run as.
alter default privileges in schema public
  grant select, insert, update, delete on tables to resonans_app;
alter default privileges in schema public
  grant usage, select on sequences to resonans_app;
