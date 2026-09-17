// sanitizeRichText is the ONLY way draft HTML becomes markup in the Studio —
// in the rich-text editor and in the fallback preview alike.
//
// A document body is written by whoever can edit the tenant (and via the dev
// header on staging), then rendered in OTHER editors' sessions. Setting it with
// innerHTML unsanitized ran `<img onerror>` / `<svg><animate onbegin>` in the
// Studio's own origin (measured). DOMPurify is maintained; a hand-written regex
// is not a sanitizer.
import DOMPurify from 'dompurify'

// What the editor's command set produces (b/i/h2/h3/lists/link/quote) plus the
// line containers contenteditable inserts (div, br).
const RICH_TAGS = ['p', 'div', 'br', 'strong', 'b', 'em', 'i', 'u', 's', 'h2', 'h3', 'h4', 'ul', 'ol', 'li', 'blockquote', 'a', 'code', 'pre']

export function sanitizeRichText(html: string): string {
  return String(
    DOMPurify.sanitize(html, {
      ALLOWED_TAGS: RICH_TAGS,
      ALLOWED_ATTR: ['href'],
      ALLOW_DATA_ATTR: false,
      // Only absolute http(s) and mailto links survive (no javascript:, data:).
      ALLOWED_URI_REGEXP: /^(?:https?:|mailto:)/i,
    }),
  )
}
