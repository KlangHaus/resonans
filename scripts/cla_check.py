#!/usr/bin/env python3
"""CLA check: may this pull request be merged under the contributor agreement?

The signature model ([jura], docs 52cbb84, legal/cla-resonans-udkast.md §C.3):
a contributor signs by adding ONE line to CLA.md in a pull request FROM THEIR OWN
ACCOUNT. The pull request and the commit they authored are the evidence; the file
is only the index. So the decisive rule is not the line's format but WHO added it:
the line's numeric user ID must be the pull request author's. A line a maintainer
adds for someone else proves nothing, and this check refuses it.

Three kinds of pull request:

  SIGNATURE   it changes CLA.md. It must change ONLY CLA.md, the file may only
              grow by exactly one table row, and that row must name the author
              (login and numeric ID), the current agreement version and the SHA of
              the commit that last changed the agreement, a date not in the future,
              and an ID that has not signed before.
  CORPORATE   it changes legal/corporate-signatures.md, the Corporate CLA's
              register. It must change ONLY that file, and only by inserting
              lines: a company's signature line, or a person's row under one.
              The company signs, not the person, so the individual rule (the
              line names the author) cannot apply here. This check verifies the
              FORM only; that the signed agreement exists is a maintainer's
              review, and the check says so in its pass message.
  CODE        anything else. The author's ID must be on the BASE branch in
              CLA.md, or in a person row of legal/corporate-signatures.md, or in
              legal/cla-exemptions.json with a reason.

Two registers and one exemption list, each meaning ONE thing ([jura], docs
b527055): the registers say "an agreement EXISTS", the exemption list says "NO
agreement is needed". A person covered by a Corporate CLA is in the corporate
register, never in the exemption list: an exemption whose reason is "there is an
agreement" would record the opposite of the truth.

Exemption is by NAME, never by role ([jura], 2026-10-28). A GitHub role grants
access, not rights in the work: nobody at KlangHaus is employed, so no employer
rule covers them, and a friend with collaborator access or a freelancer is covered
by nothing at all. A bot is exempt only when it is listed, and it is listed only
when its changes are machine-generated — a bot that lands a patch a person wrote
would otherwise bypass the agreement.

Exit codes, so a caller can tell the outcomes apart:

    0  PASS            may be merged as far as the agreement is concerned
    3  NOT-SIGNED      a code pull request from someone who has not signed
    4  BAD-SIGNATURE   a signature pull request that does not prove what it claims
    6  ERROR           the instrument failed (input missing or unreadable)

🔴 TRUST. The workflow runs this on `pull_request_target`, from the BASE branch:
the pull request's own code is never checked out or run, so a contributor cannot
edit this script or its workflow to pass. The pull request's CLA.md and file list
are read through the API as DATA, bound to its head SHA. Text from the pull
request is printed with repr(), never raw, so it cannot inject workflow commands
into the log.

The decision is a pure function, exercised from fixtures without a token or a
network — see --selftest.
"""

import argparse
import base64
import datetime
import json
import os
import re
import subprocess
import sys
import urllib.request

PASS, NOT_SIGNED, BAD_SIGNATURE, ERROR = 0, 3, 4, 6
NAMES = {PASS: "PASS", NOT_SIGNED: "NOT-SIGNED", BAD_SIGNATURE: "BAD-SIGNATURE", ERROR: "ERROR"}

INDEX = "CLA.md"
AGREEMENT = "legal/individual-cla.md"
EXEMPTIONS = "legal/cla-exemptions.json"
CORPORATE = "legal/corporate-signatures.md"
CORPORATE_AGREEMENT = "legal/corporate-cla.md"

# | @login | 1234567 | 2026-10-28 | cla-1.0 (a1b2c3d) |
ROW = re.compile(
    r"^\|\s*@(?P<login>[A-Za-z0-9](?:[A-Za-z0-9-]{0,38}))\s*"
    r"\|\s*(?P<id>[0-9]{1,12})\s*"
    r"\|\s*(?P<date>[0-9]{4}-[0-9]{2}-[0-9]{2})\s*"
    r"\|\s*(?P<version>cla-[0-9]+\.[0-9]+)\s*\(\s*(?P<sha>[0-9a-f]{7,40})\s*\)\s*\|\s*$"
)


