// The login server's rules, without a provider: the real login against a real
// provider is scripts/quickstart_smoke.sh. These pin what that run cannot provoke:
// half a configuration, an open redirect, a reused login, a token over the cookie
// budget, a header slipping through the proxy.
//
// Nitro's auto-imports do not exist under vitest, so each one is a global here,
// backed by a small fake event. The modules under test are the real ones.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const client = vi.hoisted(() => ({
  discovery: vi.fn(),
  allowInsecureRequests: Symbol('allowInsecureRequests'),
  randomPKCECodeVerifier: vi.fn(() => 'the-verifier'),
  randomState: vi.fn(() => 'the-state'),
  randomNonce: vi.fn(() => 'the-nonce'),
  calculatePKCECodeChallenge: vi.fn(async () => 'the-challenge'),
  buildAuthorizationUrl: vi.fn(() => new URL('https://idp.example/authorize')),
  authorizationCodeGrant: vi.fn(),
  refreshTokenGrant: vi.fn(),
}))
vi.mock('openid-client', () => client)

interface FakeEvent {
  method: string
  path: string
  query: Record<string, string>
  headers: Record<string, string>
  body?: Uint8Array
  sessions: Record<string, Record<string, unknown>>
  response: { status?: number; headers: Record<string, string> }
}

function fakeEvent(over: Partial<FakeEvent> = {}): FakeEvent {
  return { method: 'GET', path: '', query: {}, headers: {}, sessions: {}, response: { headers: {} }, ...over }
}

class HttpError extends Error {
  constructor(public statusCode: number, message?: string) {
    super(message)
  }
}

const settings = {
  issuer: 'https://idp.example',
  clientId: 'resonans-editor',
  clientSecret: 'client-secret',
  scopes: '',
  editorUrl: 'https://cms.example/',
  apiUrl: 'http://api:3000/',
  sessionSecret: 'x'.repeat(32),
  allowInsecure: '',
  resource: '',
}
let runtime: Record<string, unknown>
let sealedLength: number

function session(event: FakeEvent, cfg: { name: string }) {
  event.sessions[cfg.name] ??= {}
  return {
    get data() {
      return event.sessions[cfg.name]
    },
    update: async (patch: Record<string, unknown>) => {
      Object.assign(event.sessions[cfg.name], patch)
    },
    clear: async () => {
      event.sessions[cfg.name] = {}
    },
  }
}

const redirects = (v: unknown) => (v as { redirect: string }).redirect

vi.stubGlobal('useRuntimeConfig', () => ({ oidc: runtime }))
vi.stubGlobal('createError', (e: { statusCode: number; statusMessage?: string }) => new HttpError(e.statusCode, e.statusMessage))
vi.stubGlobal('useSession', async (event: FakeEvent, cfg: { name: string }) => session(event, cfg))
vi.stubGlobal('sealSession', async () => 'x'.repeat(sealedLength))
vi.stubGlobal('getRequestHeader', (event: FakeEvent, h: string) => event.headers[h])
vi.stubGlobal('defineEventHandler', (fn: unknown) => fn)
vi.stubGlobal('getQuery', (event: FakeEvent) => event.query)
vi.stubGlobal('sendRedirect', (_: FakeEvent, url: string) => ({ redirect: url }))
vi.stubGlobal('getRequestURL', (event: FakeEvent) => new URL(`http://internal-host/${event.path}`))
vi.stubGlobal('getRouterParam', (event: FakeEvent) => event.path.split('?')[0])
vi.stubGlobal('readRawBody', async (event: FakeEvent) => event.body)
vi.stubGlobal('setResponseStatus', (event: FakeEvent, status: number) => {
  event.response.status = status
})
vi.stubGlobal('setResponseHeader', (event: FakeEvent, k: string, v: string) => {
  event.response.headers[k] = v
})

const oidc = await import('./utils/oidc')
for (const [name, fn] of Object.entries(oidc)) vi.stubGlobal(name, fn)
const login = (await import('./routes/auth/login.get')).default as unknown as (e: FakeEvent) => Promise<unknown>
const callback = (await import('./routes/auth/callback.get')).default as unknown as (e: FakeEvent) => Promise<unknown>
const me = (await import('./routes/auth/me.get')).default as unknown as (e: FakeEvent) => Promise<unknown>
const logout = (await import('./routes/auth/logout.post')).default as unknown as (e: FakeEvent) => Promise<unknown>
const proxy = (await import('./routes/proxy/[...path]')).default as unknown as (e: FakeEvent) => Promise<Uint8Array>

