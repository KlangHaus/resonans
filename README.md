# Resonans

KlangHaus' eget self-hostede **headless CMS** (Go) — bredt og content-type-agnostisk.
Ét system for hele KlangHaus: **Etude-læringsindhold** (multi-tenant LMS) + **blog** +
**marketingssider** (+ senere governance-referater + kundeservice-KB). Anti-lock-in
(Manifest §13), EU-residens. Erstatter Sanity/Contentful. **Ikke offentligt OSS-released.**

Produkt-brief: `klanghaus/docs/product-briefs/resonans/index.md`.

## Stack

- **Go** content-API (REST — Beslutning W).
- **Postgres + RLS**, **multi-tenant fra dag 1** (tenant_id ≡ org_id, pool+silo,
  Beslutning K). Egen content-DB (adskilt fra Etudes money-tier-DB). Migrations køres
  ved boot (embedded).
- **BetterAuth-JWT** verificeret lokalt mod cached JWKS; tenant = `org`-claim.
- **OTel** fra dag ét (traces+metrics+logs; tenant.id per-span).
- Distroless-image, Compose-workload på **VM2** bag Caddy (`indhold.klanghaus.dk`).

## Multi-tenancy + auth

- Hvert **document** er tenant-scoped (`tenant_id`) + `dataset` (default `production`).
- Redaktører logger ind via delt auth (BetterAuth-JWT). Middlewaren udleder tenant fra
  det **verificerede `org`-claim** — aldrig fra request-body/header (anti-spoof). Postgres
  **RLS** håndhæver isolationen i databasen (fail-closed: GUC `app.tenant_id` sat pr.
  transaktion).
- **Dev uden JWT:** sæt tenant via `X-Dev-Tenant`-header (KUN når auth er deaktiveret).
- **Prod:** boot-fejler uden `RESONANS_JWT_ISSUER` + `RESONANS_JWKS_URL` (ingen åbne endpoints)
  og uden `DATABASE_URL` (ingen ikke-persistent/uisoleret store).

## Kør lokalt

```bash
# Uden DB (in-memory store, dev only) — tenant via X-Dev-Tenant:
go run ./cmd/resonans

# Med Postgres + boot-migrations + RLS via Compose:
docker compose -f docker-compose.tilt.yml up --build
```

| Action | Kommando               |
| ------ | ---------------------- |
| Lint   | `golangci-lint run`    |
| Format | `gofumpt -w .`         |
| Vet    | `go vet ./...`         |
| Test   | `go test -race ./...`  |
| Build  | `go build ./...`       |

## API (v1) — tenant-scoped

Alle ruter kræver en tenant (fra verificeret JWT `org`-claim, eller `X-Dev-Tenant` i dev).

| Metode | Rute                                   |
| ------ | -------------------------------------- |
| GET    | `/api/v1/schema` · `/api/v1/schema/{type}` (editor form/palette-kilde) |
| GET    | `/api/v1/documents?type=&status=`      |
| GET    | `/api/v1/documents/{id}`               |
| GET    | `/api/v1/content/{type}/{slug}`        |
| POST   | `/api/v1/documents` (body valideres mod schema) |
| PUT    | `/api/v1/documents/{id}` (body valideres mod schema) |
| POST   | `/api/v1/documents/{id}/publish`       |
| GET    | `/healthz`, `/readyz` (uautentificeret)|

### Delivery-API (read, published-only) — Fase 2

Tenant-scoped read/leverings-flade konsumeret **server-side** af vores Nuxt-sites
(Etude-Nitro + klanghaus.dk). **Betroet service-til-service:** kalderen autentificerer
med `RESONANS_DELIVERY_TOKEN` (bearer) og sender tenant via **`X-Scope-OrgID`** — som
kun trustes når service-token matcher (aldrig fra en slutbruger/browser). Serverer KUN
`published` indhold; RLS håndhæver isolationen uanset.

| Metode | Rute                                    | Formål |
| ------ | --------------------------------------- | ------ |
| GET    | `/delivery/v1/{type}`                   | katalog (published docs af type, fx `course`) |
| GET    | `/delivery/v1/{type}/{slug}`            | ét published document (lesson/page/…) |

Follow-up: cursor-pagination (v1 soft-cap 200), reference-ekspansion (course→modules→lessons),
draft-preview via preview-token (Fase 5).

## Content-model + schema-registry (Fase 1)

Content-typer er **ikke hardcodede** — de defineres deklarativt i
`internal/schema/schemas/` (YAML, git-versioneret): doc-typer (`course → module →
lesson`, `page`, `post`, `resource`) i `types/*.yaml` + et delt **blok-bibliotek**
(`blocks.yaml`, 14 blokke: indhold + marketing). Hver doc-type deklarerer på sit
`body`-felt en **`allow`-liste** (ét bibliotek, rigtige blokke pr. domæne) + `list.columns`
til editorens doc-liste.

- Registret loades ved boot; et document-body (JSONB) **valideres mod schemaet** på
  write (required-felter, kendte felter, blok-allowlist, field-level da/en-shape,
  nested blokke/arrays). `title`/`slug`/`status` er metadata-kolonner (ikke body-felter).
- `GET /api/v1/schema` serverer registret til editoren (form- + palette-generering).
- Felt-typer: `string · text · richtext · number · boolean · asset · reference · blocks · array`.

## Deploy-kontrakt

Honoreres af scaffoldet — ændr ikke uden en samtidig `infra/compose`-ændring:

- `DATABASE_URL` (egen content-DB) · `PUBLIC_URL=https://indhold.klanghaus.dk` · lyt på `:3000`

## Faser (roadmap)

Fase 0 multi-tenant foundation (**denne**) → Fase 1 schema-registry + authoring → **Fase 2
read/delivery-API (Etude M5)** → Fase 3 media (billed-pipeline + Vimeo-plugin) → Fase 4
editor-UI (grundtone) → Fase 5 workflow + preview → Fase 6 GROQ + SCORM.

## Åbne punkter / follow-ups

- Schema-registry + rig content-model (course/lesson-felter, blokke) — Fase 1.
- `PostgresStore` RLS-integrations-tests (kræver kørende Postgres — som permissions'
  `internal/assertions`-job).
- PDP-gating (author/publish pr. tenant) via [permissions].
- Redaktør-UI (Nuxt/Vue + grundtone-komponenter).
