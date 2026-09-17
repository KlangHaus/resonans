// Minimal frontend for trying visual editing locally — and the reference for
// what a real site (Etude, blog) has to do. No dependencies: node server.mjs.
//
//   RESONANS_URL   Resonans API origin        (default http://localhost:3999)
//   STUDIO_ORIGIN  the ONLY origin the bridge talks to (default http://localhost:3000)
//   PORT           this site's port           (default 4100)
//
// Tenant preview URL to enter in Studio → Indstillinger:
//   http://localhost:4100/preview/{type}/{slug}?token={token}&locale={locale}
//
// A real site does the same three things:
//   1. render the DRAFT server-side from GET /delivery/v1/preview/{type}/{slug}?token=…
//   2. load the bridge script on the preview route only, with its own studioOrigin
//   3. mark rendered values with data-resonans-path and re-render on onUpdate
import { createServer } from 'node:http'
import { readFile } from 'node:fs/promises'

const RESONANS_URL = process.env.RESONANS_URL || 'http://localhost:3999'
const STUDIO_ORIGIN = process.env.STUDIO_ORIGIN || 'http://localhost:3000'
const PORT = Number(process.env.PORT || 4100)
const STARTED = Date.now()

const page = await readFile(new URL('./index.html', import.meta.url), 'utf8')

createServer(async (req, res) => {
  const url = new URL(req.url, `http://localhost:${PORT}`)
  const m = url.pathname.match(/^\/preview\/([A-Za-z0-9_-]+)\/([A-Za-z0-9_-]+)$/)
  if (!m) {
    res.writeHead(404).end('not found')
    return
  }
  const [, type, slug] = m
  const token = url.searchParams.get('token') || ''
  const locale = url.searchParams.get('locale') || 'da'
  // (1) Server-side draft fetch: the token never needs CORS and the envelope
  // arrives in the first paint, exactly like an SSR site.
  const draft = await fetch(
    `${RESONANS_URL}/delivery/v1/preview/${type}/${slug}?token=${encodeURIComponent(token)}&locale=${encodeURIComponent(locale)}`,
  )
  const envelope = draft.ok ? await draft.json() : { error: `preview ${draft.status}` }
  const html = page
    // `?started=` busts the browser cache when Resonans is restarted with a new
    // bridge during local development; a real site can load the plain URL.
    .replace('__BRIDGE_SRC__', `${RESONANS_URL}/bridge/v1/resonans-bridge.js?started=${STARTED}`)
    .replace('__STUDIO_ORIGIN__', JSON.stringify(STUDIO_ORIGIN))
    // "<" escaped so a draft string cannot close the script tag.
    .replace('__ENVELOPE__', JSON.stringify(envelope).replace(/</g, '\\u003c'))
  res.writeHead(draft.ok ? 200 : 502, {
    'Content-Type': 'text/html; charset=utf-8',
    'Cache-Control': 'no-store',
    // Only the Studio may frame the preview.
    'Content-Security-Policy': `frame-ancestors ${STUDIO_ORIGIN}`,
    'Referrer-Policy': 'no-referrer',
  })
  res.end(html)
}).listen(PORT, () => {
  console.log(`visual-editing example on http://localhost:${PORT} (Resonans ${RESONANS_URL}, Studio ${STUDIO_ORIGIN})`)
})