def signed_ids(index_text):
    """The numeric IDs that have signed, from the index's table rows."""
    return {m.group("id") for m in (ROW.match(l) for l in index_text.splitlines()) if m}


# ## Example ApS · CVR 12345678 · Jane Doe, CEO · 2026-10-28 · ccla-1.0
COMPANY = re.compile(
    r"^## (?P<company>[^·|]+?) · (?P<number>[^·|]+?) · (?P<signatory>[^·|]+?) · "
    r"(?P<date>[0-9]{4}-[0-9]{2}-[0-9]{2}) · (?P<version>ccla-[0-9]+\.[0-9]+)\s*$"
)
# | @login | 1234567 |
PERSON = re.compile(r"^\|\s*@(?P<login>[A-Za-z0-9](?:[A-Za-z0-9-]{0,38}))\s*\|\s*(?P<id>[0-9]{1,12})\s*\|\s*$")
TABLE_FRAME = re.compile(r"^\|\s*GitHub\s*\|\s*User ID\s*\|\s*$|^\|-+\|-+\|\s*$")


def corporate_ids(text):
    """The numeric IDs in person rows of the corporate register."""
    return {m.group("id") for m in (PERSON.match(l) for l in text.splitlines()) if m}


def inserted_lines(base, head):
    """The lines head adds to base, or None when head also changes or removes a
    base line: every base line must survive, in order."""
    b, added, i = base.splitlines(), [], 0
    for line in head.splitlines():
        if i < len(b) and line == b[i]:
            i += 1
        else:
            added.append(line)
    return added if i == len(b) else None


def check_corporate(pr):
    """A corporate-register pull request: insertions only, each a company line or
    a person row under one, current ccla version, no date in the future."""
    added = inserted_lines(pr["base_corporate"], pr["head_corporate"])
    if added is None:
        return BAD_SIGNATURE, f"{CORPORATE} may only grow: an existing line was changed or removed"
    added = [l for l in added if l.strip()]
    seen, under_company = set(), False
    for line in pr["head_corporate"].splitlines():
        c, m = COMPANY.match(line), PERSON.match(line)
        if line.startswith("## "):
            if not c:
                return BAD_SIGNATURE, f"not '## company · number · signatory, role · YYYY-MM-DD · ccla-X.Y': {line!r}"
            under_company = True
            try:
                date = datetime.date.fromisoformat(c.group("date"))
            except ValueError:
                return BAD_SIGNATURE, f"{c.group('date')!r} is not a date"
            if line in added and date > pr["today"] + datetime.timedelta(days=1):
                return BAD_SIGNATURE, f"the date {date} is in the future"
            if line in added and c.group("version") != pr["corporate_version"]:
                return BAD_SIGNATURE, f"{c.group('version')!r} is not the current Corporate CLA, {pr['corporate_version']!r}"
        elif m:
            if not under_company:
                return BAD_SIGNATURE, f"a person row before any company line: {line!r}"
            if m.group("id") in seen:
                return BAD_SIGNATURE, f"id {m.group('id')} is listed twice in {CORPORATE}"
            seen.add(m.group("id"))
        elif line in added and not TABLE_FRAME.match(line):
            return BAD_SIGNATURE, f"the added line is neither a company line nor a person row: {line!r}"
    if not any(PERSON.match(l) or COMPANY.match(l) for l in added):
        return BAD_SIGNATURE, f"the change adds no company line and no person row to {CORPORATE}"
    return PASS, (
        f"{CORPORATE}: the form is valid. NOT verified here: that the company signed {pr['corporate_version']} "
        "— a maintainer checks the signed agreement before merging."
    )


