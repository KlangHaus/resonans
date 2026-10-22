# ADR 0001 — The editor logs in with standard OpenID Connect

- **Status:** Proposed — needs acceptance by [infra] (CTO) before implementation
- **Date:** 2026-10-22
- **Decided by:** Allan (direction, 2026-10-22); this record written by [resonans]
- **Scope:** Resonans' editor UI (the code in `studio/`) and the editor API (`/api/v1`).
  The delivery API, preview tokens and `RESONANS_DEV_AUTH` are unchanged.

## Context

Resonans is open source under AGPL-3.0 and is meant to be run by anyone, on their own
infrastructure. The editor API is already close to that. The editor UI is not.

Measured on `main` `f2114a9`:

- **The API is provider-neutral.** `internal/auth` verifies a bearer JWT against any
  issuer's JWKS (`RESONANS_JWT_ISSUER`, `RESONANS_JWKS_URL`) and reads the tenant from a
  claim named `org`, hard-coded (`internal/auth/jwt.go`, `orgClaim`).
- **The editor UI is tied to one product.** `studio/app/composables/useAuth.ts` calls
  BetterAuth's own endpoints — `/api/auth/sign-in/email`, `/api/auth/get-session`,
  `/api/auth/token` — with an e-mail and a password typed into Resonans' own login page.
  Someone running Keycloak, Authentik, Zitadel or any other identity provider cannot log in
  to the editor at all.
- **There are no roles.** The actor carries `Sub` and `Tenant` only. Every holder of a
  valid token for a tenant can do everything, including `PUT /api/v1/settings` and
  `PUT /api/v1/settings/preview` (open item: ClickUp `869fbfz73`).
- **The audience is never checked in production.** `VerifierConfig.Audience` exists and is
  asserted when set, but no configuration sets it: outside tests, `Audience` occurs only in
  `internal/auth/jwt.go`. Resonans therefore accepts any valid token from its issuer,
  whichever service it was minted for. Whether a token minted for another KlangHaus
  service actually carries an `org` claim that would pass here is UNMEASURED.
- **KlangHaus' own identity service is already a standard OIDC provider.** The `auth`
  service runs `@better-auth/oauth-provider` (authorization code + PKCE, discovery at
  `/api/auth/.well-known/openid-configuration`, RS256 via JWKS), measured on `auth`
  `main` `b3bf711`. What it does NOT do today: put `org` or a role into the tokens that
  plugin issues. `org` is added only by the `jwt()` plugin's `definePayload`, which serves
  the `/api/auth/token` path the editor uses now.

The README already promises that Resonans "does not manage users or send email. It verifies
JWTs from an identity provider you already run." The editor UI breaks that promise.

## Decision

### 1. The editor logs in with OIDC authorization code + PKCE, against any provider

The editor UI speaks only standard OpenID Connect. No provider-specific endpoint, SDK or
login form remains in Resonans.

- **Flow:** authorization code with PKCE (`S256` only; `plain` is refused). `state` and
  `nonce` are required and checked. The `iss` parameter in the authorization response
  (RFC 9207) is checked when the provider sends it.
- **Discovery:** configured by issuer URL only; endpoints and JWKS come from
  `/.well-known/openid-configuration`. Resonans refuses to start if discovery fails or the
  discovered `issuer` differs from the configured one.
