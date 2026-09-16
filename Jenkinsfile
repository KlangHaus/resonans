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
// Only ONE check. Lint, gosec, govulncheck and gofumpt stay in Actions until
// resonans moves to go 1.27 (golangci-lint v2.5.0 is go1.25-built and refuses
// a newer directive — measured: exit 3), which is a separate change.
//
// 🔴 WHY GO IS SELF-INSTALLED, AND DECLARED AS SUCH
//    We ship 1.25.13 (Dockerfile FROM golang:1.25.13-alpine). Testing on the
//    agent default (1.27.0) would test a stdlib two minors away from the one
//    we release. The agent lacks 1.25.13, so `go` fetches exactly that
//    toolchain (GOTOOLCHAIN=go1.25.13 — an exact pin, not `auto`; the download
//    is verified against the Go checksum database).
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
        export GOTOOLCHAIN="go${pinned}"
        ver_out="$(go version)"
        echo "$ver_out"
        grep -qE "go${pinned}( |$)" <<<"$ver_out" || { echo "go is NOT ${pinned}"; exit 1; }
        go build ./...
        go vet ./...
        go test -race ./...
      ''',
    ],
  ],
)
