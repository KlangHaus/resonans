import * as client from 'openid-client'

// Ends the editor session, and tells the browser where to end the provider's
// session too when the provider advertises an end_session_endpoint
// (RP-initiated logout). client_id identifies us there; no ID token is kept
// in the session, so none is sent as a hint.
export default defineEventHandler(async (event) => {
  const s = oidcSettings()
  if (!s) throw createError({ statusCode: 404 })
  assertSameOrigin(event, s)

  await (await editorSession(event, s)).clear()

  const config = await oidcConfig(s)
  if (!config.serverMetadata().end_session_endpoint) return { redirect: '/login' }
  const url = client.buildEndSessionUrl(config, {
    client_id: s.clientId,
    post_logout_redirect_uri: `${s.editorUrl}/login`,
  })
  return { redirect: url.href }
})