const s = () => oidc.oidcSettings()!

function tokens(over: Record<string, unknown> = {}) {
  return {
    access_token: 'the-access-token',
    refresh_token: 'the-refresh-token',
    id_token: 'the-id-token',
    expiresIn: () => 3600,
    claims: () => ({ sub: 'user-1', email: 'a@example.org', name: 'A' }),
    ...over,
  }
}

beforeEach(() => {
  runtime = { ...settings }
  sealedLength = 100
  vi.clearAllMocks()
  // A new issuer per test defeats the per-process discovery cache.
  runtime.issuer = `https://idp.example/${Math.random()}`
  client.discovery.mockResolvedValue({ serverMetadata: () => ({}) })
})

describe('oidcSettings', () => {
  it('is null without an issuer: that is dev mode', () => {
    runtime.issuer = ''
    expect(oidc.oidcSettings()).toBeNull()
  })

  it.each(['clientId', 'clientSecret', 'editorUrl', 'apiUrl', 'sessionSecret'])(
    'refuses an issuer without %s rather than falling back to dev mode',
    (key) => {
      runtime[key] = ''
      expect(() => oidc.oidcSettings()).toThrow(key)
    },
  )

  it('refuses a session secret under 32 characters', () => {
    runtime.sessionSecret = 'x'.repeat(31)
    expect(() => oidc.oidcSettings()).toThrow('at least 32')
  })

  it('accepts a session secret of exactly 32 characters', () => {
    expect(s().sessionSecret).toHaveLength(32)
  })

  it('reads the waiver as exactly 1, including the number Nuxt parses it into', () => {
    runtime.allowInsecure = 1
    expect(s().allowInsecure).toBe(true)
    runtime.allowInsecure = '1'
    expect(s().allowInsecure).toBe(true)
  })

  it.each(['true', 'yes', '0', '', 2])('does not read %s as the waiver', (v) => {
    runtime.allowInsecure = v
    expect(s().allowInsecure).toBe(false)
  })

  it('strips a trailing slash, so the redirect URI has one form', () => {
    expect(oidc.redirectUri(s())).toBe('https://cms.example/auth/callback')
    expect(s().apiUrl).toBe('http://api:3000')
  })

  it('defaults the scopes when none are configured', () => {
    expect(s().scopes).toBe('openid profile email')
  })
})

describe('sessionConfig', () => {
  it('is HttpOnly and Strict, and Secure on https', () => {
    const { cookie } = oidc.sessionConfig(s())
    expect(cookie).toMatchObject({ httpOnly: true, sameSite: 'strict', secure: true })
  })

  it('is not Secure on a plain-http editor, or the browser would drop it', () => {
    runtime.editorUrl = 'http://localhost:8080'
    expect(oidc.sessionConfig(s()).cookie.secure).toBe(false)
  })
})

describe('oidcConfig', () => {
  it('only allows plain http when waived', async () => {
    await oidc.oidcConfig(s())
    expect(client.discovery.mock.calls[0][4]).toBeUndefined()

    runtime.issuer = 'http://idp.local/other'
    runtime.allowInsecure = '1'
    await oidc.oidcConfig(s())
    expect(client.discovery.mock.calls[1][4]).toEqual({ execute: [client.allowInsecureRequests] })
  })

  it('caches a success and not a failure, so the editor recovers', async () => {
    client.discovery.mockRejectedValueOnce(new Error('provider down'))
    await expect(oidc.oidcConfig(s())).rejects.toThrow('provider down')
    await oidc.oidcConfig(s())
    await oidc.oidcConfig(s())
    expect(client.discovery).toHaveBeenCalledTimes(2)
  })
})

