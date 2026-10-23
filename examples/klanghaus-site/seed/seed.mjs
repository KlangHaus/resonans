#!/usr/bin/env node
// Seeds this example's pages into a Resonans running in dev mode (the Tiltfile's
// API: RESONANS_DEV_AUTH=1, tenant via X-Dev-Tenant). It goes through the same
// editor API the editor uses — upload, create or update, publish — so a seed
// that works is also a small end-to-end check of that API.
//
// Safe to run again: a page that already exists (same slug) is updated and
// re-published, not duplicated. Images are uploaded on every run (cheap; the
// old ones stay in the asset list).
//
//   RESONANS_URL=http://localhost:3000 RESONANS_TENANT=<uuid> node seed/seed.mjs
import { readFile } from 'node:fs/promises'
import { pages } from './content.mjs'

const API = (process.env.RESONANS_URL || 'http://localhost:3000').replace(/\/$/, '')
const TENANT = process.env.RESONANS_TENANT || '11111111-1111-1111-1111-111111111111'
// Where this site answers, for the editor's live preview (visual editing).
const SITE = (process.env.SITE_URL || 'http://localhost:3050').replace(/\/$/, '')
const headers = { 'X-Dev-Tenant': TENANT }

async function call(method, path, body) {
  const init = { method, headers: { ...headers } }
  if (body instanceof FormData) init.body = body
  else if (body !== undefined) {
    init.headers['Content-Type'] = 'application/json'
    init.body = JSON.stringify(body)
  }
  const res = await fetch(`${API}${path}`, init)
  const text = await res.text()
  if (!res.ok) throw new Error(`${method} ${path} → ${res.status} ${text}`)
  return text ? JSON.parse(text) : null
}

async function upload(name) {
  const form = new FormData()
  const bytes = await readFile(new URL(`./images/${name}.jpg`, import.meta.url))
  form.append('file', new Blob([bytes], { type: 'image/jpeg' }), `${name}.jpg`)
  const asset = await call('POST', '/api/v1/assets', form)
  return asset.id
}

// Replace every img('name') marker with the uploaded asset id.
async function resolveImages(value, ids) {
  if (Array.isArray(value)) return Promise.all(value.map((v) => resolveImages(v, ids)))
  if (value && typeof value === 'object') {
    if ('__img' in value) {
      if (!ids.has(value.__img)) ids.set(value.__img, await upload(value.__img))
      return ids.get(value.__img)
    }
    const out = {}
    for (const [k, v] of Object.entries(value)) out[k] = await resolveImages(v, ids)
    return out
  }
  return value
}

const health = await fetch(`${API}/healthz`).catch(() => null)
if (!health?.ok) {
  console.error(`Resonans is not answering at ${API}/healthz — start it first (tilt up).`)
  process.exit(1)
}

const existing = (await call('GET', '/api/v1/documents?type=page')).documents ?? []
const ids = new Map()
for (const p of pages) {
  const body = await resolveImages(p.body, ids)
  const found = existing.find((d) => d.slug === p.slug)
  const doc = found
    ? await call('PUT', `/api/v1/documents/${found.id}`, { title: p.title, body })
    : await call('POST', '/api/v1/documents', { type: 'page', slug: p.slug, title: p.title, body })
  await call('POST', `/api/v1/documents/${doc.id}/publish`)
  console.log(`${found ? 'updated' : 'created'}  page/${p.slug}  ${doc.id}`)
}
// Visual editing: the editor frames this URL. {token} is required by Resonans
// (http only for localhost); the site's /preview route renders the draft.
const previewUrl = `${SITE}/preview/{type}/{slug}?token={token}&locale={locale}`
const set = await call('PUT', '/api/v1/settings/preview', { previewUrl })
console.log(`preview URL  ${set.previewUrl ?? previewUrl}`)
console.log(`${ids.size} images uploaded · tenant ${TENANT}`)
