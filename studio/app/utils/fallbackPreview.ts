// Fallback preview — what visual editing shows when the space has no frontend
// preview URL. It renders the live delivery envelope generically, driven by the
// schema, inside a sandboxed iframe. It is NOT the site: the banner says so.
//
// Security model (three independent layers — each must hold on its own):
//   1. Rendering: every draft string is written as escaped text; the ONLY
//      markup that reaches the page is rich text, and it passes sanitizeRichText
//      (DOMPurify, formatting tags only, no attributes except a checked href).
//   2. The iframe: `sandbox="allow-scripts"` WITHOUT allow-same-origin, so the
//      document runs in an opaque origin — it cannot read the Studio's cookies,
//      storage or DOM even if markup got through.
//   3. CSP inside the document: scripts only with a per-render nonce, so an
//      injected <script> or inline handler (onerror=…) does not run even if (1)
//      failed; no network (default-src 'none'), images only over https/data.
import { sanitizeRichText } from '~/utils/sanitize'
import type { BlockType, Field } from '~/composables/useResonans'
import { BRIDGE_VERSION, FROM_BRIDGE, FROM_STUDIO } from '~/utils/visualEditing'

type Env = { data?: Record<string, unknown>; components?: Record<string, unknown>[] }

function esc(v: unknown): string {
  return String(v ?? '')
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;')
}

// Rich text may be HTML or portable text; portable text is shown as its plain
// text (a faithful renderer is the site's job, not the fallback's).
function richText(v: unknown): string {
  if (typeof v === 'string') return sanitizeRichText(v)
  const texts: string[] = []
  const walk = (n: unknown) => {
    if (Array.isArray(n)) n.forEach(walk)
    else if (n && typeof n === 'object') {
      const o = n as Record<string, unknown>
      if (typeof o.text === 'string') texts.push(o.text)
      Object.values(o).forEach(walk)
    }
  }
  walk(v)
  return texts.length ? `<p>${esc(texts.join(' '))}</p>` : ''
}

function safeImageUrl(u: unknown): string | null {
  return typeof u === 'string' && /^https?:\/\//i.test(u) ? u : null
}

// Block icons — the same glyph family as the editor panel's block cards.
const ICONS: Record<string, string> = {
  rich_text: '¶', video_embed: '▶', image: '▦', callout: '!', quiz: '?', code: '</>',
  download: '⤓', iframe_embed: '▭', hero: '★', section_header: '§', cta: '→', quote: '“', feature_grid: '▦', columns: '▥',
}

function label(type: string): string {
  const t = type.replace(/_/g, '-')
  return t.charAt(0).toUpperCase() + t.slice(1)
}

// Short, plain representation of a value for a card's meta line.
function short(f: Field, v: unknown): string {
  if (f.type === 'asset') {
    const a = v as Record<string, unknown>
    return String(a?.alt ? `"${a.alt}"` : (a?.url ?? 'asset'))
  }
  if (f.type === 'reference') {
    const r = v as Record<string, unknown>
    return String(r?.slug ?? r?.id ?? '')
  }
  if (typeof v === 'object') return f.type
  const s = String(v)
  return s.length > 80 ? `${s.slice(0, 80)}…` : s
}

const PRIMARY = ['heading', 'title', 'quote', 'label', 'value', 'caption', 'videoId']

// One card = one block (forlæg: .pv__gblk). The primary field fills the body;
// every other filled field is a `name: value` item in the meta line. Every
// element carries the same data-resonans-path contract as the real bridge, so
// selection works identically — only the VIEW is generic.
function card(path: string, type: string, fields: Field[], values: Record<string, unknown>): string {
  const filled = fields.filter((f) => values[f.name] != null && values[f.name] !== '')
  const primary = PRIMARY.map((n) => filled.find((f) => f.name === n)).find(Boolean)
  const body = primary
    ? primary.type === 'richtext'
      ? `<div class="body" data-resonans-path="${esc(`${path}.${primary.name}`)}">${richText(values[primary.name])}</div>`
      : `<div class="body" data-resonans-path="${esc(`${path}.${primary.name}`)}"><b>${esc(short(primary, values[primary.name]))}</b></div>`
    : ''
  const meta = filled
    .filter((f) => f !== primary)
    .map((f) => `<span data-resonans-path="${esc(`${path}.${f.name}`)}">${esc(f.name)}: ${esc(short(f, values[f.name]))}</span>`)
    .join(' · ')
  const icon = ICONS[type] ?? '▪'
  return (
    `<div class="gblk" data-resonans-path="${esc(path)}">` +
    `<span class="tag"><i>${esc(icon)}</i>${esc(label(type))}</span>${body}` +
    (meta ? `<div class="meta">${meta}</div>` : '') +
    `</div>`
  )
}

// renderBody turns an envelope into cards: the document's own fields first,
// then one card per block. Deliberately NOT a page: no page template, no
// "nice" typography — an editor must not make layout decisions on a fake site.
export function renderBody(env: Env, docFields: Field[], blockDefs: BlockType[]): string {
  const data = env.data ?? {}
  const docCardFields: Field[] = [{ name: 'title', type: 'string' }, ...docFields.filter((f) => f.type !== 'blocks')]
  const out = [card('data', 'dokument', docCardFields, data).replace('data-resonans-path="data"', 'data-resonans-path="data.title"')]
  ;(env.components ?? []).forEach((b, i) => {
    const type = String(b._type ?? '')
    const def = blockDefs.find((d) => d.name === type)
    out.push(def ? card(`components.${i}`, type, def.fields, b) : card(`components.${i}`, type || 'ukendt', [], {}))
  })
  return out.join('\n')
}

