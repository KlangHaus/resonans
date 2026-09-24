#!/usr/bin/env python3
"""Ratchet the per-package test coverage of this repository.

Why per-package and not a total. Measured on main d2928b2: the total sat at
50.3% while internal/content had fallen from 43.9% to 34.7% — nine points of
regression that a total-only gate reports as no change, because other packages
grew at the same time. A total is an average, and an average is exactly the wrong
instrument for "did anything get worse".

Exit codes, so a caller can tell the four failures apart:

    0  OK        every package is at or above its floor
    3  MISSING   a package exists in the tree with no floor recorded. A new
                 package must be admitted on purpose; otherwise an untested one
                 arrives at 0% and the gate says nothing.
    4  STALE     a floor names a package that no longer exists. A file that
                 describes a tree from last month is not a gate.
    5  BELOW     a package dropped below its floor
    6  ERROR     the input could not be read or parsed

Floors live in coverage-floors.json and are the MEASURED value rounded DOWN to a
whole percent. That gives up to one point of natural headroom, so a refactor that
shifts a statement does not fail the build. The cost is named rather than hidden:
a real regression smaller than the distance to the next whole percent passes.
Raise a floor when coverage improves — the ratchet only tightens if someone
tightens it.
"""

import argparse
import json
import math
import os
import re
import subprocess
import sys

OK, MISSING, STALE, BELOW, ERROR = 0, 3, 4, 5, 6
NAMES = {OK: "OK", MISSING: "MISSING", STALE: "STALE", BELOW: "BELOW", ERROR: "ERROR"}

# `go test -cover` prints one line per package.
_COVER = re.compile(r"^(?:ok|---\s+FAIL)?\s*(\S+)\s+.*coverage:\s+([0-9.]+)% of statements")
_NO_COVER = re.compile(r"^\?\s+(\S+)\s+\[no test files\]")


def parse_go_cover(text):
    """Extract {package: percent} from `go test -cover ./...` output.

    A package with no test files reports "[no test files]" and no percentage. It
    is recorded as 0.0 rather than skipped: leaving it out would let a package
    with no tests at all be invisible to the MISSING check, which is the one that
    catches a new untested package.
    """
    out = {}
    for line in text.splitlines():
        m = _COVER.match(line.strip())
        if m:
            out[m.group(1)] = float(m.group(2))
            continue
        m = _NO_COVER.match(line.strip())
        if m:
            out.setdefault(m.group(1), 0.0)
    return out


def decide(measured, floors):
    """Pure decision. Returns (exit_code, [messages])."""
    if not isinstance(measured, dict) or not isinstance(floors, dict):
        return ERROR, ["measured and floors must both be objects"]
    if not measured:
        # An empty measurement is the shape a broken `go test` invocation has,
        # and it would otherwise satisfy every floor vacuously.
        return ERROR, ["no package coverage was parsed at all — the measurement, not the code, is the thing that failed"]

    msgs = []
    unknown = sorted(set(measured) - set(floors))
    gone = sorted(set(floors) - set(measured))
    below = []
    for pkg in sorted(measured):
        if pkg not in floors:
            continue
        if measured[pkg] + 1e-9 < floors[pkg]:
            below.append((pkg, measured[pkg], floors[pkg]))

    for pkg, got, want in below:
        msgs.append(f"BELOW  {pkg}: {got:.1f}% < floor {want:.1f}%")
    for pkg in unknown:
        msgs.append(f"MISSING  {pkg}: {measured[pkg]:.1f}% measured, no floor recorded — add one on purpose")
    for pkg in gone:
        msgs.append(f"STALE  {pkg}: floor {floors[pkg]:.1f}% recorded, package not in the tree")

    msgs.append(f"checked {len(measured)} packages against {len(floors)} floors")

    if below:
        return BELOW, msgs
    if unknown:
        return MISSING, msgs
    if gone:
        return STALE, msgs
    return OK, msgs


def measure(packages="./..."):
    """Run the repository's own test command and parse its coverage."""
    proc = subprocess.run(
        ["go", "test", "-cover", packages],
        capture_output=True, text=True, check=False,
    )
    text = proc.stdout + proc.stderr
    if proc.returncode != 0:
        # Do not report a coverage verdict about a run that failed: the numbers
        # would be about whichever packages happened to finish.
        return None, text
    return parse_go_cover(text), text


def load_floors(path):
    with open(path, encoding="utf-8") as fh:
        doc = json.load(fh)
    return {k: float(v) for k, v in doc.get("floors", {}).items()}


def selftest(directory):
    """Each fixture encodes its expected exit code in its name: name.<code>.json."""
    files = sorted(f for f in os.listdir(directory) if f.endswith(".json"))
    if not files:
        # An empty fixture directory would make this pass while testing nothing.
        print(f"no fixtures in {directory} — a selftest over zero cases is not a selftest")
        return 1
    bad = 0
    for name in files:
        want = int(name.split(".")[-2])
        with open(os.path.join(directory, name), encoding="utf-8") as fh:
            case = json.load(fh)
        code, msgs = decide(case.get("measured"), case.get("floors"))
        status = "ok  " if code == want else "FAIL"
        if code != want:
            bad += 1
        print(f"{status} {name}: got {NAMES[code]}({code}), want {NAMES[want]}({want})")
        if code != want:
            for m in msgs:
                print(f"       {m}")
    print(f"{len(files) - bad} of {len(files)} fixtures agreed")
    return 1 if bad else 0


def main():
    p = argparse.ArgumentParser()
    p.add_argument("--floors", default="coverage-floors.json")
    p.add_argument("--packages", default="./...")
    p.add_argument("--decide", action="store_true", help="read `go test -cover` output on stdin instead of running it")
    p.add_argument("--selftest", metavar="DIR", nargs="?", const="scripts/testdata/coverage-gate")
    p.add_argument("--print-floors", action="store_true", help="print the floors the current tree would justify")
    a = p.parse_args()

    if a.selftest:
        sys.exit(selftest(a.selftest))

    if a.decide:
        measured = parse_go_cover(sys.stdin.read())
        log = ""
    else:
        measured, log = measure(a.packages)
        if measured is None:
            print("`go test -cover` failed; refusing to judge coverage of a failed run", file=sys.stderr)
            print(log[-2000:], file=sys.stderr)
            sys.exit(ERROR)

    if a.print_floors:
        print(json.dumps({"floors": {k: math.floor(v) for k, v in sorted(measured.items())}}, indent=2))
        sys.exit(OK)

    try:
        floors = load_floors(a.floors)
    except (OSError, ValueError) as exc:
        print(f"cannot read {a.floors}: {exc}", file=sys.stderr)
        sys.exit(ERROR)

    code, msgs = decide(measured, floors)
    for m in msgs:
        print(m)
    print(f"coverage gate: {NAMES[code]}")
    sys.exit(code)


if __name__ == "__main__":
    main()
