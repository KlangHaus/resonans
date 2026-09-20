// Resonans Studio — the editor SPA. It is schema-driven (renders forms + nav
// from GET /api/v1/schema) and theme-agnostic (styled via CSS custom properties
// so grundtone — or any other brand — can be swapped in without touching the
// components; product-grade requirement for the external-CMS track).
export default defineNuxtConfig({
  compatibilityDate: '2025-01-01',
  ssr: false, // admin editor = SPA; no SSR needed
  devtools: { enabled: false },
  css: ['~/assets/theme.css'],
  // The Studio frames tenant frontends (visual editing), but must never be
  // framed itself: a framed editor could be clickjacked into saving or publishing.
  routeRules: {
    '/**': {
      headers: {
        'Content-Security-Policy': "frame-ancestors 'none'",
        'X-Frame-Options': 'DENY',
      },
    },
  },
  // Local dev: proxy /api → the Resonans backend so the SPA is same-origin (no
  // CORS). Override the target via RESONANS_DEV_API. Prod serves Studio behind
  // the same origin as the API, so this proxy is dev-only.
  nitro: {
    devProxy: {
      '/api': {
        target: `${process.env.RESONANS_DEV_API || 'http://localhost:3999'}/api`,
        changeOrigin: true,
      },
    },
  },
  runtimeConfig: {
    public: {
      // Sentry (SaaS EU) — DSN er publishable; tom = Sentry slået fra.
      sentryDsn: process.env.NUXT_PUBLIC_SENTRY_DSN || '',
      sentryEnv: process.env.NUXT_PUBLIC_SENTRY_ENV || 'staging',
      // Resonans backend base URL.
      apiBase: '',
      // Dev-only tenant (sent as X-Dev-Tenant when auth is disabled). In prod
      // the BetterAuth session/JWT supplies the tenant; never trust this header
      // server-side in prod.
      devTenant: '',
      // 🔴 THE LOGIN SWITCH, AND IT IS A POSITIVE OPT-IN.
      // Empty = no auth service for this environment: no login, dev tenant
      // header, exactly as before. Set = login required and every editor call
      // carries a Bearer token. Which host owns the session is a DEPLOYMENT
      // decision (staging and prod answer on different names), so it is
      // runtime config — never read from the URL, which would let a page that
      // frames us point the login somewhere else.
      authBase: '',
    },
  },
})
