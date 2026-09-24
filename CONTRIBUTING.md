# Contributing to Resonans

Thanks for being here. Resonans is small and deliberate, so this document is mostly about
*how we argue about correctness* — that part is unusual and worth reading before you spend
an evening on a patch.

## Getting set up

```bash
go build ./...
go test -race ./...
```

The tenant-isolation cells need a real database and are behind a build tag, so
`go test ./...` never needs one:

```bash
docker run -d --name pg -p 5432:5432 -e POSTGRES_PASSWORD=postgres -e POSTGRES_USER=postgres postgres:16-alpine
RESONANS_TEST_DB_URL="postgres://postgres:postgres@127.0.0.1:5432/postgres?sslmode=disable" \
  go test -count=1 -tags=integration -v ./internal/persistence/...
```

They **fail** rather than skip when that variable is empty. A skipped test and a
passing one look identical on a check-run, and the only way to compile these is to
ask for them — so a missing database is a wiring mistake, not a reason to be quiet.

A full local check, matching CI:

```bash
go build ./... && go vet ./... && go test -race ./... && gofumpt -l . && golangci-lint run && gosec ./...
```

`golangci-lint` does not include `gosec`. Running only the former is green about a
different set of tools than CI uses, which is how a path-traversal finding once reached CI
from a locally-green branch.

Go version comes from `go.mod` and is pinned; don't bump it in a feature PR.

## The one rule that is really ours

**A test that passes is not yet evidence.** Before you claim a guard works, break the
guard and watch the test fail. If it still passes, the test was measuring something else
and you have learned something more valuable than a green tick.

This is not a formality. A recent change added a check against path traversal and a test
that appeared to prove it; mutating the check showed the test never reached it, because
`path.Clean` had already absorbed the attack before the check ran. The test was green, the
reasoning was wrong, and only the mutation said so.

So, in a PR that adds or changes a guard, please say:

- **what you mutated**, and which test went red;
- **what you did not measure** — a green with an unstated denominator reads as broader than
  it is;
- and for a negative result ("X cannot happen"), a **positive control in the same run**, so
  we can tell an absence from a broken instrument.

Two related habits: prefer *provoking* a failure over observing its absence, and make a
fallback value illegal in normal operation, so that a guard failing back to the old
behaviour is loud instead of invisible.

## Commits and PRs

- **Conventional commits**, header at most 100 characters: `feat(media): …`, `fix(api): …`,
  `docs: …`. `commitlint` enforces this and the release tooling reads it, so the type
  decides the next version number.
- **Explain the why in the body.** What changed is in the diff. Why it was wrong before is
  not, and that is the part a reader needs in a year.
- One coherent change per PR. Larger is fine; unrelated is not.
- Don't add lint suppressions to make CI pass. If a linter is wrong, say so in the PR and
  we will discuss it — a suppression that asserts "my validation is enough" is a claim
  substituted for a boundary.

## What we will probably say no to

- A second way to do something that already has a way.
- New required infrastructure — Redis, a queue, a search cluster. Postgres and a binary is
  a feature.
- Reading the tenant from anything other than a verified token, however convenient.
- Making a guard permissive by default "for developer experience". The permissive version
  usually shipped first and was removed for a reason; ask about the reason.
- Trusting a client-supplied MIME type or filename.

## Licensing of contributions

Resonans is [AGPL-3.0](LICENSE), copyright KlangHaus (Asp Productions ApS, CVR 36708077).
Everything published here is, and stays, AGPL-3.0 — that is not what the paragraph below is
about.

**A contributor licence agreement will be required before we can merge code from outside
the company, and it is not written yet.** KlangHaus decided to adopt one on 2026-09-24 and
it is being drafted with legal counsel, tracked as KH-1200. Until it exists we cannot accept
an outside pull request, because merging one would leave the project's licensing unsettled
in a way that is unpleasant to fix afterwards.

**What it will and will not change.** Everything published here stays AGPL-3.0. What the
agreement covers is whether KlangHaus may additionally grant your contribution under other
terms. Today we cannot: we hold the copyright only to what we wrote ourselves, and we
cannot relicense someone else's contribution without asking them. The agreement is how that
changes, and it is the whole reason it exists — we would rather you knew that now than read
it in the small print later.

We would rather tell you this before you spend an evening on a patch than after. So:

- **Issues, bug reports and discussion need no agreement.** They are welcome now.
- **A patch is welcome too, but expect it to wait** for the agreement rather than being
  reviewed and merged straight away.
- When the agreement lands, this section will say exactly what it asks for and what it does
  not, in plain language and before you are asked to sign anything. If it turns out to ask
  for more than you want to give, that is a fair reason not to contribute, and we would
  rather you could tell.

A CLA is a real cost to a project: plenty of capable people will not sign one, and we know
that. It is being adopted deliberately, not as boilerplate.

## Code of conduct

[CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) applies everywhere in this project.