def exemption_reasons(entries):
    """{id: reason} from the exemption list, or raise ValueError when an entry
    lacks an ID or a reason: an unexplained exemption is the thing this list
    exists to prevent, so a malformed list is an instrument failure, not a pass."""
    out = {}
    for e in entries:
        uid, reason = str(e.get("id", "")), str(e.get("reason", "")).strip()
        if not uid.isdigit() or not reason:
            raise ValueError(f"exemption entry without a numeric id and a reason: {e!r}")
        out[uid] = reason
    return out


def decide(pr):
    """Pure decision. `pr` holds: author {login, id},
    changed_files, base_index, head_index, version, version_sha, today, and
    exemptions (the base branch's list). Returns (exit_code, message)."""
    author = pr["author"]
    login, uid = author["login"], str(author["id"])

    if CORPORATE in pr["changed_files"]:
        others = sorted(f for f in pr["changed_files"] if f != CORPORATE)
        if others:
            return BAD_SIGNATURE, f"a corporate signature changes only {CORPORATE}; this one also changes {others!r}"
        return check_corporate(pr)

    if INDEX not in pr["changed_files"]:
        try:
            exempt = exemption_reasons(pr["exemptions"])
        except ValueError as e:
            return ERROR, f"{EXEMPTIONS}: {e}"
        if uid in exempt:
            return PASS, f"exempt: {login!r} (id {uid}) is listed in {EXEMPTIONS}: {exempt[uid]}"
        if uid in signed_ids(pr["base_index"]):
            return PASS, f"{login!r} (id {uid}) has signed"
        if uid in corporate_ids(pr["base_corporate"]):
            return PASS, f"{login!r} (id {uid}) is covered by a Corporate CLA in {CORPORATE}"
        return NOT_SIGNED, (
            f"{login!r} (id {uid}) has not signed the contributor agreement. "
            "See CONTRIBUTING.md → Licensing of contributions; signing is a separate pull request "
            "that changes only CLA.md."
        )

    # ── a signature pull request ─────────────────────────────────────────
    others = sorted(f for f in pr["changed_files"] if f != INDEX)
    if others:
        return BAD_SIGNATURE, f"a signature pull request changes only {INDEX}; this one also changes {others!r}"
    base, head = pr["base_index"], pr["head_index"]
    if not head.startswith(base.rstrip("\n")):
        return BAD_SIGNATURE, f"{INDEX} may only grow: an existing line was changed or removed"
    added = [l for l in head[len(base.rstrip("\n")):].splitlines() if l.strip()]
    if len(added) != 1:
        return BAD_SIGNATURE, f"a signature adds exactly one line; this adds {len(added)}"
    m = ROW.match(added[0])
    if not m:
        return BAD_SIGNATURE, f"the added line is not '| @login | id | YYYY-MM-DD | cla-X.Y (sha) |': {added[0]!r}"
    if m.group("id") != uid:
        return BAD_SIGNATURE, (
            f"the line names user id {m.group('id')!r}, but this pull request is by {login!r} (id {uid}). "
            "A contributor adds their OWN line, from their own account."
        )
    if m.group("login").lower() != login.lower():
        return BAD_SIGNATURE, f"the line names {m.group('login')!r}, but this pull request is by {login!r}"
    if uid in signed_ids(base):
        return BAD_SIGNATURE, f"id {uid} has already signed; a correction is a new line only when the old one is wrong — ask a maintainer"
    try:
        date = datetime.date.fromisoformat(m.group("date"))
    except ValueError:
        return BAD_SIGNATURE, f"{m.group('date')!r} is not a date"
    if date > pr["today"] + datetime.timedelta(days=1):
        return BAD_SIGNATURE, f"the date {date} is in the future"
    if m.group("version") != pr["version"]:
        return BAD_SIGNATURE, f"the line accepts {m.group('version')!r}; the current agreement is {pr['version']!r}"
    if not pr["version_sha"].startswith(m.group("sha")):
        return BAD_SIGNATURE, (
            f"the line names commit {m.group('sha')!r}; the agreement was last changed in "
            f"{pr['version_sha'][:12]!r}. Use `git log -1 --format=%h -- {AGREEMENT}`."
        )
    return PASS, f"{login!r} (id {uid}) signs {pr['version']} ({pr['version_sha'][:12]})"


