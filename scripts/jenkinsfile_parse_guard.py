#!/usr/bin/env python3
r"""Parse the Jenkinsfile as Groovy before pushing it.

WHY THIS EXISTS, measured 2026-10-03 on resonans#88: the Jenkinsfile did not
COMPILE. A lone backslash is not a valid escape inside a Groovy triple-quoted
string, so the script failed in parseScript -- before any stage existed, before
an agent was allocated, before an environment variable could be read.

    WorkflowScript: unexpected char: '\' @ line 165, column 22
       grep -qE 'go1\.25\.13( |$)' <<<"$ver_out" || { ... }

Six builds failed that way. Every symptom followed from it: the aggregate ran
0 SECONDS, its title, summary and text were 0 bytes, and 0 jenkins/* checks
were ever planned -- so the REQUIRED check was absent rather than red, and an
absent required name hangs a pull request forever.

🔴 AND THE SAME CLASS BIT ME AGAIN WRITING THIS FILE, in a third language:
quoting that broken shell in a non-raw Python docstring produced
 on every run. Python only WARNS where
Groovy ERRORS, which is why it survived -- a warning in a guard's own output
trains people to ignore its output. The docstring is raw now. Three languages,
one rule: a lone backslash in a string literal is the layer below talking.

🔴 WHY `bash -n` DID NOT CATCH IT, which is the whole point of this file. The
Jenkinsfile had been checked with `bash -n` on every extracted `cmd` block, and
all of them were valid. That validates the shell INSIDE the string and is blind
to the string itself not parsing as Groovy. Two layers, two parsers, and only
the inner one was being run. A triple-quoted Groovy string looks like shell and
is not.

HOW: a Groovy CompilationUnit to Phases.CONVERSION -- parse only, no symbol
resolution, so the pipeline DSL's unknown methods do not matter. That is the
same phase whose failure Jenkins reports as "startup failed".

EXIT CODES
  0  parses
  1  does not parse (the message names the line)
  2  cannot measure -- groovy is not on PATH, or a file is missing

🔴 The 2 matters. A machine without `groovy` must NOT read as a pass: that is
the fail-open direction, and it is the one that would make this guard useless
exactly on the machine where nobody checked.
"""

from __future__ import annotations

import argparse
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

OK = 0
PARSE_FAILED = 1
CANNOT_MEASURE = 2

FIXTURES = Path("scripts/testdata/jenkinsfile-parse-guard")

# Written to a temp file at run time rather than kept as a second repository
# file: one source, and nothing to drift against this docstring.
PARSE_ONLY = """
import org.codehaus.groovy.control.CompilationUnit
import org.codehaus.groovy.control.Phases

def f = new File(args[0])
def cu = new CompilationUnit()
cu.addSource(f)
try {
  cu.compile(Phases.CONVERSION)
  println "PARSE OK"
  System.exit(0)
} catch (Throwable t) {
  println "PARSE FAILED"
  t.message.readLines().findAll { it.trim() }.take(6).each { println "  " + it }
  System.exit(1)
}
"""


def parse(path: Path, program: Path) -> tuple[bool, str]:
    """Return (parses, output). Raises nothing: callers distinguish 1 from 2."""
    p = subprocess.run(
        ["groovy", str(program), str(path)], capture_output=True, text=True
    )
    return p.returncode == 0, (p.stdout + p.stderr).strip()


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("files", nargs="*", default=None, help="default: Jenkinsfile")
    ap.add_argument("--selftest", action="store_true")
    a = ap.parse_args()

    if not shutil.which("groovy"):
        print(
            "cannot measure: `groovy` is not on PATH, so the Jenkinsfile's OUTER\n"
            "layer is unchecked. Install Groovy (brew install groovy) -- do not\n"
            "treat this as a pass: `bash -n` only validates the shell inside the\n"
            "triple-quoted strings.",
            file=sys.stderr,
        )
        return CANNOT_MEASURE

    with tempfile.TemporaryDirectory() as tmp:
        program = Path(tmp) / "ParseOnly.groovy"
        program.write_text(PARSE_ONLY, encoding="utf-8")

        if a.selftest:
            cases = [("broken.groovy", False), ("valid.groovy", True)]
            if sum(1 for _, want in cases if want) == 0 or sum(
                1 for _, want in cases if not want
            ) == 0:
                print("cannot measure: the selftest needs both halves", file=sys.stderr)
                return CANNOT_MEASURE
            print(f"selftest: {len(cases)} case(s), both halves")
            bad = 0
            for name, want_ok in cases:
                f = FIXTURES / name
                if not f.is_file():
                    print(f"cannot measure: fixture {f} is missing", file=sys.stderr)
                    return CANNOT_MEASURE
                got_ok, out = parse(f, program)
                ok = got_ok == want_ok
                if not ok:
                    bad += 1
                print(f"  {'ok  ' if ok else 'WRONG'} want={'OK' if want_ok else 'FAIL'} "
                      f"got={'OK' if got_ok else 'FAIL'}  {name}")
                if not ok:
                    for line in out.splitlines():
                        print(f"        {line}")
            print(f"selftest: {len(cases) - bad}/{len(cases)} as expected")
            return OK if bad == 0 else PARSE_FAILED

        targets = [Path(f) for f in (a.files or ["Jenkinsfile"])]
        failed = 0
        for t in targets:
            if not t.is_file():
                print(f"cannot measure: {t} is missing", file=sys.stderr)
                return CANNOT_MEASURE
            ok, out = parse(t, program)
            print(f"{'PARSE OK  ' if ok else 'PARSE FAIL'}  {t}")
            if not ok:
                failed += 1
                for line in out.splitlines():
                    print(f"  {line}")
        return OK if failed == 0 else PARSE_FAILED


if __name__ == "__main__":
    sys.exit(main())