describe('assertSameOrigin', () => {
  const check = (e: FakeEvent) => oidc.assertSameOrigin(e as never, s())

  it.each(['GET', 'HEAD'])('lets %s through', (method) => {
    expect(() => check(fakeEvent({ method }))).not.toThrow()
  })

  it('accepts a write from the editor origin', () => {
    expect(() => check(fakeEvent({ method: 'POST', headers: { origin: 'https://cms.example' } }))).not.toThrow()
  })

  it('accepts a write the browser marks same-origin', () => {
    expect(() => check(fakeEvent({ method: 'POST', headers: { 'sec-fetch-site': 'same-origin' } }))).not.toThrow()
  })

  it.each(['https://evil.example', 'https://cms.example.evil.example', 'http://cms.example', 'null'])(
    'refuses a write from %s',
    (origin) => {
      expect(() => check(fakeEvent({ method: 'POST', headers: { origin } }))).toThrow('cross-origin')
    },
  )

  it('lets a foreign Origin win over a same-origin Sec-Fetch-Site', () => {
    const headers = { origin: 'https://evil.example', 'sec-fetch-site': 'same-origin' }
    expect(() => check(fakeEvent({ method: 'POST', headers }))).toThrow('cross-origin')
  })

  it('fails closed when the browser sends neither header', () => {
    expect(() => check(fakeEvent({ method: 'POST' }))).toThrow('cross-origin')
  })
})

describe('saveTokens', () => {
  it('refuses tokens over the cookie budget and leaves no session', async () => {
    const event = fakeEvent()
    sealedLength = 3801
    await expect(oidc.saveTokens(event as never, s(), tokens() as never)).rejects.toThrow('cookie budget')
    expect(event.sessions.resonans_session).toEqual({})
  })

  it('accepts tokens at exactly the budget', async () => {
    const event = fakeEvent()
    sealedLength = 3800
    await oidc.saveTokens(event as never, s(), tokens() as never)
    expect(event.sessions.resonans_session.accessToken).toBe('the-access-token')
  })

  it('keeps the old refresh token when a refresh response omits it', async () => {
    const event = fakeEvent({ sessions: { resonans_session: { refreshToken: 'old' } } })
    await oidc.saveTokens(event as never, s(), tokens({ refresh_token: undefined }) as never)
    expect(event.sessions.resonans_session.refreshToken).toBe('old')
  })

  // The ID token is the hint for logout. A refresh response may carry none, and
  // logout after the first refresh must still work.
  it('keeps the ID token when a refresh response omits it', async () => {
    const event = fakeEvent({ sessions: { resonans_session: { idToken: 'from-login' } } })
    await oidc.saveTokens(event as never, s(), tokens({ id_token: undefined }) as never)
    expect(event.sessions.resonans_session.idToken).toBe('from-login')
  })
})

describe('accessToken', () => {
  const now = () => Math.floor(Date.now() / 1000)
  const withSession = (data: Record<string, unknown>) => fakeEvent({ sessions: { resonans_session: data } })
  const get = (e: FakeEvent) => oidc.accessToken(e as never, s())

  it('is null with no session', async () => {
    expect(await get(fakeEvent())).toBeNull()
  })

  it('returns a token that is not near expiry, without refreshing', async () => {
    expect(await get(withSession({ accessToken: 'at', refreshToken: 'rt', expiresAt: now() + 60 }))).toBe('at')
    expect(client.refreshTokenGrant).not.toHaveBeenCalled()
  })

  it('refreshes within 30 seconds of expiry', async () => {
    client.refreshTokenGrant.mockResolvedValue(tokens({ access_token: 'fresh' }))
    expect(await get(withSession({ accessToken: 'at', refreshToken: 'rt', expiresAt: now() + 10 }))).toBe('fresh')
  })

  it('clears the session when it expired with no refresh token', async () => {
    const event = withSession({ accessToken: 'at', expiresAt: now() - 1 })
    expect(await get(event)).toBeNull()
    expect(event.sessions.resonans_session).toEqual({})
  })

  it('clears the session when the provider refuses the refresh', async () => {
    client.refreshTokenGrant.mockRejectedValue(new Error('invalid_grant'))
    const event = withSession({ accessToken: 'at', refreshToken: 'rt', expiresAt: now() - 1 })
    expect(await get(event)).toBeNull()
    expect(event.sessions.resonans_session).toEqual({})
  })
})

