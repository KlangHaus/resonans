#!/usr/bin/env python3
"""Release gate: does the required check that verifies THIS commit say success?

Until phase C the release chain hung on `workflow_run` of the Actions workflow
named 'CI (Go)': semantic-release ran only after that workflow completed
successfully. Deleting ci-go.yml would therefore stop releases SILENTLY — no red
check, no failed run, just an event that never fires again. This script replaces
that coupling with a measurement: semantic-release now runs on a push to main
and asks, before it cuts anything, whether the required check is green ON THIS
EXACT SHA. Since 2026-10-22 that is `Lint + Test` from GitHub Actions (app
15368); before, it was `jenkins/Lint + Test` from the Jenkins app.

Fail-closed by construction. Every way of not knowing is its own exit code, and
none of them is 0:

    0  SUCCESS   the latest run of the named check, from the named app, on this
                 sha, concluded success
    3  ABSENT    no such check-run exists (and the timeout has passed)
    4  PENDING   it exists but has not concluded (timeout passed)
    5  NOT-GREEN it concluded, but not as success
    6  ERROR     the instrument itself failed (HTTP, JSON, missing token)

ABSENT is deliberately NOT folded into PENDING: "CI never built this commit"
is a different world from "CI is still building it", and the fleet
has measured the first one happening (a push whose webhook was lost, riff
od1se2ro). Both stop the release, but the message must say which.

The decision is a pure function of the check-runs JSON, so it can be exercised
from fixtures without a token or a network — see --selftest.
"""

import argparse
import json
import os
import sys
import time
import urllib.error
import urllib.request

SUCCESS, ABSENT, PENDING, NOT_GREEN, ERROR = 0, 3, 4, 5, 6
NAMES = {SUCCESS: "SUCCESS", ABSENT: "ABSENT", PENDING: "PENDING", NOT_GREEN: "NOT-GREEN", ERROR: "ERROR"}


def decide(payload, check, app_id):
    """Return (code, message) for one /check-runs response.

    The app id is part of the identity on purpose: a check-run NAME is not a
    key. Anyone can publish a check-run called 'Lint + Test'; only GitHub
    Actions (app 15368) has the id the ruleset pins, so the gate asks for both.
    """
    runs = payload.get("check_runs")
    if runs is None:
        return ERROR, "response has no check_runs array"
    mine = [r for r in runs if r.get("name") == check and (r.get("app") or {}).get("id") == app_id]
    if not mine:
        others = sorted({r.get("name", "?") for r in runs})
        return ABSENT, (
            f"no check-run named {check!r} from app {app_id} on this sha "
            f"({len(runs)} check-runs present: {', '.join(others) if others else 'none'})"
        )
    # Latest run per name — a re-run adds a NEW check-run under the same name,
    # so the oldest one would answer about a build nobody is looking at.
    latest = max(mine, key=lambda r: (r.get("started_at") or "", r.get("id") or 0))
    conclusion = latest.get("conclusion")
    where = f"(run id {latest.get('id')}, started {latest.get('started_at')}, {len(mine)} run(s) under this name)"
    if conclusion is None:
        return PENDING, f"{check!r} has not concluded yet: status={latest.get('status')!r} {where}"
    if conclusion == "success":
        return SUCCESS, f"{check!r} is success {where}"
    return NOT_GREEN, f"{check!r} concluded {conclusion!r} {where}"


def fetch(repo, sha, token):
    url = f"https://api.github.com/repos/{repo}/commits/{sha}/check-runs?per_page=100"
    req = urllib.request.Request(
        url,
        headers={
            "Accept": "application/vnd.github+json",
            "Authorization": f"Bearer {token}",
            "X-GitHub-Api-Version": "2022-11-28",
            "User-Agent": "resonans-release-gate",
        },
    )
    with urllib.request.urlopen(req, timeout=30) as r:
        return json.loads(r.read())


def selftest(directory):
    """Run the fixture table. The expected exit code is in the file name:
    `<case>.<code>.json`. A table that matches nothing must FAIL rather than
    report success on an empty denominator."""
    files = sorted(f for f in os.listdir(directory) if f.endswith(".json"))
    if not files:
        print(f"release-gate selftest: NO FIXTURES in {directory} — refusing to report green on 0 cases")
        return 1
    check, app_id, bad = "Lint + Test", 15368, 0
    for name in files:
        want = int(name.rsplit(".", 2)[1])
        with open(os.path.join(directory, name)) as fh:
            payload = json.load(fh)
        got, msg = decide(payload, check, app_id)
        ok = "ok " if got == want else "FAIL"
        if got != want:
            bad += 1
        print(f"  {ok} {name:<34} want={NAMES.get(want, want)} got={NAMES.get(got, got)} :: {msg}")
    print(f"release-gate selftest: {len(files)} cases, {bad} failed")
    return 1 if bad else 0


def main():
    p = argparse.ArgumentParser()
    p.add_argument("--repo", default=os.environ.get("GITHUB_REPOSITORY", ""))
    p.add_argument("--sha", default=os.environ.get("GITHUB_SHA", ""))
    p.add_argument("--check", default="Lint + Test")
    p.add_argument("--app-id", type=int, default=15368)
    p.add_argument("--timeout-seconds", type=int, default=1800)
    p.add_argument("--poll-seconds", type=int, default=20)
    p.add_argument("--decide", action="store_true", help="read one check-runs JSON on stdin and decide")
    p.add_argument("--selftest", metavar="DIR", nargs="?", const="scripts/testdata/release-gate")
    a = p.parse_args()

    if a.selftest:
        sys.exit(selftest(a.selftest))

    if a.decide:
        code, msg = decide(json.load(sys.stdin), a.check, a.app_id)
        print(f"release-gate: {NAMES[code]} — {msg}")
        sys.exit(code)

    token = os.environ.get("GITHUB_TOKEN", "")
    if not (a.repo and a.sha and token):
        missing = [n for n, v in (("--repo", a.repo), ("--sha", a.sha), ("GITHUB_TOKEN", token)) if not v]
        print(f"release-gate: ERROR — missing {', '.join(missing)}")
        sys.exit(ERROR)

    deadline = time.time() + a.timeout_seconds
    code, msg = ERROR, "no attempt was made"
    while True:
        try:
            code, msg = decide(fetch(a.repo, a.sha, token), a.check, a.app_id)
        except (urllib.error.URLError, json.JSONDecodeError, TimeoutError) as e:
            # A broken instrument is never a verdict about the world: keep
            # trying while there is time, then fail as ERROR, not as ABSENT.
            code, msg = ERROR, f"could not read check-runs: {e.__class__.__name__}: {e}"
        print(f"release-gate: {NAMES[code]} — {msg}", flush=True)
        if code in (SUCCESS, NOT_GREEN):
            break
        if time.time() >= deadline:
            print(f"release-gate: giving up after {a.timeout_seconds}s in state {NAMES[code]}")
            break
        time.sleep(a.poll_seconds)
    sys.exit(code)


if __name__ == "__main__":
    main()
