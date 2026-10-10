#!/usr/bin/env python3
"""Tripwire: the Syft version that anchore/sbom-action bundles must not move silently.

The action pin already determines the SBOM generator - the version is a
compile-time constant in the action's own bundle, not resolved at run time. That
makes it invisible: resonans#115 moved Syft v1.42.3 -> v1.54.0 and the diff
showed a commit SHA and a version comment, nothing else.

This check reads the pin out of release.yml, fetches the bundle at that exact
SHA, extracts the constant the action actually uses, and compares it with a value
recorded in this repository. The recorded value is ONLY ever compared against; it
is never fed to the action, so the action stays the single source of truth.

Every failure has its own message and its own exit code, because "could not read
the bundle" and "the generator moved" are different findings and a check that
cannot tell them apart reports a false cause.
"""

from __future__ import annotations

import argparse
import re
import sys
import urllib.error
import urllib.request

# Distinct exit paths - never collapse these into one "failed".
OK = 0
EXIT_PIN_COUNT = 10  # release.yml does not hold exactly the expected pins
EXIT_UNREADABLE = 11  # bundle could not be fetched, or came back empty
EXIT_NO_CONSTANT = 12  # bundle fetched, but the constant could not be located
EXIT_MOVED = 13  # constant read, and it differs from the recorded value
EXIT_CONTROL = 14  # the positive control on the base revision failed
EXIT_NO_RECORD = 15  # the recorded value is missing or unreadable

PIN_RE = re.compile(r"anchore/sbom-action@([0-9a-f]{40})")
RAW = "https://raw.githubusercontent.com/anchore/sbom-action/{sha}/dist/index.cjs"

# How many call sites release.yml is expected to hold. A denominator, not a
# convenience: without it the check silently inspects only the first pin the day
# somebody adds a third job.
EXPECTED_PIN_COUNT = 2

# The bundle is ~3.4 MB. A floor of "more than zero bytes" is not enough: a
# raw.githubusercontent 404 body is 14 bytes of "404: Not Found", and if that
# ever arrived with a 200 it would clear a zero-floor, fail the constant lookup,
# and get reported as "the action restructured" - a wrong cause for a bad fetch.
# State the instrument's floor as a number instead.
MIN_PLAUSIBLE_BUNDLE = 1_000_000


def fail(code: int, label: str, *lines: str) -> int:
    print(f"::error title=SBOM generator pin::{label}")
    print(f"  {label}")
    for line in lines:
        print(f"  {line}")
    return code


def read_pins(path: str) -> tuple[list[str], str | None]:
    try:
        with open(path, encoding="utf-8") as handle:
            text = handle.read()
    except FileNotFoundError:
        return [], "missing"
    return PIN_RE.findall(text), None


def fetch_bundle(sha: str) -> tuple[bytes | None, str]:
    url = RAW.format(sha=sha)
    try:
        with urllib.request.urlopen(url, timeout=60) as response:
            return response.read(), url
    except (urllib.error.URLError, urllib.error.HTTPError, TimeoutError) as exc:
        print(f"  fetch failed: {exc}")
        return None, url


def extract_version(bundle: str) -> tuple[str | None, str | None]:
    """Find the constant the action ACTUALLY uses, not a neighbouring one.

    esbuild renames these constants between builds, so matching `VERSION<n>` by
    name would be reading whichever constant happens to sit nearby. Go through
    the use site instead: getSyftVersion() returns the input or this constant.
    """
    use = re.search(r'getInput\("syft-version"\)\s*\|\|\s*([A-Za-z_$][\w$]*)', bundle)
    if not use:
        return None, None
    name = use.group(1)
    decl = re.search(r"\b(?:var|const|let)\s+" + re.escape(name) + r'\s*=\s*"([^"]+)"', bundle)
    if not decl:
        return None, name
    return decl.group(1), name