describe('GET /auth/login', () => {
  const nextAfter = async (next?: string) => {
    const event = fakeEvent({ query: next === undefined ? {} : { next } })
    await login(event)
    return event.sessions.resonans_login.next
  }

  it('stores the transaction and sends PKCE, state and nonce', async () => {
    const event = fakeEvent()
    expect(redirects(await login(event))).toBe('https://idp.example/authorize')
    expect(event.sessions.resonans_login).toMatchObject({ verifier: 'the-verifier', state: 'the-state', nonce: 'the-nonce' })
    expect(client.buildAuthorizationUrl.mock.calls[0][1]).toMatchObject({
      code_challenge: 'the-challenge',
      code_challenge_method: 'S256',
      state: 'the-state',
      nonce: 'the-nonce',
      redirect_uri: 'https://cms.example/auth/callback',
    })
  })

  it('never sends the verifier to the provider', async () => {
    await login(fakeEvent())
    expect(JSON.stringify(client.buildAuthorizationUrl.mock.calls[0][1])).not.toContain('the-verifier')
  })

  it('sends a resource only when configured', async () => {
    await login(fakeEvent())
    expect(client.buildAuthorizationUrl.mock.calls[0][1]).not.toHaveProperty('resource')
    runtime.resource = 'https://cms.example/api'
    await login(fakeEvent())
    expect(client.buildAuthorizationUrl.mock.calls[1][1]).toHaveProperty('resource', 'https://cms.example/api')
  })

  // What matters is where a browser lands, so every cell resolves the stored
  // destination as a browser resolves a Location header (WHATWG URL, which Node
  // shares) and asserts the origin.
  const lands = (location: string) => new URL(location, 'https://cms.example')

  it.each([
    ['protocol-relative', '//evil.example'],
    ['absolute, other host', 'https://evil.example/x'],
    ['absolute, same host other port', 'https://cms.example:8443/x'],
    ['backslash', '/\\evil.example'],
    ['slash backslash', '/\\/evil.example'],
    ['tab', '/\t/evil.example'],
    ['leading tab', '\t//evil.example'],
    ['newline', '/\n/evil.example'],
    ['carriage return', '/\r/evil.example'],
    ['javascript:', 'javascript:alert(1)'],
    // Same origin after resolving, but the path is //evil.example, which a
    // browser reads as a host. Only the `//` rule stops it.
    ['dot segment to a double slash', '/.//evil.example'],
    ['empty', ''],
  ])('%s: replaced by the start page', async (_, next) => {
    const stored = await nextAfter(next)
    expect(stored).toBe('/')
    expect(lands(stored).origin).toBe('https://cms.example')
  })

  // Encoded forms stay encoded: they are paths on this editor, not hosts.
  it.each([
    ['encoded tab', '/%09/evil.example'],
    ['encoded slashes', '/%2F%2Fevil.example'],
    ['encoded backslash', '/%5Cevil.example'],
    ['no leading slash', 'evil.example'],
  ])('%s: stays on this editor', async (_, next) => {
    const stored = await nextAfter(next)
    expect(stored.startsWith('//')).toBe(false)
    expect(lands(stored).origin).toBe('https://cms.example')
  })

  // The control that shows the cells above can see a difference: a real path
  // comes back, so they do not pass because everything becomes '/'.
  it.each([
    ['the start page', '/', '/'],
    ['a path', '/content/post', '/content/post'],
    ['a path with query and fragment', '/content?type=post#latest', '/content?type=post#latest'],
    ['this editor as an absolute URL', 'https://cms.example/content', '/content'],
  ])('POSITIVE CONTROL: %s is kept', async (_, next, want) => {
    expect(await nextAfter(next)).toBe(want)
  })

  it('is a 404 in dev mode', async () => {
    runtime.issuer = ''
    await expect(login(fakeEvent())).rejects.toMatchObject({ statusCode: 404 })
  })
})

