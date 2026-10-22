// GET /api/posts → the published blog posts, newest first.
export default defineEventHandler(() => listDocuments('post'))
