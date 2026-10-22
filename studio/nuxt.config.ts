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
    // 🔴 THE LOGIN SWITCH, AND IT IS A POSITIVE OPT-IN (ADR 0001).
    // Server-only: none of this reaches the page. Empty issuer = dev mode (no
    // login, X-Dev-Tenant). Set = the editor's server is an OIDC client and
    // every editor call goes through /proxy with the session's token. Set from
    // the environment as NUXT_OIDC_<KEY>, e.g. NUXT_OIDC_ISSUER. Half a
    // configuration (an issuer without the rest) is refused at the first
    // request rather than falling back to dev mode.
    oidc: {
      issuer: '',
      clientId: '',
      clientSecret: '',
      scopes: 'openid profile email',
      // The editor's public origin; the redirect URI is <editorUrl>/auth/callback.
      editorUrl: '',
      // Where this server reaches the Resonans API (e.g. http://resonans:3000).
      apiUrl: '',
      // Seals the session cookie. At least 32 characters; generate with
      // `openssl rand -base64 32`.
      sessionSecret: '',
      // "1" allows plain-http discovery and token calls. Local provider only.
      allowInsecure: '',
    },
    public: {
      // Sentry (SaaS EU) — DSN er publishable; tom = Sentry slået fra.
      sentryDsn: process.env.NUXT_PUBLIC_SENTRY_DSN || '',
      sentryEnv: process.env.NUXT_PUBLIC_SENTRY_ENV || 'staging',
      // Resonans backend base URL.
      apiBase: '',
      // Dev-only tenant (sent as X-Dev-Tenant when OIDC is not configured). With
      // OIDC the token supplies the tenant; the API never reads this header
      // when JWT auth is on.
      devTenant: '',
    },
  },
})