describe('GET /auth/callback', () => {
  const started = (over: Record<string, unknown> = {}) =>
    fakeEvent({
      path: 'auth/callback?code=abc&state=the-state',
      sessions: { resonans_login: { verifier: 'v', state: 'the-state', nonce: 'n', next: '/content', ...over } },
    })

  it('logs in and goes to the stored destination', async () => {
    client.authorizationCodeGrant.mockResolvedValue(tokens())
    const event = started()
    expect(redirects(await callback(event))).toBe('/content')
    expect(event.sessions.resonans_session).toMatchObject({
      accessToken: 'the-access-token',
      idToken: 'the-id-token',
      user: { sub: 'user-1', email: 'a@example.org', name: 'A' },
    })
  })

  it('checks a stored destination again before redirecting to it', async () => {
    client.authorizationCodeGrant.mockResolvedValue(tokens())
    expect(redirects(await callback(started({ next: '/\t/evil.example' })))).toBe('/')
  })

  it('checks state, PKCE and nonce, and requires an ID token', async () => {
    client.authorizationCodeGrant.mockResolvedValue(tokens())
    await callback(started())
    expect(client.authorizationCodeGrant.mock.calls[0][2]).toEqual({
      pkceCodeVerifier: 'v',
      expectedState: 'the-state',
      expectedNonce: 'n',
      idTokenExpected: true,
    })
  })

  it('builds the current URL from configuration, not the Host header', async () => {
    client.authorizationCodeGrant.mockResolvedValue(tokens())
    await callback(started())
    expect(String(client.authorizationCodeGrant.mock.calls[0][1])).toBe(
      'https://cms.example/auth/callback?code=abc&state=the-state',
    )
  })

  it('spends the transaction, so a second callback starts over', async () => {
    client.authorizationCodeGrant.mockResolvedValue(tokens())
    const event = started()
    await callback(event)
    expect(event.sessions.resonans_login).toEqual({})
    await callback(event)
    expect(client.authorizationCodeGrant).toHaveBeenCalledTimes(1)
  })

  it('spends the transaction when the provider refuses too', async () => {
    client.authorizationCodeGrant.mockRejectedValue(new Error('invalid_grant'))
    const event = started()
    await callback(event)
    expect(event.sessions.resonans_login).toEqual({})
    expect(event.sessions.resonans_session ?? {}).toEqual({})
  })

  it.each(['verifier', 'state', 'nonce'])('does not call the provider without a stored %s', async (key) => {
    const event = started({ [key]: undefined })
    expect(redirects(await callback(event))).toMatch(/^\/login\?/)
    expect(client.authorizationCodeGrant).not.toHaveBeenCalled()
  })

  it('refuses when the grant fails', async () => {
    client.authorizationCodeGrant.mockRejectedValue(new Error('state mismatch'))
    expect(redirects(await callback(started()))).toMatch(/^\/login\?/)
  })

  it('refuses claims without a subject and saves nothing', async () => {
    client.authorizationCodeGrant.mockResolvedValue(tokens({ claims: () => ({}) }))
    const event = started()
    expect(redirects(await callback(event))).toMatch(/^\/login\?/)
    expect(event.sessions.resonans_session ?? {}).toEqual({})
  })
})

describe('GET /auth/me', () => {
  it('is 401 with no session', async () => {
    await expect(me(fakeEvent())).rejects.toMatchObject({ statusCode: 401 })
  })

  it('returns the user and never a token', async () => {
    const event = fakeEvent({ sessions: { resonans_session: { accessToken: 'at', user: { sub: 'u' } } } })
    const answer = await me(event)
    expect(answer).toEqual({ user: { sub: 'u' } })
    expect(JSON.stringify(answer)).not.toContain('at"')
  })
})

