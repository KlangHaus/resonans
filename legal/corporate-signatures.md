# Corporate CLA signatures

This file is the **index** of companies that have signed the
[Resonans Corporate Contributor Licence Agreement](corporate-cla.md), and of the persons
each one covers (its Schedule). **The company signs, not the person**, so a person listed
here has not signed the individual agreement and is not in [CLA.md](../CLA.md). The
signed agreement itself is held by KlangHaus; a maintainer checks it before merging a
change to this file.

Each company is a `##` line — company · company number · signatory, role · date ·
version — followed by a table of the persons it covers, by GitHub username and numeric
user ID. Lines are only ever added, never changed or removed.

**A person is added** to a company's table by its written notice (clause 3).

**When cover ends**, a row is added under the same company, below the person's own row:
`| @login | id | covered until YYYY-MM-DD |`. The date is the **first day the person is
no longer covered**, and it is the date of the **company's written notice**, not the
date this file is edited: the agreement takes effect on the notice, and this file only
records it. A maintainer checks the date against the notice before merging. The row
says nothing about why cover ended (clause 6: nothing else about a Scheduled Person is
recorded). Contributions made before that date remain licensed.

The `CLA` check dates a contribution by **when the check runs for a push** to the pull
request — server time — not by the commit date, which the contributor writes and could
set to any day. A pull request pushed while cover held keeps that green result; a
later push, or a reopen, after the date is refused.

The push time can only be later than the commit, never earlier, so the check can
refuse a contribution the agreement covers but cannot pass one it does not. **When a
maintainer overrides such a refusal, that is not an exception to the agreement**: it
is the agreement applying where the check could not see the commit's real date
(clause 3 licenses a contribution when it is made).

An end date that does not parse is never read as "no end": the check reports an
error. A licence field that fails open would grant rights nobody gave.

The form, indented here so the check does not read it as a signature (the check only
reads lines that start at the left margin):

    ## Example ApS · CVR 12345678 · Jane Doe, CEO · 2026-10-28 · ccla-1.0

    | GitHub | User ID |
    |---|---|
    | @jane-doe | 1234567 |
    | @jane-doe | 1234567 | covered until 2026-12-01 |

First company below this line.
