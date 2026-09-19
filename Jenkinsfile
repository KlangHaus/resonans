// resonans · thin pipeline against KlangHaus/jenkins-library
//
// 🔴 THIS FILE MUST NOT GROW. Repo-specific pipeline logic belongs in the
//    library as a parameterised facility (PR to [quality]) — never here.
//    Same drift guard as taktart/Jenkinsfile.
//
// MEASURED ON THE AGENT IMAGE (infra: compose/prod/ci/agent/Dockerfile,
// origin/main e1397db):
//
//   go   /usr/local/go-1.27.0 (default symlink) + /usr/local/go-1.26.6
//        — NO /usr/local/go-1.25.13, so `toolchains:` cannot select it
//
// THREE checks — the three names resonans' ruleset requires. Lint, gosec,
// govulncheck and gofumpt stay in Actions until resonans moves to go 1.27
// (golangci-lint v2.5.0 is go1.25-built and refuses a newer directive —
// measured: exit 3), which is a separate change.
//
// 🔴 PARALLEL MODE, AND NOTHING IS SWITCHED OFF BY THIS FILE.
//    `checksPrefix: 'jenkins/'` means every name below is published as
//    `jenkins/<name>` and is NOT required. Actions keeps producing the three
//    required names (`Lint + Test`, `Secret scan`, `commitlint` — measured on
//    ruleset `force-push-guard` 20711390). The order is A -> B -> C:
//      A (this file): Jenkins produces the same three checks in parallel.
//      B ([infra]):   the required list is flipped 1:1 to `jenkins/<name>`
//                     @4785909. resonans is a RULESET repo, so it waits for
//                     the whole-object PUT fix.
//      C (later PR):  only THEN may ci-go.yml / gitleaks.yml / commitlint.yml
//                     be deleted.
//
// 🔴 WHY C IS A SEPARATE PR, AND WHY THIS ONE DOES NOT DELETE ANYTHING.
//    `ci-go.yml` is not only three checks: it is the TRIGGER for the whole
//    release chain (`semantic-release.yml` runs on `workflow_run` of the
//    workflow NAMED 'CI (Go)', which cuts the tag and calls `release.yml`,
//    which builds and signs the images). Delete it, and semantic-release never
//    fires again — with no red check, no failed run, nothing: releases just
//    stop. That is the same silent-event class `release.yml`'s own comment
//    documents (eight tags produced zero semver images). C therefore has to
//    rewire the trigger to `push: main` + a fail-closed gate that asserts
//    `jenkins/Lint + Test` is success on `github.sha`. Measured that the signal
//    exists where such a gate would read it: that check-run is SUCCESS on main
//    commit 435ed42, not only on PR heads.
//
// 🔴 WHY GO IS SELF-INSTALLED, AND DECLARED AS SUCH
//    We ship 1.25.13 (Dockerfile FROM golang:1.25.13-alpine). Testing on the
//    agent default (1.27.0) would test a stdlib two minors away from the one
//    we release. The agent lacks 1.25.13, so `go` fetches exactly that
//    toolchain (GOTOOLCHAIN=go1.25.13 — an exact pin, not `auto`; the command
//    asserts the download is verified against the Go checksum database).
//    The first version of this file did that SILENTLY, and the library
//    refused it: "PARALLEL mode without a `tools:` declaration" — so the check
//    never ran on any commit. `selfInstalls` makes the fetch a declaration the
//    library verifies against the command text, and the command asserts that
//    go.mod and the pin agree, so bumping one without the other goes red here.
@Library('klanghaus') _

