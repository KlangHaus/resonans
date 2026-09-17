// Visual editing — the Studio side of the bridge protocol (v1). Pure functions,
// kept apart from the component so the protocol rules can be read in one place.
//
// Paths: the preview marks elements with the path of the value in the DELIVERY
// ENVELOPE ("data.seoTitle", "components.2.heading"), because that is the shape
// the frontend renders. The form addresses the document BODY ("seoTitle",
// "body.2.heading"). The only difference is the root segment: `data.*` fields
// sit at the body root, and `components` is the doc-type's blocks field.

export const BRIDGE_VERSION = 1
export const FROM_STUDIO = 'resonans-studio'
export const FROM_BRIDGE = 'resonans-bridge'

// Same grammar as the bridge: dot-separated [A-Za-z0-9_-] segments, max 20.
const PATH_RE = /^[A-Za-z0-9_-]+(\.[A-Za-z0-9_-]+){0,19}$/

export function isValidPath(p: unknown): p is string {
  return typeof p === 'string' && PATH_RE.test(p)
}

export function envelopeToBodyPath(path: string, blocksField: string | undefined): string | null {
  if (!isValidPath(path)) return null
  const [root, ...rest] = path.split('.')
  if (root === 'data' && rest.length) return rest.join('.')
  if (root === 'components' && blocksField) return [blocksField, ...rest].join('.')
  return null
}

export function bodyToEnvelopePath(path: string, blocksField: string | undefined): string | null {
  if (!isValidPath(path)) return null
  const [root, ...rest] = path.split('.')
  if (blocksField && root === blocksField) return ['components', ...rest].join('.')
  return `data.${path}`
}

// fillPreviewUrl expands the tenant's template. Every value is URL-encoded, so
// a slug cannot inject query parameters or path segments.
export function fillPreviewUrl(
  template: string,
  v: { type: string; slug: string; token: string; locale: string },
): string {
  return template
    .replaceAll('{type}', encodeURIComponent(v.type))
    .replaceAll('{slug}', encodeURIComponent(v.slug))
    .replaceAll('{token}', encodeURIComponent(v.token))
    .replaceAll('{locale}', encodeURIComponent(v.locale))
}

export type BridgeMessage =
  | { type: 'ready'; href: string }
  | { type: 'select'; path: string }

// parseBridgeMessage accepts a message ONLY from the preview iframe's window AND
// the configured preview origin; anything else is ignored (returns null).
export function parseBridgeMessage(
  e: { origin: string; source: unknown; data: unknown },
  expectedOrigin: string,
  expectedSource: unknown,
): BridgeMessage | null {
  // The fallback preview runs in an opaque origin, which the browser reports as
  // the string "null"; it is accepted ONLY together with the exact frame window.
  if (!expectedOrigin || e.origin !== expectedOrigin) return null
  if (!expectedSource || e.source !== expectedSource) return null
  const d = e.data as Record<string, unknown> | null
  if (!d || typeof d !== 'object' || d.source !== FROM_BRIDGE || d.v !== BRIDGE_VERSION) return null
  if (d.type === 'ready') return { type: 'ready', href: String(d.href ?? '') }
  if (d.type === 'select' && isValidPath(d.path)) return { type: 'select', path: d.path }
  return null
}
