# CLAUDE.md — resonans

Project-level instructions for any Claude session working in this repo.
Personal preferences live in your global memory; everything here is repo-wide.

## Worker-ejer

**[resonans]** (builder under [infra] CTO) — prompt:
`klanghaus/.orchestration/prompts/resonans.md`.

Resonans = KlangHaus' eget self-hostede headless CMS (Go). Intern dogfood,
**ikke** offentligt OSS-released. Produkt-brief:
`klanghaus/docs/product-briefs/resonans/index.md`.

## Låste beslutninger (denne service)

- **Stack = Go + go-starter** (Allan 2026-07-20; Go-everywhere).
- **API = REST** (Beslutning W: content-API til egne Nuxt-sites).
- **Single-site nu, multi-site + RLS senere** (Beslutning B) — `entries.site`
  findes fra dag ét; ingen migrations-rewrite ved multi-site.
- **Redaktør-UI genbruger grundtone-komponenter** (afklares m/ [grundtone]).
- **OTel fra dag ét** — site/tenant.id pr. request på spans, ikke på resource.

## Standing rules (link, ikke duplikat)

- Quality gates: `klanghaus/.orchestration/QUALITY-GATES.md` (L+M+N fra dag ét;
  70% branch default, 90% på penge-/auth-nær kode).
- PLAN + låste beslutninger: `klanghaus/.orchestration/PLAN.md`.
- **INGEN `Co-Authored-By` i commits** (org-konvention).
- **Conventional commits + commitlint.** Issue-first (Beslutning M): PR-body
  `Closes #N`. Branch: `<type>/<issue#>-<slug>`.
- **Merge-gate:** grøn CI + [review]-verdict + Allan merger. **Aldrig
  `--no-verify` / `--admin`.** Sletter ALDRIG repos.

## Hvordan man kører

| Action    | Go                    |
| --------- | --------------------- |
| Setup     | `go mod download`     |
| Lint      | `golangci-lint run`   |
| Format    | `gofumpt -w .`        |
| Typecheck | `go vet ./...`        |
| Test      | `go test -race ./...` |
| Build     | `go build ./...`      |
| Dev       | `go run ./cmd/resonans` (eller `docker compose -f docker-compose.tilt.yml up`) |

## Layout

- `cmd/resonans/` — entrypoint (run + healthcheck + version).
- `internal/app/` — boot-wiring (config → otel → db/migrate → router → shutdown).
- `internal/config/` — env-config (honorér VM2 deploy-kontrakt).
- `internal/content/` — domæne: Entry + Store (Postgres + in-memory).
- `internal/httpapi/` — REST-adaptering (ingen forretningslogik).
- `internal/editorauth/` — write-gate (shared-secret bearer; JWT-placeholder).
- `internal/observability/` — OTel-setup (kopieret fra permissions/notifications).
- `internal/persistence/` — pgx-pool + boot-migrations.
- `migrations/` — embedded *.sql (køres ved boot).

## Deploy

Compose-workload på VM2 (ops), bag Caddy (`indhold.klanghaus.dk`). Deploy-stub:
`klanghaus/infra/compose/prod/vm2-ops/`. Kontrakt: `DATABASE_URL`, `PUBLIC_URL`,
port `:3000`. Secrets via SOPS+age ([infra]).

## Coverage-tier (Beslutning L2)

Default = 70% branch. `PostgresStore` kræver en kørende Postgres →
integrations-tests er en follow-up (som permissions' `internal/assertions`-job);
in-memory-store + config + httpapi + editorauth er unit-dækket.
