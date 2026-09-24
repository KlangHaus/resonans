# Resonans

A self-hosted, multi-tenant **headless CMS** written in Go. One content engine behind
many surfaces: a learning platform, a blog, marketing pages, a knowledge base. Content
types are declared in YAML rather than compiled in, so adding a type is an edit, not a
release.

Built by [KlangHaus](https://klanghaus.dk) to replace hosted CMS-as-a-service for our own
sites, and released so you can run the same thing yourself. It is the CMS behind our
production content — the bugs get found by us first.

**Status:** used in production by its authors. The API is versioned (`/api/v1`) but young;
read [CHANGELOG.md](CHANGELOG.md) before upgrading.

## Why this and not the others

- **It is yours.** AGPL-3.0, no seat pricing, no API request quota, no vendor who can
  reprice the thing your website depends on. Data lives in your Postgres.
- **Multi-tenant from the first commit**, enforced in the database with row level
  security — not by remembering to add `where tenant_id = …`. See
  [Tenant isolation](#tenant-isolation).
- **It refuses to boot into an unsafe state.** No authentication configured, no database,
  no durable asset storage: the server exits instead of starting in a degraded mode you
  would discover months later. Every one of those refusals exists because the permissive
  version of it shipped first.
- **No CDN account required.** Assets go on a volume and Resonans serves them. Put a CDN
  in front when you want one, not to get started.
- **Small.** One Go binary in a distroless image, plus Postgres. No Redis, no queue, no
  search cluster.

It does **not** manage users or send email. It verifies JWTs from an identity provider you
already run.

## Quickstart

```bash
git clone https://github.com/KlangHaus/resonans.git
cd resonans
cp .env.example .env
$EDITOR .env          # fill in the passwords and point the JWT settings at your IdP
docker compose up -d
```

Editor at <http://localhost:8080>, content API at <http://localhost:3000>.

`compose.yml` deliberately does **not** terminate TLS or bind to a public address. Put
your own reverse proxy in front for that. Publishing an editor to the internet is a
decision only you can make, so it is not a default.

### Without Docker

```bash
RESONANS_DEV_AUTH=1 go run ./cmd/resonans
```

That runs with an in-memory store and no login, taking the tenant from an `X-Dev-Tenant`
header. It is for development only, and `RESONANS_DEV_AUTH=1` together with
`NODE_ENV=production` is refused rather than honoured.

## Configuration

Every setting is an environment variable; [.env.example](.env.example) documents all of
them with the reasoning. The ones that decide whether the server starts at all:

| Variable | Required in production | What it does |
| --- | --- | --- |
| `DATABASE_URL` | yes | Postgres DSN. Unset in production the server exits: the in-memory store loses content *and* does not enforce tenant isolation. |
| `RESONANS_JWT_ISSUER` + `RESONANS_JWKS_URL` | yes, both | Editor login. Either one missing and the server exits rather than exposing the editor API to an unauthenticated header. |
| `RESONANS_DELIVERY_TOKEN` | yes | Authenticates the trusted read surface. Unset, `/delivery` would trust a tenant header from anyone. |
| `RESONANS_STORAGE_DIR` *or* all three `BUNNY_STORAGE_*` | yes, one of them | Where asset bytes live. Neither set, the server exits instead of accepting uploads it will drop on restart. |
| `PUBLIC_URL` | recommended | The API's externally reachable base URL. Asset URLs are built from it, so a wrong value shows up as broken images, not as an error. |
| `RESONANS_DEV_AUTH` | never | The single value `1` opens the unauthenticated development paths, outside production only. |

## Tenant isolation

Every document carries a `tenant_id` and a `dataset` (default `production`). Three things
have to hold, and each is checked in a different place:

1. **The tenant comes from a verified token.** The middleware reads the `org` claim of a
   JWT it has verified against the issuer's JWKS. It is never read from a request body or
   header — those are attacker-controlled.
2. **Postgres enforces it.** Every content table has `enable row level security` *and*
   `force row level security`, with policies on an `app.tenant_id` GUC that the
   application sets per transaction. The policies read it as
   `current_setting('app.tenant_id', false)` — not optional — so a query with the GUC
   unset **raises an error** rather than returning an empty result. That is deliberate:
   an empty result is a plausible answer a caller might act on, and an error is not.

   Tables live in the `resonans_app` schema, not `public`. The role that connects needs
   `usage` on that schema and default privileges granted *in* it; granting on `public`
   instead produces a role that cannot read its own tables.

   `force` binds the table owner too — but read [deploy/initdb/](deploy/initdb/) for what
   that does and does not buy you, because a superuser owner bypasses RLS regardless.
3. **Every tenant table actually carries the policy.** The first two hold once and stay
   true; this one is re-decided by every migration, and a migration that forgets is
   *invisible* to the role model — the role stays correct while the data reads across
   tenants. Run [deploy/assert-rls-coverage.sql](deploy/assert-rls-coverage.sql) after
   migrating; it exits non-zero and names the offending tables, and it fails rather than
   passing if it finds no tenant table at all.
4. **The application connects as a role that cannot bypass it.** RLS is ignored by any
   role with `SUPERUSER` or `BYPASSRLS`, which is why the shipped compose file creates two
   roles: a migrator that owns the schema, and the role the server actually uses. See
   [deploy/initdb/](deploy/initdb/). Collapsing them into one is the mistake this design
   exists to prevent.

## Content model

Content types are declarative, in `internal/schema/schemas/` — YAML, versioned in git.
Document types live in `types/*.yaml`, and they share one block library (`blocks.yaml`).
Each type's `body` field names an `allow` list, so a marketing page and a lesson draw
their blocks from the same library without offering each other's.

A document body is JSONB, validated against its schema on write: required fields, unknown
fields, the block allowlist, per-field shape, nested blocks and arrays. `title`, `slug`
and `status` are columns, not body fields.

Field types: `string · text · richtext · number · boolean · asset · reference · blocks ·
array`.

`GET /api/v1/schema` serves the registry so an editor can generate its own forms and
block palette from it.

## HTTP surfaces

**Editor API** — `/api/v1`, tenant from the verified token.

| Method | Route |
| --- | --- |
| `GET` | `/api/v1/schema` · `/api/v1/schema/{type}` |
| `GET` `PUT` | `/api/v1/settings` · `/api/v1/settings/preview` |
| `GET` | `/api/v1/documents?type=&status=&deleted=` · `/api/v1/documents/{id}` |
| `GET` | `/api/v1/content/{type}/{slug}` |
| `POST` `PUT` | `/api/v1/documents` · `/api/v1/documents/{id}` |
| `POST` | `/api/v1/documents/{id}/publish` |
| `DELETE` `POST` | `/api/v1/documents/{id}` · `/api/v1/documents/{id}/restore` |
| `GET` | `/api/v1/documents/{id}/versions` |
| `POST` | `/api/v1/documents/{id}/rollback/{version}` |
| `POST` `GET` | `/api/v1/assets` · `GET /api/v1/assets/{id}` |
| `GET` | `/api/v1/video/embed?id=` · `/api/v1/video/resolve?id=` |
| `POST` | `/api/v1/preview-token` · `/api/v1/preview/envelope` |

**Delivery API** — `/delivery/v1`, published content only, for your own server-side
front-ends. The caller authenticates with `RESONANS_DELIVERY_TOKEN` and passes the tenant
in `X-Scope-OrgID`, which is trusted *only* because the token matched. Never call this
from a browser.

| Method | Route |
| --- | --- |
| `GET` | `/delivery/v1/schema` |
| `GET` | `/delivery/v1/{type}` · `/delivery/v1/{type}/{slug}` |
| `GET` | `/delivery/v1/preview/{type}/{slug}` |

**Unauthenticated** — `/healthz`, `/readyz`, and `/_assets/{key}` when Resonans serves
asset bytes itself.

## Deleting content

`DELETE` moves a document to a bin: it disappears from every read path, and
`?deleted=only` lists what is in there. `POST …/restore` brings it back.

The slug is freed while a document sits in the bin, because a full unique index would let
a deleted page hold its own address hostage forever. The cost is named: restoring into a
slug someone has since taken fails with a conflict rather than silently renaming
anything.

## Asset storage

| Backend | Selected by | Bytes served by | Survives a restart |
| --- | --- | --- | --- |
| `bunny` | `BUNNY_STORAGE_ENDPOINT` + `_ZONE` + `_KEY`, all three | Bunny CDN pull-zone | yes |
| `disk` | `RESONANS_STORAGE_DIR` | Resonans, at `GET /_assets/<key>` | yes, with a volume |
| `memory` | neither of the above | Resonans, at `GET /_assets/<key>` | **no** |

Bunny wins when both are configured: a CDN in front beats an origin shipping image bytes.
`BUNNY_CDN_BASE` defaults to `PUBLIC_URL` + `/_assets`, so leave it empty unless you
actually use Bunny.

Uploads are typed from their **content**, never from the filename or the client's
`Content-Type`, against a short allowlist (PNG, JPEG, GIF, WebP, PDF, ZIP, MP3). No
`text/*` type is accepted, because an SVG or HTML file carrying a script sniffs as
`text/plain` and would then execute on the origin serving it. `/_assets` re-derives the
type from the bytes with the same allowlist on the way out, so a file that reached the
volume by some other route still cannot be served. The disk backend confines itself with
`os.Root`, so the root is enforced by the kernel rather than by string checks.

Video is an optional plugin (`RESONANS_VIDEO_PROVIDER=vimeo`) resolving embeds and oEmbed
metadata over public endpoints, with no token.

## Development

| Action | Command |
| --- | --- |
| Build | `go build ./...` |
| Test | `go test -race ./...` |
| Vet | `go vet ./...` |
| Lint | `golangci-lint run` + `gosec ./...` |
| Format | `gofumpt -w .` |

`golangci-lint` does not include `gosec`, so running only the former is a green about a
different set of tools than CI's.

See [CONTRIBUTING.md](CONTRIBUTING.md).

## Security

Please do not open a public issue for a vulnerability. [SECURITY.md](SECURITY.md) says how
to report one.

## Copyright and license

Copyright (C) 2026 **KlangHaus** (Asp Productions ApS, CVR 36708077, Denmark).
Licensed under the [GNU AGPL-3.0](LICENSE) — see [NOTICE](NOTICE).

If you modify Resonans and offer it to users over a network, §13 says those users are
entitled to your source. Set `RESONANS_SOURCE_URL` to your repository and `/healthz` will
tell them where to find it — leaving it at the upstream default is only truthful if you
changed nothing. It must be an `http(s)` URL; anything else is refused at startup.

That field is **informative**: it is a string for a human to read, and tooling should not
fetch it automatically. A scanner that followed it would be making this instance issue
requests to an address its operator chose.

KlangHaus holds the copyright to its own contributions, so we can grant a different licence
for them. If the AGPL does not fit what you want to build, ask us.

A contributor licence agreement is being prepared, and outside patches wait for it — see
[CONTRIBUTING.md](CONTRIBUTING.md) before spending an evening on one.
