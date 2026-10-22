// GET /api/posts/:slug → one published post, or 404. The delivery envelope
// carries the post's fields but not its document title, so the title is taken
// from the catalogue entry with the same slug.
export default defineEventHandler(async (event) => {
  const slug = getRouterParam(event, 'slug') ?? ''
  const [post, all] = await Promise.all([getDocument('post', slug), listDocuments('post')])
  if (!post) throw createError({ statusCode: 404, statusMessage: 'post not found' })
  return { ...post, title: all.find((p) => p.slug === slug)?.title ?? slug }
})
