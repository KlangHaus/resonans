// GET /api/pages/:slug → the published `page` envelope, or 404.
export default defineEventHandler(async (event) => {
  const slug = getRouterParam(event, 'slug') ?? ''
  const page = await getDocument('page', slug)
  if (!page) throw createError({ statusCode: 404, statusMessage: 'page not found' })
  return page
})
