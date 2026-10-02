#!/usr/bin/env python3
"""Make `go test -run <pattern>` fail when the pattern measured nothing.

WHY THIS EXISTS, measured 2026-10-02 on this repo: `go test -run` with a
pattern that matches no test prints `ok ... [no tests to run]` and exits 0.
A lint fix had renamed a test (Unlabelled -> Unlabeled), so a targeted
verification selected ZERO tests and read as a pass. The rename was correct,
the lint fix was correct, and the check disappeared as a side effect without
anything turning red.

    go test -run TestDoesNotExist ./internal/schema/
    -> ok  github.com/.../internal/schema  0.312s [no tests to run]   exit 0

A spelling `sed` silently disarms every standing `-run` pattern.

WHAT IT CHECKS, and each layer exists because a measurement showed the layer
above it is blind:

  1. `go test -list <first segment>` must name at least one test. This layer
     is DIAGNOSIS, not detection: measured by mutation, removing it leaves the
     selftest 6/6, because layer 2 already fails every case it would. It is
     logically subsumed -- if nothing is listed, nothing runs, so top_passes
     is 0 anyway. What it adds is telling "that name does not exist" apart
     from "the name exists and was skipped", which changes what the reader
     does next. The selftest therefore asserts each case's MESSAGE, not only
     its exit code; without that, this layer and NAME_LINE had no cell.
     Only the FIRST `/`-separated segment is listed, because -list does not
     see subtests: measured, `-list 'Parent//delivery/v1/schema'` returns 0
     names for a subtest that -run does run.
  2. At least one top-level `--- PASS`. Catches "the name matched but nothing
     ran", e.g. the test skips.
  3. If the pattern names a subtest, at least one INDENTED `--- PASS`.
     Catches the case layer 1 cannot see: measured, a pattern whose parent
     exists and whose subtest segment matches nothing gives 1 top-level PASS
     and 0 indented ones -- the parent passes while nothing was verified.

FIELD OF VIEW. This sees Go tests invoked through this script. It does NOT
see: a `go test -run` you type directly (that is the point of using this
instead), committed patterns elsewhere in the repo (measured 2026-10-02:
there are none -- every `go test` in CI runs `./...` with no filter), and
other languages' filters. `pytest -k`, `vitest -t` and `jest -t` have the
same shape and are NOT covered by this script.

Caching is not a false-fail source: measured, a cached run still replays its
`--- PASS` lines under -v. This passes -count=1 anyway.

Exit codes: 0 ok, 1 assertion failed (or the tests themselves failed),
2 cannot measure.
"""

from __future__ import annotations

import argparse
import re
import subprocess
import sys
from pathlib import Path

OK = 0
FAILED = 1
CANNOT_MEASURE = 2

# A -list line that names something runnable, as `go test -list` prints it.
NAME_LINE = re.compile(r"^(Test|Benchmark|Fuzz|Example)\S*$")
TOP_PASS = re.compile(r"^--- PASS", re.MULTILINE)
SUB_PASS = re.compile(r"^ +--- PASS", re.MULTILINE)


def run(args: list[str], cwd: Path | None) -> tuple[int, str]:
    try:
        p = subprocess.run(args, cwd=cwd, capture_output=True, text=True)
    except FileNotFoundError:
        return -1, f"{args[0]} not found"
    return p.returncode, p.stdout + p.stderr


def measure(pattern: str, packages: list[str], cwd: Path | None) -> dict:
    """Collect every number the verdict is made of, so a caller can print them."""
    top_segment = pattern.split("/")[0]
    code, listing = run(["go", "test", "-list", top_segment, *packages], cwd)
    if code < 0:
        return {"error": listing}
    listed = [ln for ln in listing.splitlines() if NAME_LINE.match(ln.strip())]
    test_code, output = run(
        ["go", "test", "-count=1", "-v", "-run", pattern, *packages], cwd
    )
    if test_code < 0:
        return {"error": output}
    return {
        "top_segment": top_segment,
        "listed": listed,
        "test_code": test_code,
        "output": output,
        "top_passes": len(TOP_PASS.findall(output)),
        "sub_passes": len(SUB_PASS.findall(output)),
        "names_a_subtest": "/" in pattern,
    }