klanghausPipeline(
  repo: 'KlangHaus/resonans',
  timeoutMinutes: 20,
  checksPrefix: 'jenkins/',
  checks: [
    [
      context: 'Lint + Test',
      selfInstalls: ['go': '1.25.13'],
      cmd: '''
        set -euo pipefail
        pinned="1.25.13"
        directive="$(awk '/^go [0-9]/ {print $2; exit}' go.mod)"
        if [ "$directive" != "$pinned" ]; then
          echo "go.mod says go ${directive:-<none>}, Jenkinsfile pins ${pinned} - change BOTH"
          exit 1
        fi
        # The fetched toolchain is only as trustworthy as its checksum check.
        # Asserted BEFORE the pin is exported: with GOTOOLCHAIN=go1.25.13 set,
        # `go env` itself would trigger the download it is meant to guard.
        # GONOSUMCHECK is read from the environment - `go env` does not report it.
        sumdb="$(GOTOOLCHAIN=local go env GOSUMDB)"
        bypass="$(GOTOOLCHAIN=local go env GONOSUMDB GOINSECURE | tr -d '\\n')${GONOSUMCHECK:-}"
        if [ "$sumdb" != "sum.golang.org" ] || [ -n "$bypass" ]; then
          echo "checksum verification is weakened: GOSUMDB=${sumdb} bypass=${bypass}"
          exit 1
        fi
        export GOTOOLCHAIN="go${pinned}"
        ver_out="$(go version)"
        echo "$ver_out"
        grep -qE "go${pinned}( |$)" <<<"$ver_out" || { echo "go is NOT ${pinned}"; exit 1; }
        go build ./...
        go vet ./...
        go test -race ./...
      ''',
    ],

    // ─── gitleaks.yml · Secret scan ─────────────────────────────────────
    [
      // 🔴 DEPENDS ON FULL HISTORY. A shallow clone does not make gitleaks
      //    FAIL — it makes it find fewer secrets and report green. The library
      //    measures the clone (`git rev-parse --is-shallow-repository`).
      context: 'Secret scan',
      // Same 8.21.2 as .github/workflows/gitleaks.yml, deliberately frozen:
      // in parallel both sides must measure THE SAME thing, or a version
      // disagreement is read as a porting disagreement. The agent image
      // carries gitleaks 8.30.1 on PATH (infra compose/prod/ci/agent/
      // Dockerfile), so the pinned copy goes in .ci-tools and the assertion
      // below refuses to run with the agent's newer one. Raising BOTH sides
      // belongs after the flip, with time set aside for the noise.
      selfInstalls: ['gitleaks': '8.21.2'],
      cmd: '''
        set -euo pipefail
        mkdir -p .ci-tools
        # sha256 of gitleaks_8.21.2_linux_x64.tar.gz, measured by [account]
        # 2026-09-17 (account#148) against the release's checksums.txt. Pinned
        # HERE rather than fetched at run time: a checksum from the same origin
        # as the tarball would move together with a swapped tarball. The
        # Actions side of THIS repo does not verify it — that is a difference
        # in our favour, not a difference in what is scanned.
        sha256=5bc41815076e6ed6ef8fbecc9d9b75bcae31f39029ceb55da08086315316e3ba
        curl -sSfL -o .ci-tools/gitleaks.tgz \
          https://github.com/gitleaks/gitleaks/releases/download/v8.21.2/gitleaks_8.21.2_linux_x64.tar.gz
        echo "${sha256}  .ci-tools/gitleaks.tgz" | sha256sum --check --strict -
        tar -xzf .ci-tools/gitleaks.tgz -C .ci-tools gitleaks
        ver_out="$(.ci-tools/gitleaks version 2>&1 || true)"
        grep -qE '(^|[^0-9.])8\\.21\\.2($|[^0-9.])' <<<"$ver_out" || {
          echo "gitleaks is NOT 8.21.2 (agent PATH carries 8.30.1) - a disagreement would be unattributable"
          printf '%s\\n' "$ver_out" | head -2
          exit 1
        }
        # Character for character the Actions step, so a finding on one side is
        # a finding on the other.
        .ci-tools/gitleaks detect --redact --verbose --exit-code 1
      ''',
      when: { ctx -> ctx.isPR },
    ],

    // ─── commitlint.yml · commitlint ────────────────────────────────────
    [
      context: 'commitlint',
      // 🔴 PR-ONLY: the workflow is `pull_request:` alone. On a push build
      //    KH_BASE_REF is empty and `--from ''` would lint the whole history —
      //    green or red for the wrong reason.
      when: { ctx -> ctx.isPR },
      // The merge base comes from the library's own facility: it binds the
      // checkout credential, deepens the clone until the histories meet, and
      // treats an EMPTY merge base as an error rather than a result. After a
      // shallow fetch `origin/main` exists while the merge base is empty, and
      // commitlint would then lint a different interval — green without
      // measuring the branch's commits.
      needsMergeBase: true,
      // Actions runs `wagoid/commitlint-github-action@v6`, which bundles its
      // own copies. [permissions] measured them in the action's COMMITTED
      // package-lock.json (not guessed from its `^19.1.0` range):
      //     @commitlint/lint                = 19.1.0
      //     @commitlint/config-conventional = 19.1.0
      // config-conventional carries the RULES, hence the verdicts, so it is
      // pinned to 19.1.0. The CLI wrapper is a different package from the
      // action's direct `lint()` call; that difference is named rather than
      // hidden. Same pins as notifications and permissions.
      selfInstalls: ['@commitlint/cli': '19.8.1', '@commitlint/config-conventional': '19.1.0'],
      cmd: '''
        set -euo pipefail
        # 🔴 `npx --package` DOES NOT WORK here, and the failure looks like a
        # lint finding: commitlint.config.mjs has
        # `extends: ['@commitlint/config-conventional']`, and extends resolves
        # from the WORKING DIRECTORY. With npx the package sits in npx' own temp
        # dir, so the lookup fails with exit 1 — the same code a real violation
        # gives. Install into the workspace so extends resolves from there.
        #
        # Diagnosis by exit code, because the check-run summary shows only
        # `kode N`:
        #   10 = node/npm missing on the agent
        #   11 = npm install failed (network/registry)
        #   12 = wrong commitlint version
        #   13 = KH_MERGE_BASE empty despite needsMergeBase
        #    1 = commitlint found actual violations  <- the ONLY real finding
        mkdir -p .ci-tools
        command -v node >/dev/null 2>&1 || exit 10
        command -v npm  >/dev/null 2>&1 || exit 10
        [ -n "${KH_MERGE_BASE:-}" ] || exit 13

        npm install --no-save --no-audit --no-fund --silent \
          @commitlint/cli@19.8.1 @commitlint/config-conventional@19.1.0 || exit 11
        ./node_modules/.bin/commitlint --version > .ci-tools/cl.txt 2>&1 || exit 11
        grep -qE '(^|[^0-9.])19\\.8\\.1($|[^0-9.])' .ci-tools/cl.txt || exit 12

        ./node_modules/.bin/commitlint --from "$KH_MERGE_BASE" --to HEAD --config commitlint.config.mjs
      ''',
    ],
  ],
)
