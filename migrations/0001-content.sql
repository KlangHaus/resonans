-- Resonans content model — first migration.
--
-- Beslutning B (single-site now, multi-site later): the `site` column is
-- present from day one so multi-site + Postgres RLS can be introduced later
-- without a table rewrite. RLS policies are intentionally NOT added yet — there
-- is one site (klanghaus.dk) and no tenant boundary to enforce.

create table if not exists resonans_app.entries (
  id            uuid        primary key default gen_random_uuid(),
  site          text        not null,
  -- content type: 'page' | 'post' | 'board-minute' | ... (open-ended, not an
  -- enum, so new types don't need a migration).
  type          text        not null,
  slug          text        not null,
  title         text        not null,
  -- Structured body as JSON (portable blocks); editors may store rich content.
  body          jsonb       not null default '{}'::jsonb,
  status        text        not null default 'draft'
                            check (status in ('draft', 'published')),
  published_at  timestamptz,
  created_at    timestamptz not null default now(),
  updated_at    timestamptz not null default now()
);

-- One slug per site (a slug identifies a piece of content within its site).
create unique index if not exists entries_site_slug_uniq
  on resonans_app.entries (site, slug);

-- Fast lookups for the public content read path (published content by site).
create index if not exists entries_site_status_idx
  on resonans_app.entries (site, status);

-- Listing by type within a site.
create index if not exists entries_site_type_idx
  on resonans_app.entries (site, type);