def verdict(pattern: str, m: dict) -> tuple[int, list[str]]:
    """Return (exit code, lines to print). Every line states a number."""
    if "error" in m:
        return CANNOT_MEASURE, [f"cannot measure: {m['error']}"]
    out = [
        f"pattern   {pattern!r}",
        f"listed    {len(m['listed'])} name(s) for first segment {m['top_segment']!r}"
        + (f": {', '.join(m['listed'][:5])}" if m["listed"] else ""),
        f"passes    {m['top_passes']} top-level, {m['sub_passes']} subtest",
    ]
    if not m["listed"]:
        out += [
            "",
            f"FAILED: no test matches {m['top_segment']!r}, so this pattern measured",
            "nothing and `go test` would still exit 0. Read the names from the file",
            "instead of from memory:  grep -h '^func Test' <file>_test.go",
        ]
        return FAILED, out
    if m["test_code"] != 0:
        out += ["", f"tests FAILED (go test exit {m['test_code']}) -- not a pattern problem"]
        return FAILED, out
    if m["top_passes"] == 0:
        out += [
            "",
            f"FAILED: {len(m['listed'])} name(s) matched but 0 passed, so nothing was",
            "measured. A skipped test selects fine and verifies nothing.",
        ]
        return FAILED, out
    if m["names_a_subtest"] and m["sub_passes"] == 0:
        out += [
            "",
            "FAILED: the pattern names a subtest, but 0 subtests ran. The parent",
            "passed, which is why `go test` is green and `-list` cannot see this.",
        ]
        return FAILED, out
    return OK, out


# (pattern, expected exit, a fragment the verdict MUST contain, why).
# The fragment is what gives layer 1 and NAME_LINE a cell: both are subsumed
# by layer 2 for the exit code, so only the message distinguishes them.
SELFTEST_CASES = [
    ("TestGuardFixtureTop", OK, "listed    1 name", "plain top-level name"),
    ("TestGuardFixtureWithSubtests", OK, "1 top-level, 2 subtest", "parent with subtests"),
    ("TestGuardFixtureWithSubtests/alpha", OK, "1 top-level, 1 subtest", "subtest -list cannot see"),
    ("TestGuardFixtureDoesNotExist", FAILED, "no test matches", "renamed or mistyped name"),
    ("TestGuardFixtureWithSubtests/nosuchsub", FAILED, "names a subtest", "parent passes, subtest matched nothing"),
    ("TestGuardFixtureSkips", FAILED, "matched but 0 passed", "name matches but nothing runs"),
]


def selftest(fixture: Path) -> int:
    """Provoke BOTH sides. The accept set is not optional: a guard that failed
    everything would pass a reject-only selftest while blocking all real use."""
    if not (fixture / "go.mod").is_file():
        print(f"cannot measure: no fixture module at {fixture}", file=sys.stderr)
        return CANNOT_MEASURE
    accepts = sum(1 for _, want, _, _ in SELFTEST_CASES if want == OK)
    rejects = len(SELFTEST_CASES) - accepts
    if accepts == 0 or rejects == 0:
        print(
            f"cannot measure: selftest needs both halves, has {accepts} accept"
            f" and {rejects} reject case(s)",
            file=sys.stderr,
        )
        return CANNOT_MEASURE
    print(f"selftest: {accepts} accept + {rejects} reject case(s)")
    bad = 0
    for pattern, want, fragment, why in SELFTEST_CASES:
        got, lines = verdict(pattern, measure(pattern, ["./..."], fixture))
        blob = "\n".join(lines)
        code_ok = got == want
        # The message is asserted too: a verdict that is right for the wrong
        # reason reads identically on the exit code alone.
        msg_ok = fragment in blob
        mark = "ok  " if code_ok and msg_ok else "WRONG"
        if not (code_ok and msg_ok):
            bad += 1
        print(f"  {mark} want={want} got={got}  {pattern}  ({why})")
        if not code_ok or not msg_ok:
            if not msg_ok:
                print(f"        message lacks {fragment!r}")
            for ln in lines:
                print(f"        {ln}")
    print(f"selftest: {len(SELFTEST_CASES) - bad}/{len(SELFTEST_CASES)} as expected")
    return OK if bad == 0 else FAILED


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("pattern", nargs="?", help="the -run pattern to verify and run")
    ap.add_argument("packages", nargs="*", default=None, help="packages (default ./...)")
    ap.add_argument(
        "--selftest",
        nargs="?",
        const="scripts/testdata/gotest-run-guard",
        metavar="FIXTURE",
        help="provoke both sides against the fixture module and exit",
    )
    a = ap.parse_args()
    if a.selftest:
        return selftest(Path(a.selftest))
    if not a.pattern:
        ap.error("a pattern is required (or --selftest)")
    packages = a.packages or ["./..."]
    m = measure(a.pattern, packages, None)
    code, lines = verdict(a.pattern, m)
    if "output" in m:
        print(m["output"], end="")
    for ln in lines:
        print(ln)
    return code


if __name__ == "__main__":
    sys.exit(main())
