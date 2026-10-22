import * as client from 'openid-client'

// Starts the OIDC login: authorization code + PKCE (S256), with state and nonce.
export default defineEventHandler(async (event) => {
  const s = oidcSettings()
  if (!s) throw createError({ statusCode: 404 })

  const config = await oidcConfig(s)
  const verifier = client.randomPKCECodeVerifier()
  const state = client.randomState()
  const nonce = client.randomNonce()

  // Only a path on this site may be the destination after login. Anything
  // else — another host, a protocol-relative `//evil`, a backslash trick — is
  // replaced by the start page instead of becoming an open redirect.
  const raw = String(getQuery(event).next ?? '/')
  const next = /^\/(?![/\\])/.test(raw) ? raw : '/'

  const tx = await loginTransaction(event, s)
  await tx.update({ verifier, state, nonce, next })

  const url = client.buildAuthorizationUrl(config, {
    redirect_uri: redirectUri(s),
    scope: s.scopes,
    code_challenge: await client.calculatePKCECodeChallenge(verifier),
    code_challenge_method: 'S256',
    state,
    nonce,
  })
  return sendRedirect(event, url.href, 302)
})
