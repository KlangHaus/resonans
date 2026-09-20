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

    // ─── release-gaten (semantic-release.yml) · celle-tabel ─────────────
    [
      // 🔴 DETTE CHECK ER BEVISET FOR EN GATE DER IKKE KAN VISES I DRIFT.
      //    Release-gaten kører kun på et push til main, altså EFTER merge —
      //    en PR kan derfor aldrig vise at den virker. Det den KAN vise, er at
      //    beslutningen er rigtig: tabellen kører den rene beslutningsfunktion
      //    mod ti fixturer og fejler, hvis blot én celle svarer forkert.
      //
      //    Nævneren er med vilje en del af udfaldet: en tom fixture-mappe
      //    FEJLER frem for at melde grønt på nul celler.
      //
      //    Celler: grøn · fraværende · tomt svar · uafsluttet · fejlet ·
      //    `skipped` (som GitHub ellers tæller som opfyldt) · check-run fra en
      //    FREMMED app med samme navn · genkørsel hvor den nyeste er rød ·
      //    genkørsel hvor den nyeste er grøn · misdannet svar.
      //    Muteret tre gange lokalt (skipped=success · app-id-filteret fjernet
      //    · første run frem for nyeste) → 1, 1 og 2 celler røde; umuteret grøn.
      context: 'release gate',
      cmd: '''
        set -euo pipefail
        command -v python3 >/dev/null 2>&1 || { echo "python3 mangler paa agenten"; exit 10; }
        python3 scripts/release_gate.py --selftest scripts/testdata/release-gate
      ''',
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
      // treats an EMPTY merge base as an error rather than a result.
      needsMergeBase: true,
      // Actions runs `wagoid/commitlint-github-action@v6`, which bundles its own
      // copies. [permissions] measured them in the action's COMMITTED
      // package-lock.json: @commitlint/lint and @commitlint/config-conventional
      // are 19.1.0. config-conventional carries the RULES, hence the verdicts.
      //
      // 🔴 THE VERSIONS ARE NOW PINNED BY A LOCKFILE, NOT BY TWO ARGUMENTS.
      //    `npm install @commitlint/cli@19.8.1 …` pinned the two direct
      //    versions and let everything else float: measured 104 packages with
      //    118 floating dependency ranges against 4 exact ones. Two installs in
      //    the same minute produced identical trees — which says the registry
      //    answers consistently right now, not that a run next month will.
      //    Stability over minutes is not a pin.
      //    `package-lock.json` pins all 121 entries with a sha512 integrity
      //    each, and `npm ci` REFUSES to run when package.json and the lockfile
      //    disagree (measured: EUSAGE, exit 1) — so the lockfile is a gate
      //    rather than decoration. `--ignore-scripts` makes "no code runs at
      //    install time" structural: today 8 packages declare `prepare` (which
      //    npm does not run for registry tarballs) and 0 declare postinstall,
      //    but that is a property of today's tree, not a guarantee.
      //
      //    Parity caveat, named rather than hidden, and bigger than "the CLI
      //    wrapper differs": the Actions side runs the wagoid action's own
      //    bundled tree, and measured in our lockfile vs the action's,
      //    @commitlint/LINT is 19.8.1 here against 19.1.0 there — that is the
      //    RULE ENGINE, not just the wrapper. config-conventional is 19.1.0 on
      //    both sides, so the difference is probably without effect; "probably"
      //    is the honest word until someone measures a verdict that differs.
      //    This change makes OUR half deterministic, not the two halves
      //    identical. Full parity arrives when the Actions side goes away.
      //
      //    Measured on the lockfile (by [sikkerhed] on the artefact, re-measured
      //    by me): 122 packages entries, 121 with a sha512 (the root "" entry
      //    has none by definition), 0 with a weaker hash, 0 resolved outside
      //    registry.npmjs.org, and `hasInstallScript: true` on 0 of 121 — npm's
      //    OWN marking of exactly what --ignore-scripts protects against, which
      //    is a better object than counting `prepare` in the manifests. It is
      //    still a property of today's tree; the flag is what makes it a
      //    property of the command.
      selfInstalls: ['@commitlint/cli': '19.8.1', '@commitlint/config-conventional': '19.1.0'],
      cmd: '''
        set -euo pipefail
        # Diagnosis by exit code, because the check-run summary shows only
        # `kode N`. Each state is its own code, and none of them is 0:
        #   10 = node/npm missing on the agent
        #   11 = npm ci failed for another reason (network/registry)
        #   12 = wrong commitlint version after install
        #   13 = KH_MERGE_BASE empty despite needsMergeBase
        #   14 = package.json and package-lock.json disagree (EUSAGE)
        #   15 = the lockfile does not pin config-conventional 19.1.0
        #    1 = commitlint found actual violations  <- the ONLY real finding
        mkdir -p .ci-tools
        command -v node >/dev/null 2>&1 || exit 10
        command -v npm  >/dev/null 2>&1 || exit 10
        [ -n "${KH_MERGE_BASE:-}" ] || exit 13
        [ -f package-lock.json ] || { echo "package-lock.json mangler — pinnet kan ikke haandhaeves"; exit 14; }

        # 🔴 ASSERT THE LOCKFILE, NOT THE ARGUMENTS. The rules live in
        # config-conventional, so that is the version worth gating on — and the
        # lockfile is the object npm actually installs from. One line, no
        # heredoc: an indented heredoc body would reach python3 with its
        # indentation and die on IndentationError, which is a failure about the
        # quoting and not about the lockfile.
        lock_ver="$(python3 -c "import json;print((json.load(open('package-lock.json')).get('packages',{}).get('node_modules/@commitlint/config-conventional',{}) or {}).get('version',''))")"
        [ "$lock_ver" = "19.1.0" ] || { echo "lockfilen pinner config-conventional '$lock_ver', forventet 19.1.0"; exit 15; }

        if ! npm ci --ignore-scripts --no-audit --no-fund > .ci-tools/npmci.log 2>&1; then
          # EUSAGE is npm's own error CODE for a lockfile that disagrees with
          # package.json — a stabler signal than its sentence, and a different
          # world from a registry that is down.
          if grep -q 'EUSAGE' .ci-tools/npmci.log; then
            echo "package.json og package-lock.json er ikke i sync (npm EUSAGE)"; tail -3 .ci-tools/npmci.log; exit 14
          fi
          echo "npm ci fejlede"; tail -5 .ci-tools/npmci.log; exit 11
        fi
        ver_out="$(./node_modules/.bin/commitlint --version 2>&1 || true)"
        grep -qE '(^|[^0-9.])19\\.8\\.1($|[^0-9.])' <<<"$ver_out" || { echo "commitlint er IKKE 19.8.1: $ver_out"; exit 12; }

        ./node_modules/.bin/commitlint --from "$KH_MERGE_BASE" --to HEAD --config commitlint.config.mjs
      ''',
    ],
  ],
)
