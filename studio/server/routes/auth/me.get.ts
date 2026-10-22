// Who is logged in. 401 is "no session"; the editor's route guard sends the
// user to /login on it. It never returns a token.
export default defineEventHandler(async (event) => {
  const s = oidcSettings()
  if (!s) throw createError({ statusCode: 404 })
  const at = await accessToken(event, s)
  const { user } = (await editorSession(event, s)).data
  if (!at || !user) throw createError({ statusCode: 401 })
  return { user }
})
