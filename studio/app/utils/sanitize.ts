// sanitizeRichText is the ONLY way draft HTML becomes markup in the Studio —
// in the rich-text editor and in the fallback preview alike.
//
// A document body is written by whoever can edit the tenant, then rendered in
// OTHER editors' sessions. Setting it with innerHTML unsanitized ran
// `<img onerror>` / `<svg><animate onbegin>` in the Studio's own origin
// (measured). So every string goes through this allowlist first.
//
// 🔴 OUR OWN, NOT DOMPURIFY (Allan 2026-10-23). It is deliberately small:
//   · Parsing is the BROWSER's — an inert <template>, where scripts do not run
//     and images are not fetched. Nothing here parses HTML with a regex.
//   · Output is BUILT, never copied: for each allowed element a fresh element is
//     created and only `href` (on <a>, http(s)/mailto) is set on it. Attributes,
//     comments and unknown elements of the input never reach the output.
//   · The allowlist is the server's (internal/schema/sanitize.go). Both run the
//     same corpus (internal/schema/testdata/sanitize-corpus.json), so the two
//     cannot drift apart unnoticed.

// What the editor's command set produces (b/i/h2/h3/lists/link/quote) plus the
// line containers contenteditable inserts (div, br). Same list as the server.
const RICH_TAGS = new Set(['p', 'div', 'br', 'strong', 'b', 'em', 'i', 'u', 's', 'h2', 'h3', 'h4', 'ul', 'ol', 'li', 'blockquote', 'a', 'code', 'pre'])

// Elements whose CONTENT goes too, not just the tag: their text is code, a
// foreign namespace or an embedded document, never prose to keep.
const DROP_WITH_CONTENT = new Set(['script', 'style', 'template', 'iframe', 'object', 'embed', 'noscript', 'noembed', 'noframes', 'svg', 'math', 'textarea', 'title', 'xmp', 'select', 'option', 'frameset', 'frame'])

// Only absolute http(s) and mailto links survive (no javascript:, data:, or
// relative URLs). Control characters and whitespace a browser would strip
// before reading the scheme are refused outright, rather than reasoned about.
const SAFE_HREF = /^(?:https?:\/\/|mailto:)[^\s\u0000-\u001f\u007f]*$/i

function safeHref(raw: string | null): string | null {
  if (raw === null) return null
  return SAFE_HREF.test(raw) ? raw : null
}

function copyAllowed(from: Node, to: Node, doc: Document): void {
  for (const child of Array.from(from.childNodes)) {
    if (child.nodeType === Node.TEXT_NODE) {
      to.appendChild(doc.createTextNode(child.textContent ?? ''))
      continue
    }
    if (child.nodeType !== Node.ELEMENT_NODE) continue // comments, PIs, …
    const el = child as Element
    const tag = el.localName.toLowerCase()
    // Only the HTML namespace: an <a> or <p> inside foreign content (svg/math)
    // never gets here, because svg/math are dropped with their content.
    if (DROP_WITH_CONTENT.has(tag)) continue
    if (RICH_TAGS.has(tag) && el.namespaceURI === 'http://www.w3.org/1999/xhtml') {
      const clean = doc.createElement(tag)
      if (tag === 'a') {
        const href = safeHref(el.getAttribute('href'))
        if (href) clean.setAttribute('href', href)
      }
      copyAllowed(el, clean, doc)
      to.appendChild(clean)
    } else {
      // An unknown but harmless element (span, font, …): keep its text and its
      // allowed descendants, drop the element itself.
      copyAllowed(el, to, doc)
    }
  }
}

export function sanitizeRichText(html: string): string {
  // <template> content is inert: parsed by the browser, but not rendered, no
  // scripts run, no resources load.
  const tpl = document.createElement('template')
  tpl.innerHTML = html
  const out = document.createElement('div')
  copyAllowed(tpl.content, out, document)
  return out.innerHTML
}
