// Editor login (ADR 0001). The editor's own server is the OpenID Connect
// client; this composable only asks it who is logged in and sends the browser
// to /auth/login or /auth/logout. It never sees a token.
//
// 🔴 THE MODE COMES FROM THE SERVER, NOT FROM A BUILD FLAG.
//    GET /auth/me answers 404 when no OIDC issuer is configured (dev mode: no
//    login, tenant from X-Dev-Tenant, exactly as before), 401 when OIDC is on
//    and nobody is logged in, 200 with the user otherwise. The server decides
//    both sides — whether to require a login and how the API is reached — so
//    the page cannot drift into trusting a header the server no longer reads.
//
// 🔴 AN UNREACHABLE SERVER IS NOT "LOGGED OUT".
//    Any other status throws. "I could not ask" must not collapse into "no
//    session": the route guard shows that as its own message.
export interface AuthUser {
  sub: string
  email?: string
  name?: string
}

export type AuthMode = 'unknown' | 'dev' | 'oidc'

export function useAuth() {
  const mode = useState<AuthMode>('auth-mode', () => 'unknown')
  const user = useState<AuthUser | null>('auth-user', () => null)
  const ready = useState<boolean>('auth-ready', () => false)
  // `enabled` is the single source of truth for "is this a logged-in editor?"
  const enabled = computed(() => mode.value === 'oidc')

  /** Reads the current session. Null means "no session" (or dev mode). */
  async function refresh(): Promise<AuthUser | null> {
    try {
      const r = await $fetch<{ user: AuthUser }>('/auth/me')
      mode.value = 'oidc'
      user.value = r.user
    } catch (e: unknown) {
      const status = (e as { statusCode?: number; status?: number })?.statusCode ?? (e as { status?: number })?.status
      if (status === 404) mode.value = 'dev'
      else if (status === 401) mode.value = 'oidc'
      else throw e
      user.value = null
    }
    ready.value = true
    return user.value
  }

  /** Leaves the page for the provider's login. `next` must be a path here;
   *  the server re-checks it, so this is a convenience, not the guard. */
  function signIn(next = '/'): void {
    window.location.assign(`/auth/login?next=${encodeURIComponent(next)}`)
  }

  /** Ends the session. Resolves to true when the browser is being sent away
   *  (to the provider's logout), so the caller should not navigate itself. */
  async function signOut(): Promise<boolean> {
    user.value = null
    if (mode.value !== 'oidc') return false
    const r = await $fetch<{ redirect: string }>('/auth/logout', { method: 'POST' })
    window.location.assign(r.redirect)
    return true
  }

  return { mode, enabled, user, ready, refresh, signIn, signOut }
}
