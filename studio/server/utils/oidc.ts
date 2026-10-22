// OpenID Connect for the editor (ADR 0001): this Nitro server is the OIDC
// client — a confidential backend-for-frontend. The browser never holds a
// token. It holds one sealed, HttpOnly session cookie, and every editor API
// call goes through /proxy, which adds the bearer here on the server.
//
// 🔴 WHY THE TOKEN STAYS ON THE SERVER.
//    The editor renders user-authored HTML, and Resonans has had a stored XSS
//    (resonans#52). A token in page memory is readable by any script that gets
//    into the page; a token that never reaches the page is not. That is the
//    whole reason for the extra hop.
//
// 🔴 PROVIDER-NEUTRAL BY CONSTRUCTION.
//    Only the issuer URL is configured; every endpoint comes from discovery,
//    and openid-client refuses a discovery document whose `issuer` differs from
//    the configured one. Nothing here names a provider.
import type { H3Event } from 'h3'
import * as client from 'openid-client'

export interface OidcSettings {
  issuer: string
  clientId: string
  clientSecret: string
  scopes: string
  /** Externally visible origin of the editor, e.g. https://cms.example.com. */
  editorUrl: string
  /** Where the Resonans API answers from this server, e.g. http://resonans:3000. */
  apiUrl: string
  sessionSecret: string
  allowInsecure: boolean
}

/** The settings, or null when OIDC is not configured (dev mode). Throws when
 *  OIDC is half-configured: an issuer without the rest is a deployment
 *  mistake, and falling back to dev mode would open the editor. */
export function oidcSettings(): OidcSettings | null {
  // Nuxt parses NUXT_* environment values (destr): "1" arrives as the NUMBER
  // 1, and a numeric-looking secret would too. Every value is read back as a
  // string, so the waiver below compares like with like — measured: without
  // this, NUXT_OIDC_ALLOW_INSECURE=1 never matched '1'.
  const raw = useRuntimeConfig().oidc as Record<string, unknown>
  const str = (k: string) => (raw[k] === undefined || raw[k] === null ? '' : String(raw[k]))
  if (!str('issuer')) return null
  const s: OidcSettings = {
    issuer: str('issuer'),
    clientId: str('clientId'),
    clientSecret: str('clientSecret'),
    scopes: str('scopes') || 'openid profile email',
    editorUrl: str('editorUrl').replace(/\/$/, ''),
    apiUrl: str('apiUrl').replace(/\/$/, ''),
    sessionSecret: str('sessionSecret'),
    // Exactly "1", like the server's own waivers: "true" and "" stay closed.
    allowInsecure: str('allowInsecure') === '1',
  }
  const missing = (['clientId', 'clientSecret', 'editorUrl', 'apiUrl', 'sessionSecret'] as const).filter((k) => !s[k])
  if (missing.length) {
    throw createError({ statusCode: 500, statusMessage: `OIDC issuer is set but ${missing.join(', ')} is not` })
  }
  if (s.sessionSecret.length < 32) {
    throw createError({ statusCode: 500, statusMessage: 'NUXT_OIDC_SESSION_SECRET must be at least 32 characters' })
  }
  return s
}

let discovered: { key: string; config: client.Configuration } | null = null

/** Discovery, cached per process. A failed discovery is not cached, so the
 *  editor recovers on the next request once the provider is reachable. */
export async function oidcConfig(s: OidcSettings): Promise<client.Configuration> {
  const key = `${s.issuer}|${s.clientId}`
  if (discovered?.key === key) return discovered.config
  const config = await client.discovery(
    new URL(s.issuer),
    s.clientId,
    s.clientSecret,
    undefined,
    s.allowInsecure ? { execute: [client.allowInsecureRequests] } : undefined,
  )
  discovered = { key, config }
  return config
}

export function redirectUri(s: OidcSettings): string {
  return `${s.editorUrl}/auth/callback`
}

// --- Sessions --------------------------------------------------------------

export interface EditorUser {
  sub: string
  email?: string
  name?: string
}