def measure(sha: str, what: str, bundle_file: str | None = None) -> tuple[str | None, int]:
    """Returns (version, exit_code). Prints the byte count before parsing.

    bundle_file exists so each failure state can be PROVOKED locally rather than
    assumed to work; the workflow never passes it.
    """
    if bundle_file:
        print(f"  {what}: reading local bundle {bundle_file} (test path, not used in CI)")
        with open(bundle_file, "rb") as handle:
            raw, url = handle.read(), bundle_file
    else:
        raw, url = fetch_bundle(sha)
    if raw is None:
        return None, EXIT_UNREADABLE
    size = len(raw)
    print(f"  {what}: bundle {size} bytes  <- {url}")
    if size < MIN_PLAUSIBLE_BUNDLE:
        # The contents API answers null for files over 1 MB; an empty or tiny
        # read then looks exactly like a small file. Hence the raw route and a
        # floor that is a number, not just "non-empty".
        print(
            f"  bundle is implausibly small ({size} bytes < {MIN_PLAUSIBLE_BUNDLE}) -"
            " that is an instrument failure, not a finding"
        )
        return None, EXIT_UNREADABLE
    version, name = extract_version(raw.decode("utf-8", errors="replace"))
    if version is None:
        print(f"  constant not located (use site {'found: ' + name if name else 'not found'})")
        return None, EXIT_NO_CONSTANT
    print(f"  {what}: {name} = {version}")
    return version, OK


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--release-yml", default=".github/workflows/release.yml")
    parser.add_argument("--expected-file", default=".github/sbom-generator-expected.txt")
    parser.add_argument("--base-release-yml", help="release.yml from the base revision")
    parser.add_argument("--base-expected-file", help="recorded value from the base revision")
    parser.add_argument(
        "--bundle-file",
        help="read the bundle from this local file instead of fetching it; test path for"
        " provoking each failure state, never passed by the workflow",
    )
    args = parser.parse_args()

    pins, state = read_pins(args.release_yml)
    if state == "missing":
        # Named, explicit, green. The trigger carries no paths filter on
        # purpose: a required check that is never produced hangs a PR forever.
        print(f"NOT APPLICABLE: {args.release_yml} does not exist in this tree - nothing to check")
        return OK

    print(f"pins found in {args.release_yml}: {len(pins)} (expected {EXPECTED_PIN_COUNT})")
    for pin in pins:
        print(f"  {pin}")
    if len(pins) != EXPECTED_PIN_COUNT:
        return fail(
            EXIT_PIN_COUNT,
            f"WRONG NUMBER OF PINS: found {len(pins)}, expected {EXPECTED_PIN_COUNT}",
            "A changed number of call sites means this check no longer covers what it claims.",
            f"If the change is intended, move EXPECTED_PIN_COUNT in {__file__} with it.",
        )
    if len(set(pins)) != 1:
        return fail(
            EXIT_PIN_COUNT,
            f"CALL SITES DISAGREE: {sorted(set(pins))}",
            "Both jobs must pin the same action revision, or they build SBOMs with different generators.",
        )
    pin = pins[0]

    try:
        with open(args.expected_file, encoding="utf-8") as handle:
            expected = handle.read().strip()
    except FileNotFoundError:
        return fail(
            EXIT_NO_RECORD,
            f"RECORDED VALUE MISSING: {args.expected_file} does not exist",
            "This check compares the action's bundled Syft version against that file.",
        )
    if not expected:
        return fail(EXIT_NO_RECORD, f"RECORDED VALUE EMPTY: {args.expected_file} holds nothing")
    print(f"recorded value ({args.expected_file}): {expected}")

    # Positive control first, on the base revision, so a broken extractor cannot
    # masquerade as a clean pass or as this PR's fault.
    if args.base_release_yml and args.base_expected_file:
        base_pins, base_state = read_pins(args.base_release_yml)
        if base_state == "missing" or not base_pins:
            print("positive control: base revision holds no pin - skipped, stated explicitly")
        else:
            try:
                with open(args.base_expected_file, encoding="utf-8") as handle:
                    base_expected = handle.read().strip()
            except FileNotFoundError:
                base_expected = ""
            print(f"positive control: base pin {base_pins[0]}, base recorded {base_expected or '(none)'}")
            base_version, code = measure(base_pins[0], "positive control")
            if code != OK:
                return fail(
                    EXIT_CONTROL,
                    "POSITIVE CONTROL COULD NOT BE READ on the base revision",
                    "The extractor itself is suspect, so this run cannot certify the pin.",
                )
            if not base_expected:
                # Say what was NOT measured. Printing "they agree" here would be
                # a sentence about a comparison that never happened - the first
                # canary run caught exactly that, on the run that introduces the
                # file, when the base does not carry it yet.
                print(
                    "positive control LIMITED: the base revision carries no recorded value, so"
                    f" nothing was compared. All this establishes is that the extractor read"
                    f" {base_version} from the base pin."
                )
            elif base_version != base_expected:
                return fail(
                    EXIT_CONTROL,
                    f"POSITIVE CONTROL FAILED: base pin yields {base_version}, base record says {base_expected}",
                    "The base revision is already inconsistent, so a mismatch here is not this PR's doing.",
                )
            else:
                print(
                    f"positive control: base pin yields {base_version} and the base record says"
                    f" {base_expected} - they agree"
                )
    else:
        print("positive control: no base revision supplied - skipped, stated explicitly")

    version, code = measure(pin, "this tree", args.bundle_file)
    if code == EXIT_UNREADABLE:
        return fail(
            EXIT_UNREADABLE,
            "COULD NOT READ THE BUNDLE - this is NOT a version mismatch",
            f"pin: {pin}",
            "Nothing is claimed about the generator. Re-run; if it persists, the SHA may not exist.",
        )
    if code == EXIT_NO_CONSTANT:
        return fail(
            EXIT_NO_CONSTANT,
            "BUNDLE READ, BUT THE SYFT CONSTANT WAS NOT FOUND - this is NOT a version mismatch",
            f"pin: {pin}",
            "The action may have restructured how it selects Syft. Read the bundle before trusting this check again.",
        )
    if version != expected:
        return fail(
            EXIT_MOVED,
            f"THE SBOM GENERATOR MOVED: bundle says {version}, this repo records {expected}",
            f"pin: {pin}",
            "This is the finding this check exists for: the action pin changed the tool that",
            "produces the document we attest, and the diff would otherwise show only a SHA.",
            f"If the move is intended, verify the new generator and then move the value in",
            f"{args.expected_file} deliberately, in this PR, naming both numbers in the review.",
        )

    print(f"PASS: bundle at {pin} yields {version}, which matches the recorded value")
    return OK


if __name__ == "__main__":
    sys.exit(main())
