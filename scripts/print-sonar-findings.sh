#!/usr/bin/env bash
# Prints SonarQube's findings for this PR into the job log.
#
# Why: the quality gate says "3 New Issues" without saying WHICH. The detail
# sits behind SONAR_TOKEN, which rightly only exists in CI — so the person who
# has to fix it is exactly the one who cannot see it. This step runs where the
# token legitimately lives and puts the findings where anyone who can read an
# Actions log can read them.
#
# Diagnostic, never blocking: the gate decides red/green. If THIS step fails,
# it must not look like a failed analysis — hence `exit 0` at the end.
#
# Ported from grundtone's scripts/print-sonar-findings.sh (develop d432bf2),
# translated; the logic is unchanged.
set -uo pipefail

: "${SONAR_HOST_URL:?}" "${SONAR_TOKEN:?}" "${SONAR_PROJECT_KEY:?}" "${PR_NUMBER:?}"

api() {
  curl -sS --max-time 30 -u "${SONAR_TOKEN}:" "${SONAR_HOST_URL}/api/$1" 2>/dev/null
}

echo "── SonarQube findings for PR #${PR_NUMBER} ──"

# 🔴 Wait until Sonar has PROCESSED this analysis before asking.
#
# The scanner uploads and returns; the server processes asynchronously. Asked
# at once, the API answers from the PREVIOUS analysis — and this step would
# print it as if it were this one. Measured in grundtone (PR #156): the step
# wrote "issues: 0" while the gate reported 1.
REPORT=".scannerwork/report-task.txt"
if [ -f "$REPORT" ]; then
  CE_TASK_ID=$(grep '^ceTaskId=' "$REPORT" | cut -d= -f2- | tr -d '\r')
  status=""
  for _ in $(seq 1 40); do
    status=$(api "ce/task?id=${CE_TASK_ID}" | jq -r '.task.status // empty' 2>/dev/null)
    case "$status" in
      SUCCESS) break ;;
      FAILED | CANCELED) break ;;
      *) sleep 3 ;;
    esac
  done
  if [ "$status" != "SUCCESS" ]; then
    # Absence of confirmation must not read as confirmation.
    echo "::warning::the analysis is not confirmed processed (status: ${status:-unknown}). The findings below may be from an EARLIER analysis."
  fi
else
  echo "::warning::${REPORT} does not exist — cannot bind the lookup to THIS analysis. The findings below may be stale."
fi

issues=$(api "issues/search?componentKeys=${SONAR_PROJECT_KEY}&pullRequest=${PR_NUMBER}&resolved=false&ps=100")
if [ -z "$issues" ] || ! echo "$issues" | jq -e .issues >/dev/null 2>&1; then
  echo "::warning::could not fetch issues — see the dashboard. This says NOTHING about whether there are findings."
else
  n=$(echo "$issues" | jq '.issues | length')
  echo "issues: $n"
  echo "$issues" | jq -r '.issues[] |
    "  \(.severity) \(.type) \(.rule)\n    \(.component | split(":") | last):\(.line // "?")\n    \(.message)"'
fi

# 🔴 This branch must say something whatever happens. A silent skip when the
# lookup failed, next to a gate message about "0.00% Security Hotspots
# Reviewed", is exactly the state this step exists to remove.
hotspots=$(api "hotspots/search?projectKey=${SONAR_PROJECT_KEY}&pullRequest=${PR_NUMBER}&ps=100")
if [ -z "$hotspots" ]; then
  echo "::warning::the hotspot lookup gave no answer — see the dashboard. This says NOTHING about whether there are hotspots."
elif ! echo "$hotspots" | jq -e .hotspots >/dev/null 2>&1; then
  echo "::warning::the hotspot lookup could not be read. Sonar answered:"
  echo "$hotspots" | head -c 500
  echo
else
  n=$(echo "$hotspots" | jq '.hotspots | length')
  echo "security hotspots: $n  (need a manual review in Sonar's UI — cannot be closed from here)"
  echo "$hotspots" | jq -r '.hotspots[] |
    "  \(.vulnerabilityProbability) \(.securityCategory) \(.status)\n    \(.component | split(":") | last):\(.line // "?")\n    \(.message)"'
fi

exit 0
