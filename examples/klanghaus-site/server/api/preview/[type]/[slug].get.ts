// GET /api/preview/:type/:slug?token=…&locale=… → the DRAFT envelope, for the
// editor's preview iframe. The token is what Resonans issued to the editor
// (scoped to one tenant, type and slug, 15 minutes); Resonans checks it. No
// token, a wrong one or an expired one is an error, never content.
export default defineEventHandler(async (event) => {
  const type = getRouterParam(event, 'type') ?? ''
  const slug = getRouterParam(event, 'slug') ?? ''
  const { token, locale } = getQuery(event)
  setResponseHeader(event, 'Cache-Control', 'no-store')
  if (typeof token !== 'string' || !token) {
    throw createError({ statusCode: 401, statusMessage: 'preview token required' })
  }
  const c = useRuntimeConfig()
  const headers: Record<string, string> = {}
  if (c.resonansDeliveryToken) headers.Authorization = `Bearer ${c.resonansDeliveryToken}`
  const base = String(c.resonansUrl).replace(/\/$/, '')
  try {
    return await $fetch(`${base}/delivery/v1/preview/${encodeURIComponent(type)}/${encodeURIComponent(slug)}`, {
      query: { token, locale: typeof locale === 'string' && locale ? locale : 'da' },
      headers,
    })
  } catch (e: unknown) {
    const status = (e as { statusCode?: number })?.statusCode ?? 502
    throw createError({ statusCode: status, statusMessage: 'preview not available' })
  }
})