export interface EditorSession {
  user?: EditorUser
  accessToken?: string
  refreshToken?: string
  /** Epoch seconds. */
  expiresAt?: number
}

interface LoginTransaction {
  verifier?: string
  state?: string
  nonce?: string
  next?: string
}

// A sealed cookie is capped by the browser at about 4 kB. Measured, not hoped:
// the session is sealed after every write and refused above this size, with a
// message that says why, rather than silently losing the cookie.
const MAX_SEALED = 3800

function secure(s: OidcSettings): boolean {
  return s.editorUrl.startsWith('https://')
}

/** The editor session: Strict, so no cross-site request ever carries it. */
export function sessionConfig(s: OidcSettings) {
  return {
    name: 'resonans_session',
    password: s.sessionSecret,
    maxAge: 60 * 60 * 8,
    cookie: { httpOnly: true, secure: secure(s), sameSite: 'strict' as const, path: '/' },
  }
}

/** The login transaction (PKCE verifier, state, nonce) lives only between the
 *  redirect to the provider and the callback. It must be Lax: the callback is a
 *  top-level navigation FROM the provider's site, and a Strict cookie is not
 *  sent on it. Ten minutes, then it is useless. */
function loginConfig(s: OidcSettings) {
  return {
    name: 'resonans_login',
    password: s.sessionSecret,
    maxAge: 600,
    cookie: { httpOnly: true, secure: secure(s), sameSite: 'lax' as const, path: '/auth' },
  }
}

export async function editorSession(event: H3Event, s: OidcSettings) {
  return useSession<EditorSession>(event, sessionConfig(s))
}

export async function loginTransaction(event: H3Event, s: OidcSettings) {
  return useSession<LoginTransaction>(event, loginConfig(s))
}

export async function saveTokens(event: H3Event, s: OidcSettings, tokens: client.TokenEndpointResponse & client.TokenEndpointResponseHelpers, user?: EditorUser) {
  const session = await editorSession(event, s)
  const expiresIn = tokens.expiresIn()
  await session.update({
    ...(user ? { user } : {}),
    accessToken: tokens.access_token,
    // A refresh response may omit the refresh token, meaning "keep the old one".
    refreshToken: tokens.refresh_token ?? session.data.refreshToken,
    expiresAt: expiresIn ? Math.floor(Date.now() / 1000) + expiresIn : undefined,
  })
  const sealed = await sealSession(event, sessionConfig(s))
  if (sealed.length > MAX_SEALED) {
    await session.clear()
    throw createError({
      statusCode: 500,
      statusMessage: `the provider's tokens seal to ${sealed.length} bytes, over the ${MAX_SEALED}-byte cookie budget — use shorter access tokens (fewer claims) for this client`,
    })
  }
}

/** A usable access token, refreshed when it is within 30 s of expiry. Null
 *  means "log in again"; the session is cleared so the next answer is honest. */
export async function accessToken(event: H3Event, s: OidcSettings): Promise<string | null> {
  const session = await editorSession(event, s)
  const { accessToken: at, refreshToken: rt, expiresAt } = session.data
  if (!at) return null
  const now = Math.floor(Date.now() / 1000)
  if (!expiresAt || expiresAt - 30 > now) return at
  if (!rt) {
    await session.clear()
    return null
  }
  try {
    const tokens = await client.refreshTokenGrant(await oidcConfig(s), rt)
    await saveTokens(event, s, tokens)
    return tokens.access_token
  } catch {
    await session.clear()
    return null
  }
}

/** State-changing requests must come from the editor's own origin. The
 *  session cookie is SameSite=Strict already; this is the second lock, and it
 *  fails closed when the browser sends neither header. */
export function assertSameOrigin(event: H3Event, s: OidcSettings) {
  const method = event.method
  if (method === 'GET' || method === 'HEAD') return
  const origin = getRequestHeader(event, 'origin')
  if (origin) {
    if (origin === new URL(s.editorUrl).origin) return
  } else if (getRequestHeader(event, 'sec-fetch-site') === 'same-origin') {
    return
  }
  throw createError({ statusCode: 403, statusMessage: 'cross-origin request refused' })
}


