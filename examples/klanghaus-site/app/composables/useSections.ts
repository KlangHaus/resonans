// A Resonans page is a flat list of blocks. A landing page reads it as sections:
// everything before the first `section_header` is the opening (hero + facts),
// and each `section_header` starts a section that runs until the next one.
import type { InjectionKey } from 'vue'
import type { Component } from '~~/server/utils/resonans'

/** A block plus `_i`, its index in the envelope's `components`. The index is
 *  what an editable path is built from ("components.3.heading"), and sectioning
 *  would otherwise lose it. */
export type Block = Component & { _i?: number }

export interface Section {
  header?: Block
  blocks: Block[]
}

export function toSections(components: Block[] = []): Section[] {
  const sections: Section[] = [{ blocks: [] }]
  components.forEach((raw, i) => {
    const c: Block = { ...raw, _i: i }
    if (c._type === 'section_header') sections.push({ header: c, blocks: [] })
    else sections[sections.length - 1]!.blocks.push(c)
  })
  return sections.filter((s) => s.header || s.blocks.length)
}

export const str = (v: unknown): string => (typeof v === 'string' ? v : '')

/** An asset field resolves to an object with a url (Resonans expands asset ids). */
export function assetUrl(v: unknown): string {
  if (v && typeof v === 'object' && 'url' in v) return str((v as { url: unknown }).url)
  return ''
}

/** Headings may carry a line break and one emphasis (*like this*), which is all
 *  klanghaus.dk's hero uses. The line break is `|`, not a newline: the editor
 *  edits a heading in a single-line input, which shows a newline as nothing and
 *  drops it on the first save (measured in the editor). A newline still works for
 *  content written before. Escaped first, so no other HTML gets in. */
export function headingHtml(s: string): string {
  const esc = s.replace(/[&<>"']/g, (ch) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[ch]!)
  return esc.replace(/\*([^*]+)\*/g, '<em>$1</em>').replace(/\s*(\||\n)\s*/g, '<br />')
}

export interface Feature {
  title?: string
  body?: string
  icon?: string
}

export const features = (b: Block): Feature[] => (Array.isArray(b.features) ? (b.features as Feature[]) : [])

/** Columns whose cells are all images: a photo row. */
export function columnImages(b: Block): Array<{ url: string; alt: string }> {
  const cols = Array.isArray(b.columns) ? (b.columns as Array<{ content?: Block[] }>) : []
  return cols.flatMap((c) =>
    (c.content ?? [])
      .filter((x) => x._type === 'image')
      .map((x) => ({ url: assetUrl(x.asset), alt: str((x.asset as { alt?: unknown } | undefined)?.alt) })),
  )
}

export const isExternal = (href: string) => /^https?:\/\//.test(href)

/** Resonans' sanitizer keeps rich text to formatting markup and drops `id`
 *  attributes, so a link like /privatlivspolitik#venteliste would have nothing
 *  to land on. Every <h2>/<h3> gets an id from its own text instead
 *  ("Venteliste" → "venteliste"), which keeps the anchors klanghaus.dk links to. */
export function withHeadingIds(html: string): string {
  return html.replace(/<(h[23])>([\s\S]*?)<\/\1>/g, (_, tag: string, inner: string) => {
    const id = inner
      .replace(/<[^>]+>/g, '')
      .toLowerCase()
      .replace(/æ/g, 'ae').replace(/ø/g, 'oe').replace(/å/g, 'aa')
      .normalize('NFKD').replace(/[^\w\s-]/g, '')
      .trim().replace(/\s+/g, '-')
    return `<${tag} id="${id}">${inner}</${tag}>`
  })
}

// ── Visual editing ──────────────────────────────────────────────────────────
// On the preview route only, rendered values carry `data-resonans-path`, the
// path of the value in the delivery envelope, so a click in the editor's preview
// selects its field. Everywhere else the helper returns nothing: the public site
// gets no attributes and no bridge.
export const PREVIEW_KEY: InjectionKey<boolean> = Symbol('resonans-preview')

export function useEditable() {
  const on = inject(PREVIEW_KEY, false)
  return (path: string | number | undefined, ...rest: Array<string | number>) =>
    on && path !== undefined ? { 'data-resonans-path': [`components.${path}`, ...rest].join('.') } : {}
}
