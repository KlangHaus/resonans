#!/usr/bin/env python3
"""Report when the pinned API-reference renderer has fallen behind.

WHY THIS EXISTS: choosing between vendoring a bundle and pinning one is
choosing between two ways to rot. [backstage] vendored @scalar/api-reference
1.72.2; measured 2026-10-02, dist-tags.latest was 1.72.4 -- two patches behind,
and nothing had noticed. A pin without a freshness check has the same defect as
a vendored blob, so this is the condition the pin ships with rather than an
extra.

THE PIN'S SINGLE SOURCE OF TRUTH is the Dockerfile's two ARGs. There is no
second file to drift against it, which is deliberate: a version written in two
places is a version that will disagree with itself.

EXIT CODES
  0  pinned == latest, or behind by a release too young to act on
  1  behind, and the newer release is older than --max-age days
  2  cannot measure (unreadable Dockerfile, malformed pin, registry unreachable)

🔴 The 2 matters more than the 1. A registry that cannot be reached must NOT
read as "up to date": that is the fail-open direction, and it is the one that
would make this guard useless exactly when the network is the reason nobody
noticed.

Every outcome states the date it measured, because "up to date" is a claim with
a timestamp on it.
"""

from __future__ import annotations

import argparse
import datetime as dt
import json
import re
import sys
import urllib.error
import urllib.request
from pathlib import Path

OK = 0
BEHIND = 1
CANNOT_MEASURE = 2

PACKAGE = "@scalar/api-reference"
REGISTRY = "https://registry.npmjs.org/" + PACKAGE.replace("/", "%2f")
SHA_RE = re.compile(r"^[0-9a-f]{64}$")


def read_pin(dockerfile: Path) -> tuple[str, str]:
    """Read SCALAR_VERSION and SCALAR_SHA256 from the Dockerfile.

    Raises on anything ambiguous. A guard that guesses which of two ARG lines
    is current would be asserting against a value nobody set.
    """
    text = dockerfile.read_text(encoding="utf-8")
    out = {}
    for key in ("SCALAR_VERSION", "SCALAR_SHA256"):
        found = re.findall(rf"^ARG {key}=(\S+)\s*$", text, re.M)
        if len(found) != 1:
            raise ValueError(f"{key}: found {len(found)} ARG lines, want exactly 1")
        out[key] = found[0]
    if not SHA_RE.match(out["SCALAR_SHA256"]):
        raise ValueError("SCALAR_SHA256 is not 64 lowercase hex characters")
    return out["SCALAR_VERSION"], out["SCALAR_SHA256"]


def _day(stamp: str) -> dt.date:
    return dt.datetime.fromisoformat(stamp.replace("Z", "+00:00")).date()


def fetch_registry(timeout: float = 15.0) -> tuple[str, dict[str, dt.date]]:
    """Return (latest version, {version: publish date}) from the npm registry."""
    with urllib.request.urlopen(REGISTRY, timeout=timeout) as r:  # noqa: S310
        doc = json.load(r)
    latest = doc["dist-tags"]["latest"]
    times = {v: _day(t) for v, t in doc.get("time", {}).items() if v[0].isdigit()}
    if latest not in times:
        raise ValueError(f"registry has no publish time for {latest}")
    return latest, times


def superseded_since(pinned: str, times: dict[str, dt.date]) -> dt.date | None:
    """The date the pin was first overtaken, or None if nothing newer exists.

    🔴 THIS IS THE OBJECT THE VERDICT IS ABOUT, and the first version of this
    guard measured the wrong one. It used the age of the NEWEST release, which
    for a package that ships every one to three days -- measured: 1.72.0 on
    2026-09-24 through 1.72.4 on 2026-10-02 -- is always a day or two old. So
    "latest is younger than max_age" was always true and the guard could never
    fail. It was armed against nothing.

    How long OUR PIN has been overtaken is a number that actually grows.
    """
    if pinned not in times:
        return None
    base = times[pinned]
    newer = [d for v, d in times.items() if d > base or (d == base and v > pinned)]
    return min(newer) if newer else None


