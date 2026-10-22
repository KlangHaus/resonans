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
//        — NO /usr/local/go-1.26.10, so `toolchains:` cannot select it
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
//    We ship 1.26.10 (Dockerfile FROM golang:1.26.10-alpine). Testing on the
//    agent default (1.27.0) would test a stdlib two minors away from the one
//    we release. The agent lacks 1.26.10, so `go` fetches exactly that
//    toolchain (GOTOOLCHAIN=go1.26.10 — an exact pin, not `auto`; the command
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
      selfInstalls: ['go': '1.26.10'],
      cmd: '''
        set -euo pipefail
        pinned="1.26.10"
        directive="$(awk '/^go [0-9]/ {print $2; exit}' go.mod)"
        if [ "$directive" != "$pinned" ]; then
          echo "go.mod says go ${directive:-<none>}, Jenkinsfile pins ${pinned} - change BOTH"
          exit 1
        fi
        # The fetched toolchain is only as trustworthy as its checksum check.
        # Asserted BEFORE the pin is exported: with GOTOOLCHAIN=go1.26.10 set,
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

        # -cover on the run CI already pays for. Measured that -race does not
        # change the percentages (identical for all 16 reporting packages), so the
        # floors in coverage-floors.json describe THIS run and not a neighbouring
        # one. `set -euo pipefail` at the top is what makes a failing go test red
        # through the pipe — without it the pipeline reports tee's status.
        go test -race -cover ./... 2>&1 | tee .ci-cover.log

        command -v python3 >/dev/null 2>&1 || { echo "python3 missing on the agent; the coverage gate cannot run"; exit 20; }

        # The gate's own cells first. A gate whose decision function is untested
        # is a threshold nobody has seen refuse anything.
        python3 scripts/coverage_gate.py --selftest

        # Same shape, different filter: `go test -run <pattern>` prints
        # `ok ... [no tests to run]` and exits 0 when the pattern matches
        # nothing, so a renamed test turns a targeted verification into a green
        # that measured nothing. This selftest provokes both halves (3 accept +
        # 3 reject) against the fixture module under scripts/testdata/, and
        # asserts each case's MESSAGE as well as its exit code: two of the
        # guard's layers are subsumed for the exit code, and only the message
        # tells them apart.
        python3 scripts/gotest_run_guard.py --selftest

        # Per-package floors, not a total: measured on d2928b2 the total sat at
        # 50.3% while internal/content had fallen nine points, because other
        # packages grew at the same time. An average cannot answer "did anything
        # get worse". Exit codes: 3 a package with no floor, 4 a floor with no
        # package, 5 a drop.
        python3 scripts/coverage_gate.py --decide < .ci-cover.log
      ''',
    ],

    // ─── gitleaks.yml · Secret scan ─────────────────────────────────────
    [
      // 🔴 NOT ON THE REQUIRED LIST, AND THAT IS THE POINT FOR NOW.
      // A required name that is never produced hangs a PR forever, and this check
      // has never run anywhere — the facility it needs (infra#395, merged
      // 2026-09-30T12:58Z) landed today. So it goes red and VISIBLE without
      // blocking, until it has produced a run on a real head. Flipping it to
      // required is a ruleset change and belongs to [infra] afterwards (KH-1220).
      //
      // WHY IT EXISTS: internal/persistence carries two cells behind
      // //go:build integration that need a real postgres — tenant isolation, and
      // migrate under a least-privilege role. Until today nothing could run them, so
      // two guards existed, passed by hand, and fired at nobody. That is the weakest
      // guard state there is: the capability exists, the cell exists, and no schedule
      // touches it.
      //
      // THIN ON PURPOSE, because this file must not grow: the per-build database is
      // created and dropped by the Go code (testPoolDSN), not here. The agents
      // deliberately have no docker ([sikkerhed]'s veto) and psql is not something
      // this repo may assume is on the image, so doing it in Go also means the same
      // code path runs on a laptop and the contract is measurable there.
      context: 'Tenant isolation',
      selfInstalls: ['go': '1.26.10'],
      cmd: '''
        set -euo pipefail

        # Fail closed and NAME the gap. An unset DSN here means the facility was
        # renamed or removed, and a check that quietly passes on no database is the
        # exact silence these cells were written to end.
        test -n "${KH_TEST_PG_DSN:-}" || {
          echo "KH_TEST_PG_DSN is unset — this check needs the CI postgres from"
          echo "infra compose/prod/ci (ci-postgres, agent env via jenkins.yaml)."
          echo "Refusing to pass: no database is a wiring error, not a green."
          exit 1
        }

        # Same toolchain as `Lint + Test` so a disagreement between the two is a
        # disagreement about postgres and not about a stdlib. The full checksum
        # assertion is NOT repeated here on purpose: it runs on this same commit in
        # `Lint + Test`, so a weakened GOSUMDB would already be red there, and
        # duplicating twelve lines is how two copies drift apart.
        export GOTOOLCHAIN=go1.26.10
        ver_out="$(go version)"
        echo "$ver_out"
        # 🔴 EXACT FIELD EQUALITY, NOT A REGEX, AND THAT IS THE FIX FOR A
        # COMPILE ERROR -- not a style preference. This line read
        # `grep -qE 'go1\\.25\\.13( |$)'`, and inside a Groovy
        # triple-quoted string a lone backslash is not a valid escape: the
        # Jenkinsfile did not COMPILE. Six builds failed in 0 seconds with an
        # empty output surface, and no jenkins/* check was ever planned,
        # because nothing ran.
        #
        # This file already held two correct examples of the same construct
        # (lines ~200 and ~323 double the backslashes) and one that avoids the
        # metacharacter entirely (~85). I wrote the only one that breaks. So
        # the cure here removes the metacharacter rather than remembering to
        # double it: `go version` prints the version as its THIRD field, so
        # comparing that field exactly is stronger than the regex was -- there
        # is no boundary question at all, where a loose `grep -F 'go1.26.9'`
        # measurably accepts go1.26.90 -- and it reports what it found
        # instead of only that it was wrong.
        ver_got="$(awk '{print $3}' <<<"$ver_out")"
        [ "$ver_got" = "go1.26.10" ] || {
          echo "go is $ver_got, expected exactly go1.26.10"; exit 1; }

        # The contract for this facility, stated in infra's jenkins.yaml: the role is
        # the cluster owner and may CREATE DATABASE, and each build must make its own
        # and drop it — two concurrent builds of this repo would otherwise share a
        # schema. BUILD_TAG is what makes the names unique, and deterministically so:
        # a retry reclaims its own leftovers instead of orphaning a new set.
        export RESONANS_TEST_DB_URL="$KH_TEST_PG_DSN"
        export RESONANS_TEST_SCOPE="$BUILD_TAG"

        # -count=1 so a cached result can never stand in for a run against the
        # database. Verbose, because the cells state what they measured — how many
        # tenant tables were checked, and 1 of 2 rows seen — and a bare "ok" would
        # throw away the denominator.
        go test -tags=integration -count=1 -v ./internal/persistence/
      ''',
    ],
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
