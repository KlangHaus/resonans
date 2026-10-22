// The editor's only way to the Resonans API when OIDC is on: /proxy/api/v1/…
// is forwarded to the API with the session's bearer token added here.
//
// 🔴 WHAT IS FORWARDED IS A LIST, NOT "EVERYTHING BUT".
//    The browser's cookies (our sealed session), any Authorization it sends,
//    and X-Dev-Tenant never reach the API. Only the headers below do. A header
//    added to this list is a decision; a header that slips through a deny-list
//    is an accident.
const FORWARD_REQUEST = ['accept', 'content-type', 'if-none-match', 'if-match']
const FORWARD_RESPONSE = ['content-type', 'content-disposition', 'cache-control', 'etag', 'location']

export default defineEventHandler(async (event) => {
  const s = oidcSettings()
  if (!s) throw createError({ statusCode: 404 })
  assertSameOrigin(event, s)

  const path = getRouterParam(event, 'path') ?? ''
  // The editor API only. Not /delivery, not /_assets, not ../anything.
  if (!/^api\/v1\/[\w\-./{}%]*$/.test(path) || path.includes('..')) {
    throw createError({ statusCode: 404 })
  }

  const token = await accessToken(event, s)
  if (!token) throw createError({ statusCode: 401 })

  const headers: Record<string, string> = { authorization: `Bearer ${token}` }
  for (const h of FORWARD_REQUEST) {
    const v = getRequestHeader(event, h)
    if (v) headers[h] = v
  }

  const target = new URL(`${s.apiUrl}/${path}`)
  target.search = new URL(getRequestURL(event)).search
  const raw = event.method === 'GET' || event.method === 'HEAD' ? undefined : await readRawBody(event, false)
  const body = raw ? new Uint8Array(raw) : undefined

  const res = await fetch(target, { method: event.method, headers, body, redirect: 'manual' })
  setResponseStatus(event, res.status, res.statusText)
  for (const h of FORWARD_RESPONSE) {
    const v = res.headers.get(h)
    if (v) setResponseHeader(event, h, v)
  }
  return new Uint8Array(await res.arrayBuffer())
})
