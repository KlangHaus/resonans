<!--
Thanks for the patch. Until the contributor licence agreement exists, a pull request from
outside KlangHaus will wait rather than be merged — CONTRIBUTING.md, "Licensing of
contributions", says why. It is still worth opening: the discussion is not wasted.
-->

## Why

<!-- What was wrong or missing before. The diff shows what changed; this is the part a
reader needs in a year. Link the issue if there is one. -->

## What changed

## How it was verified

<!-- Commands you ran and what they printed. A test that would have FAILED before this
change is worth more than one that passes after it. -->

- [ ] `go build ./... && go vet ./... && go test -race ./...`
- [ ] `gofumpt -l .` prints nothing, and `golangci-lint run` + `gosec ./...` are clean
- [ ] No new lint suppressions

## Operator impact

- [ ] None
- [ ] Yes — and the commit carries an `ACTION REQUIRED:` footer saying what to do (a commit body never reaches the release notes)
