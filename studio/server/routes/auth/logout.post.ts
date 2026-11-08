// Ends the session at the provider and then here (OIDC RP-Initiated Logout). A
// POST with the same-origin check, so a foreign page cannot log someone out.
//
// This server makes the call to the provider's end_session_endpoint, with the ID
// token as id_token_hint, so the answer can be read. Two answers count as
// success, and both must name exactly the registered post-logout URI:
//   - a 302 with that Location;
//   - a 200 with {"redirect": true, "url": <that URI>}, which some providers
//     (KlangHaus auth among them) send to a request they read as a fetch.
//
// 🔴 ANYTHING ELSE IS AN ERROR, AND THE LOCAL SESSION STAYS. A provider that did
// not end its session would sign the editor straight back in on the next login,
// so clearing ours anyway would show "logged out" when it is not true.
//
// A provider that advertises no end_session_endpoint offers no logout to call:
// then only the editor session ends, as before, unless
// NUXT_OIDC_REQUIRE_PROVIDER_LOGOUT=1 says the provider must end it. Then a
// missing endpoint is an error too: a discovery document that lost it would
// otherwise turn every logout into a local one without anyone noticing.
// Ported from KlangHaus/backstage web/server/routes/auth/logout.post.ts.
export default defineEventHandler(async (event) => {
  const s = oidcSettings()
  if (!s) throw createError({ statusCode: 404 })
  assertSameOrigin(event, s)

  const session = await editorSession(event, s)
  const endpoint = (await oidcConfig(s)).serverMetadata().end_session_endpoint
  if (!session.data.accessToken) {
    await session.clear()
    return { redirect: '/login' }
  }
  if (!endpoint) {
    if (s.requireProviderLogout) {
      throw createError({ statusCode: 502, statusMessage: 'the provider offers no logout; you are still logged in' })
    }
    await session.clear()
    return { redirect: '/login' }
  }

  const idToken = session.data.idToken
  if (!idToken) {
    throw createError({
      statusCode: 409,
      statusMessage: 'this session has no ID token, so the provider cannot be asked to end it; you are still logged in',
    })
  }

  const after = postLogoutRedirectUri(s)
  let res: Response
  try {
    res = await fetch(endpoint, {
      method: 'POST',
      headers: { 'content-type': 'application/x-www-form-urlencoded', accept: '*/*' },
      body: new URLSearchParams({ id_token_hint: idToken, client_id: s.clientId, post_logout_redirect_uri: after }),
      redirect: 'manual',
    })
  } catch {
    throw createError({ statusCode: 502, statusMessage: 'the provider could not be reached; you are still logged in' })
  }

  if (!(await endedAt(res, after))) {
    const detail = await res.text().then((t) => t.slice(0, 200)).catch(() => '')
    const answer = detail ? `${res.status}: ${detail}` : String(res.status)
    throw createError({
      statusCode: 502,
      statusMessage: `the provider did not end the session (${answer}); you are still logged in`,
    })
  }

  await session.clear()
  return { redirect: '/login' }
})

/** Whether the provider's answer says it ended the session and sends the editor to `after`. */
async function endedAt(res: Response, after: string): Promise<boolean> {
  if (res.status === 302) return res.headers.get('location') === after
  if (res.status !== 200 || !res.headers.get('content-type')?.includes('application/json')) return false
  const body = (await res.clone().json().catch(() => null)) as { redirect?: unknown; url?: unknown } | null
  return body?.redirect === true && body.url === after
}
