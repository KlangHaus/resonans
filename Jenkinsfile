// resonans · tynd pipeline mod KlangHaus/jenkins-library
//
// 🔴 DENNE FIL MAA IKKE VOKSE. Repo-specifik pipeline-logik hoerer i
//    biblioteket som en parametreret facilitet (PR til [quality]) — aldrig her.
//    Samme drift-vaern som taktart/Jenkinsfile.
//
// MAALT PAA AGENT-IMAGET FOER denne fil blev skrevet
// (infra: compose/prod/ci/agent/Dockerfile, origin/main):
//
//   go            1.27.0 (default-symlink) + 1.26.6 — INGEN 1.25.13
//   node · osv-scanner v2.5.1 · gitleaks 8.30.1 · sonar-scanner
//   gofumpt · golangci-lint · gosec · govulncheck   IKKE INSTALLERET
//   docker        BEVIDST udeladt (ingen image-smoke her)
//
// Derfor kun ET check, og kun det agenten faktisk kan koere. Lint, gosec,
// govulncheck og gofumpt bliver i Actions indtil vaerktoejerne findes paa
// agenten — et check der kalder et vaerktoej der ikke er der, bliver roedt af
// agentens mangel og ikke af vores kode. Det var praecis taktarts fund.
//
// 🔴 GOTOOLCHAIN er pinnet til go.mod-direktivet, ikke agentens default.
//    Vi shipper 1.25.13 (Dockerfile-FROM = golang:1.25.13-alpine, laast af
//    golangci-lint v2.5.0 som afviser et 1.26-direktiv — maalt: exit 3).
//    Agentens default er 1.27.0. Uden pin ville Jenkins teste mod en stdlib to
//    minor-versioner fra den vi udgiver — den divergens #34 lukkede, genopstaaet
//    i et andet CI-system. Go henter toolchainen; det koster et download pr.
//    kold cache og er prisen for at teste dét vi shipper.
@Library('klanghaus') _

klanghausPipeline(
  repo: 'KlangHaus/resonans',
  timeoutMinutes: 20,
  checksPrefix: 'jenkins/',
  checks: [
    [
      context: 'Lint + Test',
      cmd: '''
        set -euo pipefail
        export GOTOOLCHAIN="go$(awk '/^go [0-9]/ {print $2; exit}' go.mod)"
        echo "go.mod-direktiv -> GOTOOLCHAIN=${GOTOOLCHAIN}"
        go version
        go build ./...
        go vet ./...
        go test -race ./...
      ''',
    ],
  ],
)
