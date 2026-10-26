# Reporting a vulnerability

**Please do not open a public issue.** Use GitHub's private vulnerability reporting:
**[Report a vulnerability](https://github.com/KlangHaus/resonans/security/advisories/new)**.
It is private to you and the maintainers, and it gives us a place to write the fix and the
advisory together.

We will acknowledge within 5 working days and tell you what we found, whether or not we
agree it is exploitable. If we disagree, you get the reasoning, not a closed ticket.

If you have no GitHub account, open a regular issue containing **only** the words
"security report, please contact me" and no details at all, and we will find a private
channel.

## What we consider a vulnerability

Resonans is multi-tenant and self-hosted, so the things we care about most:

- **Cross-tenant access.** Any way to read or write another tenant's content, including
  through the delivery API, preview tokens, asset URLs, or a crafted `org` claim.
- **Authentication bypass.** Reaching an editor endpoint without a verified JWT, or
  getting the tenant from something other than that verified token.
- **A guard that fails open.** A missing or malformed configuration value that *opens*
  something instead of refusing to start. We treat this as severe even without a working
  exploit, because it is invisible in normal operation.
- **Served content executing on the origin.** Anything that gets a scriptable type past
  the upload allowlist or out through `/_assets`.
- **Path traversal** out of the asset volume.

## What is not a vulnerability

- Anything that requires `RESONANS_DEV_AUTH=1`. That is the documented development
  opt-in, and setting it with `NODE_ENV=production` is already refused.
- Missing TLS or a publicly bound port in `compose.yml`. That file deliberately serves
  plain HTTP on localhost and expects your own reverse proxy in front; the README says so.
- Denial of service by an authenticated editor against their own tenant.
- Vulnerability-scanner output with no reachable call path. We will read a report that
  names the path; we will not act on a CVE list.

## If you run Resonans

Pin a released version rather than `latest`, keep `RESONANS_DEV_AUTH` unset, and do not
give the role in `DATABASE_URL` `SUPERUSER` or `BYPASSRLS` — that single change removes
every tenant-isolation guarantee in the database. With `NODE_ENV=production` the server
refuses to start as such a role; in any other environment it starts and logs
`db_role_bypasses_rls`, so do not run a shared instance in development mode.
