// Resonans Studio — the editor SPA. It is schema-driven (renders forms + nav
// from GET /api/v1/schema) and theme-agnostic (styled via CSS custom properties
// so grundtone — or any other brand — can be swapped in without touching the
// components; product-grade requirement for the external-CMS track).
export default defineNuxtConfig({
  compatibilityDate: '2025-01-01',
  ssr: false, // admin editor = SPA; no SSR needed
  devtools: { enabled: false },
  css: ['~/assets/theme.css'],
  runtimeConfig: {
    public: {
      // Resonans backend base URL.
      apiBase: '',
      // Dev-only tenant (sent as X-Dev-Tenant when auth is disabled). In prod
      // the BetterAuth session/JWT supplies the tenant; never trust this header
      // server-side in prod.
      devTenant: '',
    },
  },
})
