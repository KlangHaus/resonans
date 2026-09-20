// Route guard. Dev mode (no auth service configured) behaves exactly as before
// this file existed: no redirect, no login, tenant from X-Dev-Tenant.
//
// 🔴 FAIL CLOSED ON AN UNREACHABLE AUTH SERVICE.
//    If the session lookup throws — auth down, JWKS 500, DNS gone — we do NOT
//    fall through to the editor. "I could not ask" is not "you are allowed";
//    the whole staging incident (KH-1071) was a service answering 500 while
//    everything downstream kept looking normal. The editor is sent to /login,
//    which states that the service could not be reached.
export default defineNuxtRouteMiddleware(async (to) => {
  const { enabled, user, ready, refresh } = useAuth()
  if (!enabled.value) return
  if (to.path === '/login') return

  if (!ready.value) {
    try {
      await refresh()
    } catch {
      return navigateTo({ path: '/login', query: { grund: 'utilgaengelig', naeste: to.fullPath } })
    }
  }
  if (!user.value) {
    return navigateTo({ path: '/login', query: { naeste: to.fullPath } })
  }
})
