# Resonans

KlangHaus' eget self-hostede **headless CMS** (Go). Intern dogfood — dogfooder
KlangHaus' eget marketing-indhold (`klanghaus.dk`) + Ethical Board-referater —
frem for et proprietært SaaS-CMS. Anti-lock-in (Manifest §13), EU-residens.
Erstatter Sanity/Contentful i stakken. **Ikke offentligt OSS-released.**

Produkt-brief: `klanghaus/docs/product-briefs/resonans/index.md`.

## Stack

- **Go** content-API (REST — Beslutning W: content-API til egne Nuxt-sites).
- **Postgres** (Beslutning K, self-host). Migrations køres ved boot (embedded).
- **OTel** fra dag ét (traces + metrics + logs, OTLP/HTTP; no-op uden endpoint).
- Distroless-image, kører som Compose-workload på **VM2 (ops)** bag Caddy
  (`indhold.klanghaus.dk`).

## Kør lokalt

```bash
# Uden DB (in-memory content-store, dev only):
go run ./cmd/resonans

# Med Postgres + boot-migrations via Compose:
docker compose -f docker-compose.tilt.yml up --build
```

| Action    | Kommando               |
| --------- | ---------------------- |
| Lint      | `golangci-lint run`    |
| Format    | `gofumpt -w .`         |
| Vet       | `go vet ./...`         |
| Test      | `go test -race ./...`  |
| Build     | `go build ./...`       |

## API (v1)

Læse-flader er **offentlige** (published-indhold serveres til vores Nuxt-sites).
Skrive-flader (editor) er gated (se nedenfor).

| Metode | Rute                                   | Adgang  |
| ------ | -------------------------------------- | ------- |
| GET    | `/api/v1/entries?site=&type=&status=`  | public  |
| GET    | `/api/v1/entries/{id}`                 | public  |
| GET    | `/api/v1/content/{site}/{slug}`        | public  |
| POST   | `/api/v1/entries`                      | editor  |
| PUT    | `/api/v1/entries/{id}`                 | editor  |
| POST   | `/api/v1/entries/{id}/publish`         | editor  |
| GET    | `/healthz`, `/readyz`                  | probes  |

## Editor-auth (placeholder)

Skrive-endpoints gates i dag af en shared-secret bearer (`EDITOR_API_TOKEN`).
Det er **midlertidigt**: den kanoniske editor-login er BetterAuth-JWT + PDP-gating
via [auth]/[permissions] (follow-up issue). Fail-closed: i `production` nægter
serveren at boote uden token.

## Deploy-kontrakt

Honoreres af scaffoldet — ændr ikke uden en samtidig `infra/compose`-ændring:

- `DATABASE_URL=postgres://resonans:…@postgres:5432/resonans`
- `PUBLIC_URL=https://indhold.klanghaus.dk`
- lyt på `:3000` (Caddy `reverse_proxy resonans:3000`)

## Åbne punkter

- Redaktør-UI (Nuxt/Vue + grundtone-komponenter) — ikke i dette repo endnu.
- Multi-site + Postgres RLS — udskudt (Beslutning B); `entries.site` findes fra
  dag ét så det kan landes uden migrations-rewrite.
- BetterAuth-JWT + PDP editor-login — erstatter shared-secret-gaten.
- PostgresStore integrations-tests (kræver kørende Postgres).
