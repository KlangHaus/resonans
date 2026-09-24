# Contributing to Resonans

Thanks for being here. Resonans is small and deliberate, so this document is mostly about
*how we argue about correctness* — that part is unusual and worth reading before you spend
an evening on a patch.

## Getting set up

```bash
go build ./...
go test -race ./...
```

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
By opening a pull request you license your contribution under those same terms.

**You keep the copyright in what you write.** We do not ask for assignment, which cuts both
ways and we would rather say it plainly than have you find out later: KlangHaus can grant a
different licence for the parts it wrote itself, and cannot do so for yours without asking
you first.

## Code of conduct

[CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) applies everywhere in this project.