describe('POST /auth/logout', () => {
  const after = 'https://cms.example/login'
  const loggedIn = { accessToken: 'at', idToken: 'the-id-token' }
  const post = (sessions = {}) =>
    fakeEvent({ method: 'POST', headers: { origin: 'https://cms.example' }, sessions })

  // The provider's end_session_endpoint, answered per test. The proxy's tests
  // below stub fetch as well, so the stub is put back after each one.
  const endSession = vi.fn()
  let fetchBefore: typeof fetch
  beforeEach(() => {
    fetchBefore = globalThis.fetch
    vi.stubGlobal('fetch', endSession)
    client.discovery.mockResolvedValue({ serverMetadata: () => ({ end_session_endpoint: 'https://idp.example/logout' }) })
  })
  afterEach(() => {
    vi.stubGlobal('fetch', fetchBefore)
  })
  const sent = () => new URLSearchParams(String(endSession.mock.calls[0][1].body))
  const answer = (status: number, body = '', headers: Record<string, string> = {}) =>
    endSession.mockResolvedValue(new Response(body, { status, headers }))

  it('refuses a cross-origin logout and keeps the session', async () => {
    const event = post({ resonans_session: { ...loggedIn } })
    event.headers.origin = 'https://evil.example'
    await expect(logout(event)).rejects.toMatchObject({ statusCode: 403 })
    expect(event.sessions.resonans_session.accessToken).toBe('at')
    expect(endSession).not.toHaveBeenCalled()
  })

  it('clears the session and goes to /login when the provider has no logout', async () => {
    client.discovery.mockResolvedValue({ serverMetadata: () => ({}) })
    const event = post({ resonans_session: { ...loggedIn } })
    expect(await logout(event)).toEqual({ redirect: '/login' })
    expect(event.sessions.resonans_session).toEqual({})
    expect(endSession).not.toHaveBeenCalled()
  })

  it('asks the provider nothing when nobody is logged in', async () => {
    expect(await logout(post())).toEqual({ redirect: '/login' })
    expect(endSession).not.toHaveBeenCalled()
  })

  // The success path: the provider ends its session for the hint, and only then
  // is the local one cleared.
  it.each([
    ['a 302 to the registered URI', () => answer(302, '', { location: after })],
    ['a 200 JSON naming the registered URI', () => answer(200, JSON.stringify({ redirect: true, url: after }), { 'content-type': 'application/json' })],
  ])('ends the session at the provider with the ID token as hint, then here (%s)', async (_, provider) => {
    provider()
    const event = post({ resonans_session: { ...loggedIn } })
    expect(await logout(event)).toEqual({ redirect: '/login' })
    expect(String(endSession.mock.calls[0][0])).toBe('https://idp.example/logout')
    expect(endSession.mock.calls[0][1]).toMatchObject({ method: 'POST', redirect: 'manual' })
    expect(Object.fromEntries(sent())).toEqual({
      id_token_hint: 'the-id-token',
      client_id: 'resonans-editor',
      post_logout_redirect_uri: after,
    })
    expect(event.sessions.resonans_session).toEqual({})
  })

  // Every other answer is a failure, and then nothing pretends: the provider's
  // session is alive, so the local one stays and the editor is told.
  it.each([
    ['the provider asks for confirmation', () => answer(400, 'User confirmation is required')],
    ['the provider cannot verify the hint', () => answer(400, '{"error":"invalid_token"}', { 'content-type': 'application/json' })],
    ['the provider answers with an HTML page', () => answer(200, '<html>Log out?</html>', { 'content-type': 'text/html' })],
    ['the provider redirects somewhere unregistered', () => answer(302, '', { location: 'https://evil.example/' })],
    ['the provider answers JSON naming somewhere unregistered', () => answer(200, JSON.stringify({ redirect: true, url: 'https://evil.example/' }), { 'content-type': 'application/json' })],
    ['the provider answers JSON without redirect: true', () => answer(200, JSON.stringify({ url: after }), { 'content-type': 'application/json' })],
    ['the connection drops', () => endSession.mockRejectedValue(new TypeError('fetch failed'))],
  ])('%s: error, and still logged in', async (_, provider) => {
    provider()
    const event = post({ resonans_session: { ...loggedIn } })
    await expect(logout(event)).rejects.toMatchObject({ statusCode: 502, message: expect.stringContaining('still logged in') })
    expect(event.sessions.resonans_session).toEqual(loggedIn)
  })

  // The exact text of the 502, with and without a response body to quote.
  it.each([
    ['with a body', () => answer(400, 'User confirmation is required'), 'the provider did not end the session (400: User confirmation is required); you are still logged in'],
    ['without a body', () => answer(302, '', { location: 'https://evil.example/' }), 'the provider did not end the session (302); you are still logged in'],
  ])('names the provider\'s answer in the error (%s)', async (_, provider, message) => {
    provider()
    await expect(logout(post({ resonans_session: { ...loggedIn } }))).rejects.toMatchObject({ statusCode: 502, message })
  })

  it('refuses when the session holds no ID token, and keeps it', async () => {
    // A session from before the ID token was kept.
    const event = post({ resonans_session: { accessToken: 'at' } })
    await expect(logout(event)).rejects.toMatchObject({ statusCode: 409, message: expect.stringContaining('still logged in') })
    expect(endSession).not.toHaveBeenCalled()
    expect(event.sessions.resonans_session).toEqual({ accessToken: 'at' })
  })
})