def check_docs(root="."):
    """CONTRIBUTING.md quotes clause 8 so a signer reads it before signing. The
    quote must stay the agreement's clause, word for word."""
    agr = open(os.path.join(root, AGREEMENT), encoding="utf-8").read()
    doc = open(os.path.join(root, "CONTRIBUTING.md"), encoding="utf-8").read()
    start, end = "**8. Your personal data and the public record.**", "**9. Governing law.**"
    if start not in agr or end not in agr:
        return 1, "the agreement no longer has clause 8 where this check looks"
    clause = agr[agr.index(start):agr.index(end)]
    q = doc.find("> " + start)
    if q < 0:
        return 1, "CONTRIBUTING.md no longer quotes clause 8"
    quote = doc[q:doc.index("### How to sign", q)]
    words = lambda t: re.sub(r"[>\s]+", " ", t).strip()
    if words(quote) != words(clause):
        return 1, "CONTRIBUTING.md's clause 8 differs from the agreement's — update the quote"
    return 0, "clause 8 in CONTRIBUTING.md is the agreement's, word for word"


CORPORATE_EMPTY = "# Corporate CLA signatures\n\n"


def selftest(directory):
    """Run the fixture table. Expected exit code is in the file name:
    `<case>.<code>.json`. An empty table FAILS rather than passing on 0 cases."""
    files = sorted(f for f in os.listdir(directory) if f.endswith(".json"))
    if not files:
        print(f"cla-check selftest: NO FIXTURES in {directory} — refusing to report green on 0 cases")
        return 1
    bad = 0
    for name in files:
        want = int(name.rsplit(".", 2)[1])
        with open(os.path.join(directory, name)) as fh:
            pr = json.load(fh)
        pr["today"] = datetime.date.fromisoformat(pr["today"])
        pr.setdefault("base_corporate", CORPORATE_EMPTY)
        pr.setdefault("head_corporate", pr["base_corporate"])
        pr.setdefault("corporate_version", "ccla-1.0")
        got, msg = decide(pr)
        if got != want:
            bad += 1
        print(f"  {'ok ' if got == want else 'FAIL'} {name:<40} want={NAMES[want]} got={NAMES[got]} :: {msg}")
    code, msg = check_docs()
    print(f"  {'ok ' if code == 0 else 'FAIL'} docs: {msg}")
    try:
        with open(EXEMPTIONS, encoding="utf-8") as fh:
            n = len(exemption_reasons(json.load(fh)["exemptions"]))
        print(f"  ok  {EXEMPTIONS}: {n} entries, each with an id and a reason")
    except (OSError, KeyError, ValueError) as e:
        code = 1
        print(f"  FAIL {EXEMPTIONS}: {type(e).__name__}: {e}")
    live = open(CORPORATE, encoding="utf-8").read()
    n = len(corporate_ids(live))
    if any(l.startswith(("## ", "|")) for l in live.splitlines()):
        # The prose above the first signature is the base; everything from there
        # on must be signatures, so check it as if it had all just been added.
        lines = live.splitlines(keepends=True)
        first = next(i for i, l in enumerate(lines) if l.startswith(("## ", "|")))
        live_code, live_msg = check_corporate({"base_corporate": "".join(lines[:first]), "head_corporate": live,
                                               "corporate_version": "ccla-1.0",
                                               "today": datetime.datetime.now(datetime.timezone.utc).date()})
    else:
        live_code, live_msg = PASS, "no companies yet"
    code = code or (1 if live_code else 0)
    print(f"  {'ok ' if live_code == PASS else 'FAIL'} {CORPORATE}: {n} covered persons; {live_msg}")
    print(f"cla-check selftest: {len(files)} cases + docs, {bad + code} failed")
    return 1 if bad or code else 0


