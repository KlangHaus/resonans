#!/usr/bin/env python3
"""CLA check: may this pull request be merged under the contributor agreement?

The signature model ([jura], docs 52cbb84, legal/cla-resonans-udkast.md §C.3):
a contributor signs by adding ONE line to CLA.md in a pull request FROM THEIR OWN
ACCOUNT. The pull request and the commit they authored are the evidence; the file
is only the index. So the decisive rule is not the line's format but WHO added it:
the line's numeric user ID must be the pull request author's. A line a maintainer
adds for someone else proves nothing, and this check refuses it.

Two kinds of pull request:

  SIGNATURE   it changes CLA.md. It must change ONLY CLA.md, the file may only
              grow by exactly one table row, and that row must name the author
              (login and numeric ID), the current agreement version and the SHA of
              the commit that last changed the agreement, a date not in the future,
              and an ID that has not signed before.
  CODE        anything else. The author must have signed (their ID is in CLA.md
              on the BASE branch), or be exempt: a bot, or someone the repository
              already lists as owner, member or collaborator.

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
EXEMPT_ASSOCIATIONS = {"OWNER", "MEMBER", "COLLABORATOR"}

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


def decide(pr):
    """Pure decision. `pr` holds: author {login, id, type, association},
    changed_files, base_index, head_index, version, version_sha, today.
    Returns (exit_code, message)."""
    author = pr["author"]
    login, uid = author["login"], str(author["id"])

    if INDEX not in pr["changed_files"]:
        if author.get("type") == "Bot":
            return PASS, f"exempt: {login!r} is a bot"
        if author.get("association") in EXEMPT_ASSOCIATIONS:
            return PASS, f"exempt: {login!r} is a repository {author['association'].lower()}"
        if uid in signed_ids(pr["base_index"]):
            return PASS, f"{login!r} (id {uid}) has signed"
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
        got, msg = decide(pr)
        if got != want:
            bad += 1
        print(f"  {'ok ' if got == want else 'FAIL'} {name:<40} want={NAMES[want]} got={NAMES[got]} :: {msg}")
    code, msg = check_docs()
    print(f"  {'ok ' if code == 0 else 'FAIL'} docs: {msg}")
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
        import base64

        blob = api(f"repos/{repo}/contents/{INDEX}?ref={head_sha}", token)
        head_index = base64.b64decode(blob["content"]).decode("utf-8")
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
            "type": pr["user"].get("type", ""),
            "association": pr.get("author_association", ""),
        },
        "changed_files": changed,
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