def decide(
    pinned: str,
    latest: str,
    since: dt.date | None,
    today: dt.date,
    max_age: int,
):
    """The whole verdict, separated from the network so the selftest is hermetic.

    `since` is the date the pin was first overtaken -- NOT the latest release's
    date. See superseded_since for why that distinction is the guard's subject.
    """
    lines = [
        f"measured   {today.isoformat()}",
        f"package    {PACKAGE}",
        f"pinned     {pinned}",
        f"latest     {latest}",
    ]
    if pinned == latest or since is None:
        lines.append("verdict    up to date AS OF THE DATE ABOVE")
        return OK, lines
    age = (today - since).days
    lines.append(f"superseded {since.isoformat()} ({age} days ago)")
    if age < max_age:
        lines.append(
            f"verdict    behind for {age} days (< {max_age}): reporting only"
        )
        return OK, lines
    lines += [
        f"verdict    BEHIND for {age} days (>= {max_age})",
        "",
        "Bump SCALAR_VERSION and SCALAR_SHA256 in the Dockerfile together. The",
        "digest is the sha256 of the raw standalone.js, not of the gzip:",
        f"  curl -sS https://cdn.jsdelivr.net/npm/{PACKAGE}@{latest}/dist/browser/standalone.js \\",
        "    | shasum -a 256",
    ]
    return BEHIND, lines


# (pinned, latest, superseded-days-ago or None, expected code, a fragment the
# output must contain, why). Both halves, and the fragment is asserted because
# two different reasons produce exit 0 and only the message tells them apart.
SELFTEST_CASES = [
    ("1.72.4", "1.72.4", None, OK, "up to date", "pinned is latest"),
    ("1.72.4", "1.72.4", 400, OK, "up to date", "latest wins over a stale date"),
    ("1.72.2", "1.72.4", 3, OK, "reporting only", "overtaken, but only days ago"),
    ("1.72.2", "1.72.4", 30, BEHIND, "BEHIND for 30 days", "exactly at the threshold"),
    ("1.0.0", "1.72.4", 900, BEHIND, "BEHIND for 900 days", "far behind"),
    # 🔴 THE CASE THE FIRST VERSION OF THIS GUARD GOT WRONG. A package that
    # ships every two days always has a young LATEST release, so a verdict
    # built on that age could never fail. Here the newest release is a day old
    # and the pin has been overtaken for three months: the old logic returned
    # OK, this returns BEHIND.
    ("1.60.0", "1.72.4", 92, BEHIND, "BEHIND for 92 days", "young latest, long-overtaken pin"),
]


def selftest(max_age: int) -> int:
    oks = sum(1 for c in SELFTEST_CASES if c[3] == OK)
    bad_n = len(SELFTEST_CASES) - oks
    if oks == 0 or bad_n == 0:
        print(
            f"cannot measure: selftest needs both halves, has {oks} ok and {bad_n} behind",
            file=sys.stderr,
        )
        return CANNOT_MEASURE
    print(f"selftest: {oks} ok + {bad_n} behind case(s), max_age={max_age}")
    today = dt.date(2026, 10, 2)
    bad = 0
    for pinned, latest, days, want, fragment, why in SELFTEST_CASES:
        since = None if days is None else today - dt.timedelta(days=days)
        code, lines = decide(pinned, latest, since, today, max_age)
        blob = "\n".join(lines)
        ok = code == want and fragment in blob
        if not ok:
            bad += 1
        print(f"  {'ok  ' if ok else 'WRONG'} want={want} got={code}  {why}")
        if not ok:
            if fragment not in blob:
                print(f"        message lacks {fragment!r}")
            for ln in lines:
                print(f"        {ln}")
    print(f"selftest: {len(SELFTEST_CASES) - bad}/{len(SELFTEST_CASES)} as expected")
    return OK if bad == 0 else BEHIND


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--dockerfile", default="Dockerfile")
    ap.add_argument(
        "--max-age",
        type=int,
        default=30,
        help="days a newer release must have existed before this fails (default 30)",
    )
    ap.add_argument("--selftest", action="store_true")
    a = ap.parse_args()
    if a.selftest:
        return selftest(a.max_age)
    try:
        pinned, _ = read_pin(Path(a.dockerfile))
    except (OSError, ValueError) as e:
        print(f"cannot measure: {e}", file=sys.stderr)
        return CANNOT_MEASURE
    try:
        latest, times = fetch_registry()
    except (urllib.error.URLError, OSError, ValueError, KeyError) as e:
        print(f"cannot measure: registry unreachable or malformed: {e}", file=sys.stderr)
        return CANNOT_MEASURE
    if pinned not in times:
        print(
            f"cannot measure: the registry does not list the pinned version {pinned}",
            file=sys.stderr,
        )
        return CANNOT_MEASURE
    code, lines = decide(pinned, latest, superseded_since(pinned, times), dt.date.today(), a.max_age)
    for ln in lines:
        print(ln)
    return code


if __name__ == "__main__":
    sys.exit(main())