def api(path, token):
    req = urllib.request.Request(
        f"https://api.github.com/{path}",
        headers={"Authorization": f"Bearer {token}", "Accept": "application/vnd.github+json"},
    )
    with urllib.request.urlopen(req, timeout=30) as r:
        return json.load(r)


def gather(event, repo, token):
    """Everything decide() needs, from the event, the API (head side, as data)
    and the BASE checkout this script runs in."""
    pr = event["pull_request"]
    base_sha, head_sha = pr["base"]["sha"], pr["head"]["sha"]
    compare = api(f"repos/{repo}/compare/{base_sha}...{head_sha}", token)
    changed = [f["filename"] for f in compare.get("files", [])]
    if len(changed) >= 300:
        raise RuntimeError("the compare API caps at 300 files; cannot see the whole change")
    base_index = open(INDEX, encoding="utf-8").read()
    head_index = base_index
    if INDEX in changed:
        blob = api(f"repos/{repo}/contents/{INDEX}?ref={head_sha}", token)
        head_index = base64.b64decode(blob["content"]).decode("utf-8")
    with open(EXEMPTIONS, encoding="utf-8") as fh:
        exemptions = json.load(fh)["exemptions"]
    base_corporate = open(CORPORATE, encoding="utf-8").read()
    head_corporate = base_corporate
    if CORPORATE in changed:
        blob = api(f"repos/{repo}/contents/{CORPORATE}?ref={head_sha}", token)
        head_corporate = base64.b64decode(blob["content"]).decode("utf-8")
    cv = re.search(r"^Version:\s*(ccla-[0-9]+\.[0-9]+)\s*$", open(CORPORATE_AGREEMENT, encoding="utf-8").read(), re.M)
    if not cv:
        raise RuntimeError(f"{CORPORATE_AGREEMENT} has no 'Version: ccla-X.Y' line")
    agreement = open(AGREEMENT, encoding="utf-8").read()
    v = re.search(r"^Version:\s*(cla-[0-9]+\.[0-9]+)\s*$", agreement, re.M)
    if not v:
        raise RuntimeError(f"{AGREEMENT} has no 'Version: cla-X.Y' line")
    version_sha = subprocess.check_output(["git", "log", "-1", "--format=%H", "--", AGREEMENT], text=True).strip()
    if not version_sha:
        raise RuntimeError(f"git log found no commit for {AGREEMENT} (shallow checkout?)")
    return {
        "author": {
            "login": pr["user"]["login"],
            "id": pr["user"]["id"],
        },
        "changed_files": changed,
        "exemptions": exemptions,
        "base_corporate": base_corporate,
        "head_corporate": head_corporate,
        "corporate_version": cv.group(1),
        "base_index": base_index,
        "head_index": head_index,
        "version": v.group(1),
        "version_sha": version_sha,
        "today": datetime.datetime.now(datetime.timezone.utc).date(),
    }


def main():
    p = argparse.ArgumentParser()
    p.add_argument("--selftest", metavar="DIR", nargs="?", const="scripts/testdata/cla-check")
    p.add_argument("--event", default=os.environ.get("GITHUB_EVENT_PATH", ""))
    p.add_argument("--repo", default=os.environ.get("GITHUB_REPOSITORY", ""))
    args = p.parse_args()
    if args.selftest:
        return selftest(args.selftest)
    token = os.environ.get("GH_TOKEN", "")
    if not (args.event and args.repo and token):
        print("cla-check: ERROR — needs GITHUB_EVENT_PATH, GITHUB_REPOSITORY and GH_TOKEN")
        return ERROR
    try:
        with open(args.event) as fh:
            event = json.load(fh)
        pr = gather(event, args.repo, token)
    except Exception as e:  # the instrument failing is its own outcome, never a pass
        print(f"cla-check: ERROR — {type(e).__name__}: {e}")
        return ERROR
    code, msg = decide(pr)
    print(f"cla-check: {NAMES[code]} — {msg}")
    return code


if __name__ == "__main__":
    sys.exit(main())
