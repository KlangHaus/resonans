# Getting help

Resonans is maintained by a small team at KlangHaus, who run it in production for their own
sites. Help here is best effort: there is no support contract and no response-time promise.

## Where to go

| You have | Go to |
| --- | --- |
| A security problem | [Private vulnerability reporting](https://github.com/KlangHaus/resonans/security/advisories/new) — never a public issue. See [SECURITY.md](SECURITY.md). |
| Something broken | A [bug report](https://github.com/KlangHaus/resonans/issues/new?template=bug_report.yml). Include the `version` and `revision` from `GET /healthz`. |
| An idea or a missing feature | A [feature request](https://github.com/KlangHaus/resonans/issues/new?template=feature_request.yml). Describe the problem before the solution. |
| A question about running it | Read the [README](README.md) first — configuration, tenant isolation and asset storage are covered there. If it still is not answered, open an issue and say it is a question; a gap in the docs is a bug in the docs. |

## What to expect while Resonans is in beta

Resonans is **beta**: it runs our production content, but the API, the configuration and
the content model can still change in ways that need you to act. That is why the most useful
thing you can do before reporting an upgrade problem is to read [CHANGELOG.md](CHANGELOG.md)
for every version you skipped — operator actions are listed there under
**ACTION REQUIRED**.

## How we handle issues

We read every issue. We plan our own work in an internal tracker, so an issue you open may be
picked up there and closed here with a link to the commit or release that fixed it, rather
than staying open while we work. If we decide not to do something, we close it with the
reason.
