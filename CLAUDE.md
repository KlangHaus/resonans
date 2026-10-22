# CLAUDE.md — resonans

Project-level instructions for any Claude session working in this repo.
Personal preferences live in your global memory; everything here is repo-wide.

## Worker-ejer

**[resonans]** (builder under [infra] CTO) — prompt:
`klanghaus/.orchestration/prompts/resonans.md`.

Resonans = KlangHaus' eget self-hostede headless CMS (Go). Open source under
AGPL-3.0 (offentligt repo siden 2026-10-12, Allans instruks); dogfoodes internt.
Produkt-brief:
`klanghaus/docs/product-briefs/resonans/index.md`.

## Låste beslutninger (denne service)

- **Stack = Go + go-starter** (Allan 2026-07-20; Go-everywhere).
- **API = REST** (Beslutning W: content-API til egne Nuxt-sites).
- **Multi-tenant + RLS fra dag 1** (tenant_id ≡ org_id, pool+silo, Beslutning K)
  — vendte den oprindelige single-site-Beslutning B (Etude = primær forbruger;
  klanghaus.dk-marketing = tenant med count=1).
- **Bredt, content-type-agnostisk CMS** (Etude-læring + blog + marketing) via
  schema-registry.
- **Egen content-DB** (adskilt fra Etudes money-tier-DB).
- **Redaktør-UI genbruger grundtone-komponenter** (afklares m/ [grundtone]).
- **OTel fra dag ét** — tenant.id pr. request på spans, ikke på resource.

## Standing rules (link, ikke duplikat)

- Quality gates: `klanghaus/.orchestration/QUALITY-GATES.md` (L+M+N fra dag ét;
  70% branch default, 90% på penge-/auth-nær kode).
- PLAN + låste beslutninger: `klanghaus/.orchestration/PLAN.md`.
- **INGEN `Co-Authored-By` i commits** (org-konvention).
- **Conventional commits + commitlint.** Issue-first (Beslutning M): PR-body
  `Closes #N`. Branch: `<type>/<issue#>-<slug>`.
- 🔴 **Operator actions go in an `ACTION REQUIRED:` FOOTER, never in the body** — the
  notes generator drops bodies, so a body reaches 0 published surfaces (measured on
  v1.18.3). `scripts/release_notes_guard.mjs` fails the release if that breaks.
  Details: CONTRIBUTING.md.
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
| Test (én) | `python3 scripts/gotest_run_guard.py <mønster> [pakker]` |
| Build     | `go build ./...`      |
| Dev       | `go run ./cmd/resonans` (eller `docker compose -f docker-compose.tilt.yml up`) |

🔴 **Kør ALDRIG `go test -run <mønster>` direkte for at verificere noget.** Et
mønster der ikke matcher noget printer `ok ... [no tests to run]` og giver
**exit 0** — målt 2/10-2026, hvor en lint-rettelse havde omdøbt en test, så en
målrettet verifikation valgte nul tests og læste som bestået. En stave-`sed`
afvæbner alle stående `-run`-mønstre tavst. Vagten gør det rødt, printer sin
nævner (`listed N name(s)`), og skelner «navnet findes ikke» fra «navnet findes
og blev sprunget over». Synsfelt: **kun Go**; `pytest -k`, `vitest -t` og
`jest -t` har samme form og er ikke dækket.

## Layout

- `cmd/resonans/` — entrypoint (run + healthcheck + version).
- `internal/app/` — boot-wiring (config → otel → jwt → db/migrate → router → shutdown).
- `internal/config/` — env-config (honour the deploy contract in README).
- `internal/content/` — domæne: Document + tenant-scoped Store (Postgres+RLS + in-memory).
- `internal/httpapi/` — REST-adaptering, tenant fra actor (ingen forretningslogik).
- `internal/auth/` — BetterAuth-JWT/JWKS-verifier + Actor + net/http-middleware (porteret fra permissions).
- `internal/observability/` — OTel-setup (kopieret fra permissions/notifications).
- `internal/persistence/` — pgx-pool + boot-migrations.
- `migrations/` — embedded *.sql (køres ved boot).

## Deploy

Published image, run behind a reverse proxy that terminates TLS. Contract:
`DATABASE_URL`, `PUBLIC_URL`, one of `RESONANS_STORAGE_DIR` / `BUNNY_STORAGE_*`,
listening on `:3000`. `compose.yml` in this repo is the reference deployment;
secrets come from the environment, never from a file in the image.

This file is published with the repository, so it names no host, no internal
repository path and no secret store.

## Coverage-tier (Beslutning L2)

Default = 70% branch (90% på tenant-isolations-kode — sikkerhedskritisk).
`PostgresStore` + RLS kræver en kørende Postgres → integrations-tests er en
follow-up (som permissions' `internal/assertions`-job); in-memory-store + config +
httpapi + auth (JWT/middleware) er unit-dækket.
