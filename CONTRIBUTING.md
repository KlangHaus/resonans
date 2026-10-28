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

**They do not run in CI yet, and that is deliberate.** The shared pipeline can
*declare* a database sidecar but does not start one — measured on the one other
repository that declares it, whose equivalent check has been red on its last three
commits to main. Arming a second permanently red check would teach people that red
is background noise, which is the same failure as a red nobody reads. The check is
written and waiting; it lands when the sidecar does.

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
- 🔴 **If an operator has to DO something, put it in an `ACTION REQUIRED:` footer — not in
  the body.** The release notes generator renders only the subject line, so a body is
  dropped from the GitHub release and from `CHANGELOG.md`. Measured once the hard way:
  v1.18.3 fixed a password written in cleartext to the postgres container log, the
  instruction to recreate the container was in the body, and it reached **0** published
  surfaces — the only one carrying it needed a clone and a `git log` of that commit.
  Write it as a real footer, token then colon, or it is not a note and is still dropped:

  ```
  ACTION REQUIRED: recreate your postgres container with `docker compose down -v` and a
  new password. Upgrading the image does not remove it; the log already has it.
  ```

  It publishes under its own `### ACTION REQUIRED` heading, separate from
  `BREAKING CHANGES`, and `scripts/release_notes_guard.mjs` fails the release if that
  stops working. State the wrong inference explicitly, as above — "new image, same
  container, same log" is what a reader otherwise assumes.
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

Resonans is [AGPL-3.0](LICENSE); the copyright is held by the authors of the work, not by
the company that publishes it — [NOTICE](NOTICE) explains why. Everything published here is,
and stays, AGPL-3.0; that is not what the paragraph below is about.

**Outside contributions need a signed contributor licence agreement.** The agreement is
[legal/individual-cla.md](legal/individual-cla.md) (version `cla-1.0`). **No lawyer has
reviewed it** — KlangHaus decided not to pay for one, and we would rather say that than imply
a review that has not happened. Read it before you sign; it is one page.

**What it changes.** Everything published here stays AGPL-3.0. The agreement adds two things,
and the second one is easy to leave out: KlangHaus may additionally grant your contribution
under other terms, including proprietary ones, **and may transfer those rights onward to
another legal entity** — a group company, or a company in the same ownership — without asking
you again. It is a **licence**, not an assignment: you keep your copyright.

Without it we hold the copyright only to what we wrote ourselves and cannot relicense or hand
on someone else's contribution without asking them. The transfer half is named because our own
first requirement list left it out: Ophavsretsloven § 56(2) gives no right to pass a licence
onward unless it is customary or clearly presupposed, so a grant that lists relicensing and
stops there ends at one company.

### 🔴 Before you sign: clause 8, in full

This is the clause a signer most needs to read **before** signing, so here it is, word for
word from the agreement:

> **8. Your personal data and the public record.** To record this agreement, KlangHaus
> processes Your **GitHub username, Your GitHub numeric user ID, the date, and the
> version of this agreement You accepted**, held in a file in the Project's public
> repository. **We record no email address in that file.** Note that the commit You make
> carries whatever author identity Your own git configuration uses, which may include an
> email address; that is Your setting, not our record, and GitHub can supply a
> `noreply` address instead if You prefer.
>
> The purpose is to show who granted which rights. The basis is **Article 6(1)(b) GDPR**
> (this agreement is with You) and **6(1)(f)** (keeping the record as evidence). **The
> record is kept for as long as the Contribution is distributed, plus the general
> limitation period** — the evidence has to outlive what it proves.
>
> 🔴 **This record is public and permanent.** It is part of the repository's history, it
> is copied into every clone and fork, and **it cannot be removed.** We will refuse a
> request to erase it, because the record is what establishes that the Project may use
> and distribute Your Contribution (**Article 17(3)(e) GDPR**: processing necessary for
> the establishment, exercise or defence of legal claims). **You can ask us to correct an
> entry that is wrong**, and we will do so by adding a correcting entry rather than by
> rewriting history.

### How to sign

1. **Read [the agreement](legal/individual-cla.md), and clause 8 above.**
2. **Find your numeric GitHub user ID.** It does not change when you rename your account,
   which is why it is recorded:
   `gh api users/<your-username> --jq .id`, or open `https://api.github.com/users/<your-username>`
   and read `"id"`.
3. **Find the version you are accepting:** the short SHA of the last commit that changed the
   agreement — `git log -1 --format=%h -- legal/individual-cla.md` in a current clone.
4. **From your own GitHub account**, open a pull request that changes **only**
   [CLA.md](CLA.md), appending one line at the end of the table:

   ```
   | @your-username | 1234567 | 2026-10-28 | cla-1.0 (a1b2c3d) |
   ```

   Today's date, UTC. Your username as GitHub shows it.
5. **The `CLA` check verifies the line against the pull request itself**: the user ID and
   username must be the pull request author's, the version must be the current one and its
   SHA the commit that last changed the agreement, the file may only grow by your one line,
   and you must not have signed before. **A line added for someone else is refused** — the
   evidence is that you added it yourself.

Once that pull request is merged, the same check lets your code pull requests through.
**Issues, bug reports and discussion need no agreement.**

**If an employer or client owns your work**, do not sign the individual agreement for it:
the Corporate CLA that clause 5 mentions is not published yet. Open an issue first.

A CLA is a real cost to a project: plenty of capable people will not sign one, and we know
that. It is being adopted deliberately, not as boilerplate.

## Code of conduct

[CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) applies everywhere in this project.
