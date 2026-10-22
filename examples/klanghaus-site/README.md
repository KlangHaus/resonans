# klanghaus.dk on Resonans

[klanghaus.dk](https://klanghaus.dk), rebuilt so that every word and photo on it comes from
Resonans. The front-end decides only how things look — it uses the same
[grundtone](https://github.com/KlangHaus/grundtone) components and the same stylesheets as the
live site, so the front page renders the same text at the same height (measured against the live
site at 1280 px: identical `main` text, page height 4950 px on both).

It is also a test bed: a real landing page, a text page, a landing page made only of placeholder
content, and a blog, all edited in Resonans.

| Route | Resonans document | What it shows |
| --- | --- | --- |
| `/` | `page` `forside` | klanghaus.dk's front page, text unchanged |
| `/privatlivspolitik` | `page` `privatlivspolitik` | the privacy policy, text unchanged, marked as a **test copy** |
| `/bunker-udlejning` | `page` `bunker-udlejning` | a second landing page — **placeholder content**, not an offer |
| `/blog`, `/blog/:slug` | every published `post` | the blog posts from Resonans' own demo seed |

## Run it

With Tilt, from the repository root:

```bash
tilt up
```

Then press **resonans-seed** (the blog posts) and **klanghaus-site-seed** (the pages and photos) in
the Tilt UI, and open <http://localhost:3050>.

Without Tilt, against any Resonans running with `RESONANS_DEV_AUTH=1`:

```bash
pnpm install
RESONANS_URL=http://localhost:3000 pnpm seed
NUXT_RESONANS_URL=http://localhost:3000 pnpm dev --port 3050
```

The seed is safe to run again: a page that exists is updated and republished, not duplicated.

## How a page becomes a landing page

A Resonans `page` is a flat list of blocks. This site reads it as sections:

- everything before the first `section_header` is the opening — a `hero`, and a `feature_grid`
  with variant `facts` under it;
- each `section_header` starts a section (`eyebrow`, `heading`, `lead`, `anchor` for `#links`,
  `navLabel` for the menu) that runs until the next one.

The menu is built from the front page's section headers, so adding a section in Resonans adds it
to the menu.

Layout is chosen by the `variant` a block carries. Resonans stores it and never interprets it:

| Block | `variant` | Renders as |
| --- | --- | --- |
| `feature_grid` | `facts` | the number row under the hero |
| `feature_grid` | `cards` | icon cards |
| `feature_grid` | `steps` | numbered steps |
| `feature_grid` | `checklist` | the ✓ list beside a `quote` |
| `feature_grid` | `plan` | the studio overview and floor plan |
| `section_header` | `band` | a section on the tinted band |
| `section_header` | `waitlist` | button and note in one row |
| `section_header` | `contact` | the contact block, with rich text as the card |
| `columns` of `image` | — | a photo row |

### The studio overview

A `feature_grid` with variant `plan`, one feature per studio: `title` "Studie 3", `body` "15 m²",
and `icon` the date it is free from (`2026-11-01`), or empty when it is not. On klanghaus.dk this
lives in code and changes with a deploy; here an editor changes it and the next request shows it.
"Fra 1. november" turns into "nu" on the day, because the page is rendered per request. Which
studio sits where in the building is layout, not content, and stays in `app/site.ts`.

## Not content, on purpose

- The header and footer frame (`app/site.ts`): name, logo, e-mail, the company line.
- `assets/styles/brand.scss` and `landing.scss` are klanghaus.dk's own, copied unchanged;
  `site.scss` adds only what its one-pager never needed (rich text, callouts, code, the blog).

## Known differences from klanghaus.dk

- Photos are served by Resonans as one JPEG each; the live site serves AVIF/WebP/JPEG at two
  sizes.
- The live site is static and has no scripts; this one is a Nuxt app with server-side rendering.
  That is why the privacy policy is marked as a test copy: its own premises describe klanghaus.dk.
- The live site's JSON-LD and `llms.txt` are not reproduced.
