// The preview route shows unpublished drafts, so it is fenced off:
//   frame-ancestors  only the editor may frame it (the origin comes from this
//                    site's configuration, never from the URL)
//   no-store         a draft is never cached, by a browser or anything between
//   noindex          and never indexed, should a token URL leak
// The public pages are untouched.
export default defineEventHandler((event) => {
  if (!event.path.startsWith('/preview/')) return
  const studio = String(useRuntimeConfig().public.studioOrigin || '')
  setResponseHeaders(event, {
    'Content-Security-Policy': `frame-ancestors ${studio || "'none'"}`,
    'Cache-Control': 'no-store',
    'X-Robots-Tag': 'noindex, nofollow',
  })
})
