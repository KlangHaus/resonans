// klanghaus.dk, rebuilt on Resonans: every word and photo on these pages comes
// from Resonans' delivery API; the front-end only decides how it looks. It is
// the same site as https://klanghaus.dk — same grundtone components, same
// styles — so it doubles as the test bed for moving that page onto the CMS.
//
// Server-side only: the delivery token and the tenant never reach the browser
// (server/utils/resonans.ts). Run it through Resonans' Tiltfile, or see README.
export default defineNuxtConfig({
  compatibilityDate: '2026-10-01',
  devtools: { enabled: false },
  app: {
    head: {
      htmlAttrs: { lang: 'da' },
      meta: [
        { charset: 'utf-8' },
        { name: 'viewport', content: 'width=device-width, initial-scale=1' },
        { name: 'theme-color', content: '#16213e' },
      ],
      link: [{ rel: 'icon', type: 'image/svg+xml', href: '/klanghaus-logo.svg' }],
    },
  },
  build: {
    transpile: ['@grundtone/vue', '@grundtone/design-system'],
  },
  css: [
    '@grundtone/vue/css',
    // Self-hosted fonts, as on klanghaus.dk: latin + latin-ext only.
    '@fontsource/ibm-plex-sans/latin-400.css',
    '@fontsource/ibm-plex-sans/latin-600.css',
    '@fontsource/ibm-plex-sans/latin-700.css',
    '@fontsource/ibm-plex-sans/latin-ext-400.css',
    '@fontsource/ibm-plex-sans/latin-ext-600.css',
    '@fontsource/ibm-plex-sans/latin-ext-700.css',
    '@fontsource/ibm-plex-mono/latin-600.css',
    // brand.scss + landing.scss are klanghaus.dk's own, copied unchanged;
    // site.scss adds only what that one-pager never needed (blog, generic pages).
    '~/assets/styles/brand.scss',
    '~/assets/styles/landing.scss',
    '~/assets/styles/site.scss',
  ],
  runtimeConfig: {
    // Where this server reaches Resonans, and as whom. NUXT_RESONANS_URL etc.
    resonansUrl: 'http://localhost:3000',
    resonansTenant: '11111111-1111-1111-1111-111111111111',
    // Empty in Tilt: the dev API runs with RESONANS_DEV_AUTH=1 and trusts the
    // tenant header. Any real deployment sets RESONANS_DELIVERY_TOKEN.
    resonansDeliveryToken: '',
  },
})
