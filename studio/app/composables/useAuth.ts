// Studio login against the shared BetterAuth service.
//
// 🔴 THE OPT-IN IS POSITIVE, AND IT MIRRORS THE SERVER'S.
//    `authBase` empty  -> dev mode: no login, the tenant comes from
//                         X-Dev-Tenant, exactly as before this file existed.
//    `authBase` set    -> login required, and every editor call carries
//                         `Authorization: Bearer <jwt>`.
//    The server decides the same way (config.EditorDevAuth: JWT configured
//    wins, otherwise RESONANS_DEV_AUTH must be exactly "1", never in
//    production). One side cannot quietly drift into trusting a header the
//    other side has stopped reading: both are driven by whether JWT is
//    configured, not by a build flag.
//
// 🔴 THE JWT LIVES IN MEMORY ONLY.
//    Not localStorage, not a cookie we set: a token in localStorage survives
//    the tab, is readable by any script that gets into the page, and outlives
//    the sign-out the user thought they performed. The session cookie is
//    BetterAuth's to own; the JWT is a short-lived derivative we re-mint.
//
// The auth ORIGIN is runtime configuration, not a build-time constant, because
// the choice of which host owns the session is a deployment decision (staging
// may answer on auth.staging.klanghaus.dk while prod answers on
// auth.klanghaus.dk). Reading it from the URL or guessing it from the current
// host would let any page that frames us point the login somewhere else.

export interface AuthUser {
  id: string
  email: string
  name?: string
}

interface TokenResponse {
  token?: string
}

// Module scope, deliberately: `useState` is serialised into the SSR payload,
// and a token must never travel in a document. The Studio is ssr:false today,
// so the payload is empty either way — but a future `ssr: true` must not turn
// this into a leak by accident.
let jwt: string | null = null

export function useAuth() {
  const cfg = useRuntimeConfig()
  const authBase = ((cfg.public.authBase as string) || '').replace(/\/$/, '')
  // `enabled` is the single source of truth for "is this a logged-in Studio?"
  const enabled = computed(() => authBase !== '')
  const user = useState<AuthUser | null>('auth-user', () => null)
  const ready = useState<boolean>('auth-ready', () => false)

  async function call<T>(path: string, opts: Record<string, unknown> = {}) {
    return $fetch<T>(`${authBase}${path}`, {
      // The session cookie belongs to the auth host; without credentials the
      // call is anonymous and every answer would be a truthful answer about
      // the wrong thing: "no session" for a user who has one.
      credentials: 'include',
      ...opts,
    })
  }

  /** Reads the current session. Null means "no session", and an unreachable
   *  auth service throws — the two must not collapse into the same answer. */
  async function refresh(): Promise<AuthUser | null> {
    if (!enabled.value) {
      ready.value = true
      return null
    }
    const s = await call<{ user?: AuthUser } | null>('/api/auth/get-session')
    user.value = s?.user ?? null
    ready.value = true
    return user.value
  }

  async function signIn(email: string, password: string): Promise<AuthUser | null> {
    await call('/api/auth/sign-in/email', {
      method: 'POST',
      body: { email, password },
      headers: { 'Content-Type': 'application/json' },
    })
    jwt = null // a new session must mint a new token, never reuse the old one
    return refresh()
  }

  async function signOut(): Promise<void> {
    jwt = null
    user.value = null
    if (!enabled.value) return
    await call('/api/auth/sign-out', { method: 'POST' })
  }

  /** Mints (or returns) the JWT the Resonans API requires. Returns null in dev
   *  mode, where there is no auth service to ask. */
  async function token(force = false): Promise<string | null> {
    if (!enabled.value) return null
    if (jwt && !force) return jwt
    const r = await call<TokenResponse>('/api/auth/token')
    jwt = r?.token ?? null
    return jwt
  }

  function forget(): void {
    jwt = null
  }

  return { enabled, user, ready, refresh, signIn, signOut, token, forget }
}
