// Build identity for the Studio image. Studio is an SPA (ssr: false), but Nitro
// still serves server routes — so the RUNNING container can be asked what it is
// without trusting compose, the deploy agent or a mutable tag (Beslutning AJ).
//
// Read from process.env at REQUEST time, not from runtimeConfig: runtimeConfig
// is baked at build time, and the whole point is that one image can be probed
// wherever it runs.
export default defineEventHandler(() => ({
  status: 'ok',
  service: 'resonans-studio',
  version: process.env.RESONANS_STUDIO_VERSION || '0.0.0',
  revision: process.env.RESONANS_STUDIO_REVISION || 'unknown',
}))
