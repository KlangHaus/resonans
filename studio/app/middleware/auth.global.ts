// Route guard. The first navigation asks the editor's server which mode it is
// in (useAuth: /auth/me). Dev mode (no OIDC configured) behaves exactly as
// before: no redirect, no login, tenant from X-Dev-Tenant.
//
// 🔴 FAIL CLOSED ON AN UNREACHABLE SERVER.
//    If the lookup throws — server down, provider unreachable, a 500 — we do
//    NOT fall through to the editor. "I could not ask" is not "you are
//    allowed"; the whole staging incident (KH-1071) was a service answering 500
//    while everything downstream kept looking normal. The editor is sent to
//    /login, which states that the service could not be reached.
export default defineNuxtRouteMiddleware(async (to) => {
  if (to.path === '/login') return
  const { enabled, user, ready, refresh } = useAuth()

  if (!ready.value) {
    try {
      await refresh()
    } catch {
      return navigateTo({ path: '/login', query: { grund: 'utilgaengelig', naeste: to.fullPath } })
    }
  }
  if (!enabled.value) return
  if (!user.value) {
    return navigateTo({ path: '/login', query: { naeste: to.fullPath } })
  }
})
