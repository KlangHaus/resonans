# Visual editing

Storyblok-style editing: the Studio shows the tenant's own frontend in an
iframe next to the form, updates it live from the **unsaved** document, and a
click on an element in the preview jumps to its field.

This directory is a dependency-free reference site (`server.mjs` + `index.html`).
It is both the local test bed and the integration guide for real frontends
(Etude, blog).

## How it fits together

```
Studio (editor)                          Frontend preview route (iframe)
───────────────                          ───────────────────────────────
POST /api/v1/preview-token  ─ token ─►   GET /delivery/v1/preview/{type}/{slug}?token=…
                                         renders the draft server-side (first paint)
                             ◄─ ready ─  bridge loaded, ResonansBridge.init(...)
POST /api/v1/preview/envelope
(unsaved body → envelope)   ─ update ─►  onUpdate(envelope) → re-render
click/focus a block or field ─ select ─► "✎ Valgt blok" frame on that element
select block/field in editor ◄─ select ─ click on [data-resonans-path]
```

All messages are `{ source, v: 1, type, ... }`. The Studio accepts messages only
from the iframe's window **and** the origin derived from the tenant's preview URL,
and posts only to that origin. The bridge accepts messages only from
`window.parent` **and** its configured `studioOrigin`, and posts only to it.

## Integrating a frontend

1. **Preview route.** Add a route matching the tenant's preview URL template,
   for example `/preview/{type}/{slug}?token={token}&locale={locale}`. Render the
   draft server-side from `GET /delivery/v1/preview/{type}/{slug}?token=…&locale=…`.
   The token is scoped to one tenant, type, and slug, and expires after 15 minutes.
   Send `Cache-Control: no-store`.
2. **Frame protection.** On the preview route only, send
   `Content-Security-Policy: frame-ancestors <studio origin>`, so no other site
   can frame the draft.
3. **Bridge.** On the preview route only, load
   `<resonans>/bridge/v1/resonans-bridge.js` and call:

   ```js
   ResonansBridge.init({
     studioOrigin: 'https://studio.example', // site configuration, NOT from the URL
     onUpdate: (envelope) => render(envelope),
     accent: '#0059b3', // optional: color of the hover and selection frames
   })
   ```

   `studioOrigin` must come from the site's own configuration. Reading it from
   the query string would let any page that frames the preview drive it.
4. **Mark rendered values** with the path of the value in the delivery envelope:
   `data-resonans-path="data.seoTitle"`, `data-resonans-path="components.2.heading"`.
   In Vue: `v-bind="ResonansBridge.attr('components.' + i + '.heading')"`.
   Only mark elements on the preview route.
5. **Render from the envelope.** `onUpdate` receives the same shape as the
   delivery API, so the production renderer is reused as is. Render draft strings
   as text, never through `innerHTML` without the site's normal sanitizing.

Then enter the preview URL under **Studio → Indstillinger → Visuel redigering**.
It must be `https` (plain `http` only for `localhost`), have a fixed host, and
contain `{token}`.

## Run locally

Start the Resonans API with a preview secret, the Studio, and this site:

```bash
GOTOOLCHAIN=go1.25.13 PORT=3998 RESONANS_PREVIEW_SECRET=local-secret go run ./cmd/resonans
```

```bash
cd studio && RESONANS_DEV_API=http://localhost:3998 NUXT_PUBLIC_DEV_TENANT=11111111-1111-1111-1111-111111111111 npx nuxt dev --port 3030
```

```bash
cd examples/visual-editing && RESONANS_URL=http://localhost:3998 STUDIO_ORIGIN=http://localhost:3030 node server.mjs
```

Set the preview URL to
`http://localhost:4100/preview/{type}/{slug}?token={token}&locale={locale}`,
create a `page` document, and open it with **Visuel**.

The bridge is served with `Cache-Control: max-age=300`. After changing the
bridge, a browser can keep running the old script for up to five minutes; the
example appends `?started=` to avoid that during development.