describe('/proxy', () => {
  const upstream = vi.fn()
  vi.stubGlobal('fetch', upstream)
  const loggedIn = { resonans_session: { accessToken: 'the-access-token' } }
  const call = (over: Partial<FakeEvent> = {}) =>
    proxy(fakeEvent({ path: 'api/v1/schema', sessions: loggedIn, ...over }))

  beforeEach(() => {
    upstream.mockImplementation(
      async () => new Response('{}', { status: 200, headers: { 'content-type': 'application/json' } }),
    )
  })

  it('adds the bearer on the server', async () => {
    await call()
    const [url, init] = upstream.mock.calls[0]
    expect(String(url)).toBe('http://api:3000/api/v1/schema')
    expect(init.headers.authorization).toBe('Bearer the-access-token')
  })

  it('is 401 without a session and calls nothing', async () => {
    await expect(call({ sessions: {} })).rejects.toMatchObject({ statusCode: 401 })
    expect(upstream).not.toHaveBeenCalled()
  })

  it('forwards the listed headers and nothing else', async () => {
    await call({
      headers: {
        accept: 'application/json',
        'if-match': '"v1"',
        cookie: 'resonans_session=sealed',
        authorization: 'Bearer from-the-browser',
        'x-dev-tenant': 'other-tenant',
        'x-scope-orgid': 'other-tenant',
      },
    })
    expect(upstream.mock.calls[0][1].headers).toEqual({
      authorization: 'Bearer the-access-token',
      accept: 'application/json',
      'if-match': '"v1"',
    })
  })

  it('does not return headers off the list', async () => {
    upstream.mockResolvedValue(
      new Response('{}', {
        status: 200,
        headers: { 'content-type': 'application/json', etag: '"v1"', 'set-cookie': 'a=b', 'x-internal': 'c' },
      }),
    )
    const event = fakeEvent({ path: 'api/v1/schema', sessions: loggedIn })
    await proxy(event)
    expect(Object.keys(event.response.headers).sort()).toEqual(['content-type', 'etag'])
  })

  it.each([
    'delivery/v1/post',
    '_assets/a.png',
    'healthz',
    'api/v2/schema',
    'api/v1/../../delivery/v1/post',
    'api/v1/%2e%2e/x',
    'api/v1/%2E%2E/%2E%2E/delivery/v1/post',
    'api/v1/.%2e/x',
  ])('refuses the path %s', async (path) => {
    await expect(call({ path })).rejects.toMatchObject({ statusCode: 404 })
    expect(upstream).not.toHaveBeenCalled()
  })

  it('refuses a cross-origin write before it reads a token', async () => {
    await expect(call({ method: 'POST', headers: { origin: 'https://evil.example' } })).rejects.toMatchObject({
      statusCode: 403,
    })
    expect(upstream).not.toHaveBeenCalled()
  })

  it('passes the status and the body back', async () => {
    upstream.mockResolvedValue(new Response('nope', { status: 409 }))
    const event = fakeEvent({ path: 'api/v1/documents', sessions: loggedIn })
    const body = await proxy(event)
    expect(event.response.status).toBe(409)
    expect(new TextDecoder().decode(body)).toBe('nope')
  })

  it('does not follow an upstream redirect with the token', async () => {
    await call()
    expect(upstream.mock.calls[0][1].redirect).toBe('manual')
  })

  it('sends a body on a write and none on a read', async () => {
    const bytes = new TextEncoder().encode('{"a":1}')
    await call({ method: 'POST', headers: { origin: 'https://cms.example' }, body: bytes })
    expect(new TextDecoder().decode(upstream.mock.calls[0][1].body)).toBe('{"a":1}')
    await call({ body: bytes })
    expect(upstream.mock.calls[1][1].body).toBeUndefined()
  })
})