- **Where the tokens live: a backend-for-frontend.** The editor UI already has a server
  (Nitro, `studio/server/`). It becomes the OIDC client: a *confidential* client that does
  the code exchange, keeps access and refresh tokens server-side, and gives the browser
  only an `HttpOnly; Secure; SameSite=Strict` session cookie. Editor API calls go through
  it and get the bearer added there.
  - Why not a public client in the browser: a CMS editor renders user-authored HTML, and
    Resonans has a stored-XSS history (resonans#52). A token that no script can read is
    the one an XSS cannot steal. This follows the IETF best current practice for
    browser-based apps.
  - The current "JWT in memory only" rule is kept in spirit and strengthened: the browser
    holds no token at all.
  - **As implemented:** the session is a sealed (encrypted + MACed) cookie, not a server-side
    store, so the editor stays stateless and needs no table. It holds the access and refresh
    tokens and a small user object, never the ID token. Its sealed size is measured on every
    write and refused above 3 800 bytes with a message naming the cause, instead of the
    browser silently dropping a cookie over its ~4 kB limit.
- **Logout:** local session ends; RP-initiated logout (`end_session_endpoint`) is called
  when the provider advertises it.

### 2. The API checks audience, and tenant and role come from configurable claims

The editor API keeps verifying JWTs locally against the issuer's JWKS, with four changes:

| Setting | Default | Meaning |
| --- | --- | --- |
| `RESONANS_JWT_AUDIENCE` | *(required)* | The `aud` a token must carry. Startup is refused without it once JWT auth is configured — closes the gap above. |
| `RESONANS_TENANT_CLAIM` | `org` | Claim holding the tenant id (multi-tenant mode). |
| `RESONANS_ROLE_CLAIM` | `resonans_role` | Claim holding the role. A string or an array of strings. |
| `RESONANS_ROLE_MAP` | *(empty)* | Optional `provider-value=role` pairs, e.g. `cms-admins=admin,cms-writers=editor`, so a Keycloak group or a BetterAuth organization role can be used without changing the provider. |

Signature algorithm is pinned to an allowlist (`RS256`, `ES256`, `EdDSA`); `none` and HMAC
algorithms are refused.

### 3. Four built-in roles, enforced in Resonans, fail closed

| Role | Can |
| --- | --- |
| `viewer` | Read documents, versions, schema, assets and settings. |
| `editor` | `viewer`, plus create and update documents, upload assets, request a preview token. |
| `publisher` | `editor`, plus publish, delete and restore documents and assets, roll back versions. |
| `admin` | `publisher`, plus `PUT /api/v1/settings` and `PUT /api/v1/settings/preview`. |

- A token with no recognised role is refused with `403`, never given a default role. A
  missing claim is a configuration error to fix, not a reason to open up.
- Roles are checked in one middleware against a route table, so a new route without an
  entry fails a test instead of being reachable by everyone.
- The check sits behind an `Authorizer` interface. The built-in role model is the only
  implementation now. An external decision point (KlangHaus' PDP / OpenFGA) can be added
  later as a second implementation without changing handlers. Self-hosters never need it.

### 4. A single-tenant mode

`RESONANS_TENANT_MODE=single` with `RESONANS_TENANT_ID=<uuid>` serves exactly one tenant.
The tenant claim is not read. This is the normal case for an organisation hosting its own
CMS, and it means their provider needs only a role claim. Row level security stays on in
both modes: single-tenant is one tenant id, not RLS turned off.

`RESONANS_TENANT_MODE=multi` (default, today's behaviour) requires the tenant claim and
refuses a token without it.

### 5. No user database of our own

Resonans stores no users, passwords, sessions beyond the editor's own cookie, or e-mail
addresses. It keeps the token's `sub` where it records who did something. Accounts,
password reset, MFA and SSO belong to the provider.

### 6. KlangHaus keeps BetterAuth as its OIDC provider

KlangHaus' own deployment uses the same code path as everyone else. The `auth` service
registers Resonans' editor as a confidential OIDC client in `oauthProvider`, and adds `org`
and a role to the tokens it issues for that client. The role can come from the existing
BetterAuth organization membership (`owner`, `admin`, `member`) through
`RESONANS_ROLE_MAP`, so no new role store is needed.

## Consequences

**Good**

- Anyone can run Resonans with the identity provider they already have, which is what
  "self-hosted, AGPL" promises.
- Editors' tokens are out of reach of scripts in the editor.
- The missing audience check is closed, and per-route roles replace "a valid token can do
  everything". `869fbfz73` is resolved by this.
- KlangHaus dogfoods the same path outside users take, so its bugs reach us first.

**Costs**

- The editor UI's server needs a session secret (`NUXT_OIDC_SESSION_SECRET`). Rotating it
  logs every editor out. A provider that issues very large access tokens can exceed the
  cookie budget; that is refused loudly, and the cure is fewer claims for this client.
- Editor API calls gain a hop through the editor's server.
- Every deployment must configure an OIDC client. There is no "just log in" without a
  provider; the README's quickstart needs a tested example (Keycloak or Authentik in
  `compose.yml` under a profile).
- KlangHaus needs work in `auth` before cutover (claims for the Resonans client).

## Rejected alternatives

- **A built-in user database and login.** Passwords, reset e-mails, MFA, lockout and
  breach handling would all become Resonans' problem, contradicting "Postgres plus a
  binary" and the README's own promise. Every provider worth running already does this.
- **Keep BetterAuth-specific login and document it as a requirement.** Forces every
  self-hoster to run our identity stack; that is lock-in, the thing Resonans exists to
  avoid.
- **Public OIDC client in the browser (tokens in memory).** Simpler, no session store, but
  a successful XSS can use or steal the token. Rejected for the reason in §1.
- **A default role for tokens without one.** Convenient for first setup, and exactly the
  "permissive by default for developer experience" guard CONTRIBUTING.md says we refuse.

## Rollout

1. `RESONANS_JWT_AUDIENCE` added and required (API only; independent of everything else —
   can ship first).
2. Configurable claims, the role middleware and single-tenant mode in the API.
3. OIDC client and session in the editor UI's server; login page replaced by a redirect.
4. `auth`: register the client and add `org` + role claims for it ([auth]'s change).
5. KlangHaus cutover in production. (The BetterAuth-specific calls in `useAuth.ts` were
   removed in the same change as step 3: steps 1–2 already require a role claim, so no
   KlangHaus editor could have kept working through the old path anyway.)
6. README quickstart with a provider example, measured from a clean checkout.

Each step ships on its own. Until step 5, KlangHaus keeps today's login.

## Open questions (UNMEASURED)

- Whether `@better-auth/oauth-provider` access tokens are JWTs Resonans can verify
  locally, and which `iss` they carry (`https://auth.klanghaus.dk` or `…/api/auth`). If
  they are opaque, step 4 needs token introspection or a JWT-format option.
- Whether a token minted for another KlangHaus service passes Resonans today (the
  audience gap above). To be measured by [sikkerhed] before step 1 ships, not after.
- Tenant ids that are not UUIDs (some providers use slugs). Out of scope here;
  `RESONANS_ROLE_MAP` does not cover tenants.
