-- Resonans: database and roles, for a setup you run BY HAND.
--
-- Use THIS file — not deploy/initdb/10-app-role.sql — whenever you are not the
-- postgres image running that file as POSTGRES_USER on a cluster's first start.
-- A hand-run setup differs in one way that changes three statements: the person
-- at the psql prompt is a THIRD role, neither the migrator nor the owner of what
-- the migrator will create. Everything below is written so it does not matter who
-- runs it.
--
-- Run it with psql, because it changes database mid-file:
--
--   RESONANS_MIGRATOR_PASSWORD=... RESONANS_APP_PASSWORD=... \
--     psql -v ON_ERROR_STOP=1 -U postgres -d postgres -f deploy/manual-setup.sql
--
-- Passwords come from the environment so they stay out of your shell history and
-- out of this file. Use HEX, not base64: a `/` in a base64 password breaks DSN
-- parsing in several clients, measured the hard way on 2026-09-20.
--
-- Then run the migrations as the migrator, and only then start the server:
--
--   DATABASE_URL=postgres://resonans_migrator:...@HOST:5432/resonans resonans migrate
--
-- Three things below are load-bearing and each has a measurement behind it.
-- They are numbered so a code review can point at one.

\set ON_ERROR_STOP on
\set migrator_password `echo "$RESONANS_MIGRATOR_PASSWORD"`
\set app_password `echo "$RESONANS_APP_PASSWORD"`

-- The `\o` redirect is load-bearing: `set_config` returns the value it sets and
-- psql prints returned rows, so without it both passwords are written in cleartext
-- to your terminal and into any log or transcript of this run. Measured on
-- postgres:16 with a canary. Do not "clean this up" into a `do $$ ... perform ...`
-- block: psql does not interpolate `:'name'` inside a dollar-quoted body, and that
-- form fails at parse time on the server.
\o /dev/null
select set_config('resonans.mp', :'migrator_password', false);
select set_config('resonans.ap', :'app_password', false);
\o
do $guard$
begin
  if coalesce(current_setting('resonans.mp', true), '') = ''
     or coalesce(current_setting('resonans.ap', true), '') = '' then
    raise exception 'RESONANS_MIGRATOR_PASSWORD and RESONANS_APP_PASSWORD must both be set — refusing to create passwordless roles';
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

-- (1) ROLES BEFORE THE DATABASE.
-- Not style: the database is created OWNER resonans_migrator below, so the role
-- has to exist first. Creating the database first and the roles after is the
-- ordering a hand-written guide naturally produces, and it forces you into (2).
create role resonans_migrator with
  login password :'migrator_password'
  nosuperuser nocreatedb nocreaterole;

create role resonans_app with
  login password :'app_password'
  nosuperuser nocreatedb nocreaterole noinherit nobypassrls;

-- (2) THE MIGRATOR OWNS THE DATABASE.
-- `create schema if not exists` is checked against CREATE on the DATABASE
-- *before* the existence check. So a migrator that does not own the database is
-- refused with SQLSTATE 42501 even when the schema is already there and nothing
-- would be created. Measured 2026-09-28 on postgres:16, three databases with the
-- same role shape in one run:
--
--   superuser owns the db, schema pre-created, no CREATE granted
--     -> resonans migrate: permission denied for database (42501), 0 tables
--   same, plus `grant create on database resonans to resonans_migrator`
--     -> green, 5 tables
--   `create database resonans owner resonans_migrator`   (this file)
--     -> green, 5 tables
--
-- The third is chosen over the second because it grants nothing anyone has to
-- remember to revoke later. Newer Resonans looks the schema up before creating it
-- (internal/persistence/migrate.go, ensureSchema), so the first shape works too —
-- but this file must not depend on the server being new enough.
create database resonans owner resonans_migrator;

-- (3) NOW CHANGE DATABASE. This is a command, not a comment.
-- Everything below belongs to `resonans`. Run it while still connected to
-- `postgres` and the schema lands in the WRONG database: measured, the schema
-- appeared in `postgres` and `resonans` had none, and the only instruction to
-- reconnect was a comment in an earlier draft of this setup. `resonans migrate`
-- then failed with a named error and 0 tables — loudly, which is the one mercy —
-- but the deploy still stopped. A step a human has to remember is not a step.
\connect resonans

-- The tables live in the `resonans_app` SCHEMA, not in `public`. Granting on
-- `public` produced an app role that could not read its own tables at all, and
-- every request answered `permission denied for schema resonans_app`.
create schema if not exists resonans_app authorization resonans_migrator;

grant usage on schema resonans_app to resonans_app;

-- (4) `FOR ROLE` IS LOAD-BEARING.
-- `alter default privileges` attaches to the role that RUNS it unless `for role`
-- says otherwise, and it is per-SCHEMA. You are running this as a third role; the
-- TABLES will be created by resonans_migrator during `resonans migrate`. Without
-- `for role` the defaults attach to YOU, the migrator's tables get nothing, and
-- the app role answers `permission denied for table documents` on every request.
-- Measured against postgres:16-alpine with the working variant as the control in
-- the same run.
alter default privileges for role resonans_migrator in schema resonans_app
  grant select, insert, update, delete on tables to resonans_app;
alter default privileges for role resonans_migrator in schema resonans_app
  grant usage, select on sequences to resonans_app;

-- Nothing grants BYPASSRLS or SUPERUSER to either role, which is what makes
-- tenant isolation hold. A table that carries a tenant_id and forgot
-- `enable`/`force row level security` is still fully visible to the app role, so
-- run deploy/assert-rls-coverage.sql after migrating — it is part of the model,
-- not an extra.