function nonce(): string {
  const b = new Uint8Array(16)
  crypto.getRandomValues(b)
  return Array.from(b, (x) => x.toString(16).padStart(2, '0')).join('')
}

// buildSrcdoc returns the sandboxed document. The script is the bridge protocol
// v1 in miniature: it posts to the Studio's exact origin and accepts messages
// only from window.parent + that origin. The document's own origin is opaque
// ("null"), which is why the Studio pins the iframe's WINDOW, not an origin.
export type Theme = { bg: string; surface: string; border: string; text: string; muted: string; primary: string }

const COLOR = /^(#[0-9a-fA-F]{3,8}|rgba?\(\s*\d{1,3}\s*,\s*\d{1,3}\s*,\s*\d{1,3}\s*(,\s*(0|1|0?\.\d+)\s*)?\))$/
const FALLBACK_THEME: Theme = { bg: '#ffffff', surface: '#f6f7f9', border: '#e3e6ea', text: '#1c2024', muted: '#6b7280', primary: '#3b5bdb' }

export function buildSrcdoc(studioOrigin: string, theme: Partial<Theme>): string {
  // Theme values land in CSS: only a plain hex or rgb()/rgba() color is accepted.
  const t = { ...FALLBACK_THEME }
  for (const k of Object.keys(t) as (keyof Theme)[]) {
    const v = theme[k]?.trim()
    if (v && COLOR.test(v)) t[k] = v
  }
  const n = nonce()
  const csp = `default-src 'none'; img-src https: data:; style-src 'unsafe-inline'; script-src 'nonce-${n}'`
  const script = `
(function () {
  var O = ${JSON.stringify(studioOrigin)}, V = ${BRIDGE_VERSION};
  var FROM_S = ${JSON.stringify(FROM_STUDIO)}, FROM_B = ${JSON.stringify(FROM_BRIDGE)};
  var RE = /^[A-Za-z0-9_-]+(\\.[A-Za-z0-9_-]+){0,19}$/;
  var main = document.getElementById('fp'), sel = null;
  function post(m) { m.source = FROM_B; m.v = V; parent.postMessage(m, O); }
  function byPath(p) { return p && RE.test(p) ? document.querySelector('[data-resonans-path="' + p + '"]') : null; }
  function mark(p) {
    var old = document.querySelector('.fp-on'); if (old) old.classList.remove('fp-on');
    var chip = document.querySelector('.fp-chip'); if (chip) chip.remove();
    sel = p; var el = byPath(p); if (!el) return;
    el.classList.add('fp-on');
    // "✎ Valgt blok" sits on the CARD that contains the selection (forlæg).
    var cardEl = el.closest('.gblk') || el;
    chip = document.createElement('span'); chip.className = 'fp-chip'; chip.textContent = '✎ Valgt blok';
    cardEl.appendChild(chip);
    el.scrollIntoView({ block: 'center', behavior: 'smooth' });
  }
  // EVERY click is prevented: the document must never navigate away, because
  // the Studio can only address an opaque-origin frame with targetOrigin '*'.
  document.addEventListener('click', function (e) {
    e.preventDefault();
    var el = e.target.closest && e.target.closest('[data-resonans-path]');
    if (!el) return;
    var p = el.getAttribute('data-resonans-path'); if (!RE.test(p)) return;
    mark(p); post({ type: 'select', path: p });
  }, true);
  addEventListener('message', function (e) {
    if (e.source !== parent || e.origin !== O) return;
    var d = e.data; if (!d || d.source !== FROM_S || d.v !== V) return;
    // html is ALREADY sanitized by the Studio; CSP blocks scripts either way.
    if (d.type === 'render' && typeof d.html === 'string') { main.innerHTML = d.html; if (sel) mark(sel); }
    else if (d.type === 'select') mark(typeof d.path === 'string' ? d.path : null);
  });
  post({ type: 'ready', href: 'fallback' });
})();`
  return `<!doctype html><html lang="da"><head><meta charset="utf-8">
<meta http-equiv="Content-Security-Policy" content="${csp}">
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>
body{margin:0;padding:14px;font:12.5px/1.5 system-ui,-apple-system,"Segoe UI",sans-serif;color:${t.text};background:${t.surface}}
*{box-sizing:border-box}
.gblk{position:relative;background:${t.bg};border:1px solid ${t.border};border-radius:7px;padding:10px 12px;margin-bottom:8px}
.gblk .tag{display:inline-flex;align-items:center;gap:5px;font-size:9px;font-weight:600;text-transform:uppercase;letter-spacing:.04em;color:${t.muted};margin-bottom:5px}
.gblk .tag i{color:${t.primary};font-style:normal}
.gblk .body{font-size:12.5px;color:${t.text};line-height:1.5}
.gblk .body b{font-weight:600}.gblk .body p{margin:0 0 4px}
.gblk .meta{font-size:10.5px;color:${t.muted};margin-top:4px}
[data-resonans-path]{cursor:pointer}
[data-resonans-path]:hover{outline:1px solid ${t.primary};outline-offset:1px;border-radius:3px}
.fp-on{outline:2px solid ${t.primary} !important;outline-offset:0;border-radius:7px}
.fp-chip{position:absolute;top:-9px;left:8px;background:${t.primary};color:#fff;font-size:10px;font-weight:500;padding:2px 7px;border-radius:5px;pointer-events:none}
</style></head><body>
<main id="fp"></main>
<script nonce="${n}">${script}</script>
</body></html>`
}
