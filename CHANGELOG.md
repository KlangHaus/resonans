## [2.2.1](https://github.com/KlangHaus/resonans/compare/v2.2.0...v2.2.1) (2026-10-23)


### Bug Fixes

* **deps:** upgrade studio to nuxt 4.6.2 to clear 17 Dependabot alerts ([00c436b](https://github.com/KlangHaus/resonans/commit/00c436b7621657b912b17b64136fe104247366fa))

# [2.2.0](https://github.com/KlangHaus/resonans/compare/v2.1.0...v2.2.0) (2026-10-23)


### Bug Fixes

* **examples:** the klanghaus preview drops DOMPurify; Resonans sends live previews already clean ([15e666e](https://github.com/KlangHaus/resonans/commit/15e666e2d1039ebc6606b25731ad1dafdbb73150))
* **sanitize:** one own HTML policy — the API cleans live previews, the editor drops DOMPurify ([48d17fa](https://github.com/KlangHaus/resonans/commit/48d17fa17ba38eeb072ec323f5abd9a7bc927d91))


### Features

* **examples:** live preview in the editor for the klanghaus.dk example ([bdded61](https://github.com/KlangHaus/resonans/commit/bdded61b113e50deeafe43c709682fa6e68b7913))

# [2.1.0](https://github.com/KlangHaus/resonans/compare/v2.0.0...v2.1.0) (2026-10-22)


### Bug Fixes

* **settings:** qualify tenant_settings with its schema ([208354d](https://github.com/KlangHaus/resonans/commit/208354d73706909e29d8049ff0200480e53b1f1b))


### Features

* **examples:** klanghaus.dk on Resonans, with landing pages and a blog ([db7ad00](https://github.com/KlangHaus/resonans/commit/db7ad00238e63cb8df93c15f2ae58a98a1836841))
* **schema:** landing-page sections — section_header, hero extras, variants ([198bb51](https://github.com/KlangHaus/resonans/commit/198bb51e59c085f2ea43abfcf3932197aa80615b))

# [2.0.0](https://github.com/KlangHaus/resonans/compare/v1.21.8...v2.0.0) (2026-10-22)


* feat(auth)!: log in to the editor with standard OIDC, and enforce roles in the API ([78dcf34](https://github.com/KlangHaus/resonans/commit/78dcf3485dcc2e81951fb679f75c4f6a0fc89929))


### ACTION REQUIRED

* before upgrading, register the editor as a confidential OIDC
client at your provider (redirect URI <editor URL>/auth/callback), make the
provider put an audience and a role into access tokens, and set
RESONANS_JWT_AUDIENCE, RESONANS_ROLE_CLAIM / RESONANS_ROLE_MAP and the editor's
NUXT_OIDC_* values (see .env.example). Upgrading the image alone leaves every
editor locked out with 403.

### BREAKING CHANGES

* the API refuses to start with JWT configured but no
RESONANS_JWT_AUDIENCE, and refuses every request whose token carries no
recognised role. The editor's NUXT_PUBLIC_AUTH_BASE is gone; it now needs
NUXT_OIDC_* settings and a client registered at the provider.

## [1.21.8](https://github.com/KlangHaus/resonans/compare/v1.21.7...v1.21.8) (2026-10-22)


### Bug Fixes

* **deps:** lift npm bundled by the release chain to 11.21.1, closing five advisories ([29a1b64](https://github.com/KlangHaus/resonans/commit/29a1b6466ef42863e0e25fbbff97ea95a2be60ef))
* **go:** move to go 1.26.10 for GO-2026-6809 in crypto/x509 ([07dd52e](https://github.com/KlangHaus/resonans/commit/07dd52e558f137bd3a6e276bd3658b3a7d5b883f))

## [1.21.7](https://github.com/KlangHaus/resonans/compare/v1.21.6...v1.21.7) (2026-10-10)


### Bug Fixes

* **ci:** pin base images as tag@digest so Dependabot lifts the right variant ([#116](https://github.com/KlangHaus/resonans/issues/116)) ([1c7b176](https://github.com/KlangHaus/resonans/commit/1c7b1768cd7680cc20b34b64f2b5e520bb05ca60))

## [1.21.6](https://github.com/KlangHaus/resonans/compare/v1.21.5...v1.21.6) (2026-10-10)


### Bug Fixes

* **studio:** drop the simple-git override that broke the image build on main ([#112](https://github.com/KlangHaus/resonans/issues/112)) ([f26be31](https://github.com/KlangHaus/resonans/commit/f26be318a69389ac9c5b92a82a460cac4449215d))

## [1.21.5](https://github.com/KlangHaus/resonans/compare/v1.21.4...v1.21.5) (2026-10-10)


### Bug Fixes

* **studio:** bump serialize-javascript to 7.1.2, the last reachable advisory ([#109](https://github.com/KlangHaus/resonans/issues/109)) ([8c8f852](https://github.com/KlangHaus/resonans/commit/8c8f852b936e72deb924f3c860bc40109532fc71))

## [1.21.4](https://github.com/KlangHaus/resonans/compare/v1.21.3...v1.21.4) (2026-10-10)


### Bug Fixes

* **studio:** close three critical dev advisories via shell-quote and simple-git 4.0.2 ([#108](https://github.com/KlangHaus/resonans/issues/108)) ([4322410](https://github.com/KlangHaus/resonans/commit/43224105334f8204304b4f942662c4915d19765d))

## [1.21.3](https://github.com/KlangHaus/resonans/compare/v1.21.2...v1.21.3) (2026-10-10)


### Bug Fixes

* **studio:** force source-map-js to 1.2.2, closing a HIGH runtime event-loop DoS ([#107](https://github.com/KlangHaus/resonans/issues/107)) ([d2d21a9](https://github.com/KlangHaus/resonans/commit/d2d21a9493f68b0fdf8ff8647d00d182e3298755))

## [1.21.2](https://github.com/KlangHaus/resonans/compare/v1.21.1...v1.21.2) (2026-10-07)


### Bug Fixes

* **app:** log the NODE_ENV source in every entry point, not just the server ([#103](https://github.com/KlangHaus/resonans/issues/103)) ([33bee53](https://github.com/KlangHaus/resonans/commit/33bee535ddfee4ddb0793a9621b502138d4abb1c))

## [1.21.1](https://github.com/KlangHaus/resonans/compare/v1.21.0...v1.21.1) (2026-10-07)


### Bug Fixes

* **config:** say whether NODE_ENV was declared or defaulted at boot ([#102](https://github.com/KlangHaus/resonans/issues/102)) ([b654f2e](https://github.com/KlangHaus/resonans/commit/b654f2e5663232a8ce0969af9bfa33d2c8b9d4c3))

# [1.21.0](https://github.com/KlangHaus/resonans/compare/v1.20.0...v1.21.0) (2026-10-03)


### Features

* **ci:** arm the two integration cells, and fix the Jenkinsfile that did not compile ([#88](https://github.com/KlangHaus/resonans/issues/88)) ([3405768](https://github.com/KlangHaus/resonans/commit/3405768a2a529b800f59f88f328e2c2a0232b4a6))

# [1.20.0](https://github.com/KlangHaus/resonans/compare/v1.19.0...v1.20.0) (2026-10-03)


### Features

* **build:** fetch the pinned api-reference renderer into the image ([#98](https://github.com/KlangHaus/resonans/issues/98)) ([f813475](https://github.com/KlangHaus/resonans/commit/f8134754a4c119091f7beba1b995b5595dce9fce)), closes [resonans#97](https://github.com/resonans/issues/97) [#97](https://github.com/KlangHaus/resonans/issues/97)

# [1.19.0](https://github.com/KlangHaus/resonans/compare/v1.18.9...v1.19.0) (2026-10-02)


### Features

* **api:** serve the OpenAPI document and a page that renders it ([#97](https://github.com/KlangHaus/resonans/issues/97)) ([328463c](https://github.com/KlangHaus/resonans/commit/328463ca0437b61bb101be15dd0da46c8e16d473)), closes [resonans#93](https://github.com/resonans/issues/93)

## [1.18.9](https://github.com/KlangHaus/resonans/compare/v1.18.8...v1.18.9) (2026-10-02)


### Bug Fixes

* **api:** declare the path parameters the spec templated but never defined ([#89](https://github.com/KlangHaus/resonans/issues/89)) ([e8b1e74](https://github.com/KlangHaus/resonans/commit/e8b1e7423edc0334a000e7771c2f991cebbeee9b))

## [1.18.8](https://github.com/KlangHaus/resonans/compare/v1.18.7...v1.18.8) (2026-10-02)


### Bug Fixes

* **studio:** label a new rich_text block as HTML, because that is what it holds ([#91](https://github.com/KlangHaus/resonans/issues/91)) ([c16256f](https://github.com/KlangHaus/resonans/commit/c16256f0b3ff3373252824a2ee7366dd012de941))

## [1.18.7](https://github.com/KlangHaus/resonans/compare/v1.18.6...v1.18.7) (2026-10-02)


### Bug Fixes

* **backfill:** label and sanitize rich_text stored as HTML without a format ([#93](https://github.com/KlangHaus/resonans/issues/93)) ([3bba597](https://github.com/KlangHaus/resonans/commit/3bba597f035cddfe3e9b53fff3d4560cf37d135b)), closes [#92](https://github.com/KlangHaus/resonans/issues/92)

## [1.18.6](https://github.com/KlangHaus/resonans/compare/v1.18.5...v1.18.6) (2026-10-02)


### Bug Fixes

* **schema:** fail closed on an unrecognized rich_text format, and reject it at the door ([#92](https://github.com/KlangHaus/resonans/issues/92)) ([b95556f](https://github.com/KlangHaus/resonans/commit/b95556f40306e97249fd1d1d96cf261045a01150))

## [1.18.5](https://github.com/KlangHaus/resonans/compare/v1.18.4...v1.18.5) (2026-09-30)


### Bug Fixes

* **release:** drop the version-print step that broke the release chain on its first run ([#87](https://github.com/KlangHaus/resonans/issues/87)) ([151b35f](https://github.com/KlangHaus/resonans/commit/151b35ffb3589e1801b4a0ce1ecf3ce997622806)), closes [#84](https://github.com/KlangHaus/resonans/issues/84)
* **release:** publish operator actions in the release notes, and guard that they stay there ([#84](https://github.com/KlangHaus/resonans/issues/84)) ([7af65b1](https://github.com/KlangHaus/resonans/commit/7af65b182c84529456d9b3c6b7959b65281930a2))

## [1.18.4](https://github.com/KlangHaus/resonans/compare/v1.18.3...v1.18.4) (2026-09-30)


### Bug Fixes

* **deps:** bump the otel log family together, not sdk/log alone ([#85](https://github.com/KlangHaus/resonans/issues/85)) ([807d8f2](https://github.com/KlangHaus/resonans/commit/807d8f2ba5c3df08bf68e8d640d6d0ead8476862)), closes [#83](https://github.com/KlangHaus/resonans/issues/83)

## [1.18.3](https://github.com/KlangHaus/resonans/compare/v1.18.2...v1.18.3) (2026-09-28)


### Bug Fixes

* **deploy:** stop echoing the app password into the container log; add a copy-safe setup ([#82](https://github.com/KlangHaus/resonans/issues/82)) ([f84f67a](https://github.com/KlangHaus/resonans/commit/f84f67a26253e988769bd8cb4b9bbc63a0265f5f))


### ACTION REQUIRED

* **deploy:** if you already run Resonans, recreate your postgres container — `docker compose down -v`, then start again with a NEW app password. Before this release `deploy/initdb/10-app-role.sql` wrote the app-role password in cleartext to that container's log. **Upgrading the image does not remove it; the log already has it.** Added by hand after publication: the instruction was in the commit body, which the notes generator drops — the gap this release's successor fixes.

## [1.18.2](https://github.com/KlangHaus/resonans/compare/v1.18.1...v1.18.2) (2026-09-28)


### Bug Fixes

* **persistence:** migrate without CREATE on the database when the schema exists ([#81](https://github.com/KlangHaus/resonans/issues/81)) ([2221e36](https://github.com/KlangHaus/resonans/commit/2221e3653c2dc2f8ce4cb84d4b14cfccac65c5c2))

## [1.18.1](https://github.com/KlangHaus/resonans/compare/v1.18.0...v1.18.1) (2026-09-28)


### Bug Fixes

* **legal:** say who holds the copyright rather than who we want to ([#80](https://github.com/KlangHaus/resonans/issues/80)) ([64bfc78](https://github.com/KlangHaus/resonans/commit/64bfc7896c695b1d7522715b34ff037cd451f498)), closes [#77](https://github.com/KlangHaus/resonans/issues/77)

# [1.18.0](https://github.com/KlangHaus/resonans/compare/v1.17.0...v1.18.0) (2026-09-24)


### Features

* **docs:** describe the API, and a cell that keeps the description true ([#78](https://github.com/KlangHaus/resonans/issues/78)) ([993d985](https://github.com/KlangHaus/resonans/commit/993d9850899d616000d282da5849b074f6587268))

# [1.17.0](https://github.com/KlangHaus/resonans/compare/v1.16.0...v1.17.0) (2026-09-24)


### Features

* **auth:** production must fetch the JWKS over https ([#76](https://github.com/KlangHaus/resonans/issues/76)) ([01928ce](https://github.com/KlangHaus/resonans/commit/01928cefc71ad7fe317fddc2441ca2fa0dbee5be))

# [1.16.0](https://github.com/KlangHaus/resonans/compare/v1.15.2...v1.16.0) (2026-09-24)


### Features

* **media:** an editor can delete an upload, and undo it ([#73](https://github.com/KlangHaus/resonans/issues/73)) ([63fd68f](https://github.com/KlangHaus/resonans/commit/63fd68f0ce5f92428d9f402323f017f9139e5f49))

## [1.15.2](https://github.com/KlangHaus/resonans/compare/v1.15.1...v1.15.2) (2026-09-24)


### Bug Fixes

* **deploy:** grant the app role in resonans_app, not public ([#72](https://github.com/KlangHaus/resonans/issues/72)) ([c445a83](https://github.com/KlangHaus/resonans/commit/c445a83d7441d7630463fec728fb5fd8b72c18fa)), closes [#69](https://github.com/KlangHaus/resonans/issues/69)

## [1.15.1](https://github.com/KlangHaus/resonans/compare/v1.15.0...v1.15.1) (2026-09-24)


### Bug Fixes

* **studio:** stop the image build when the lockfile disagrees ([#71](https://github.com/KlangHaus/resonans/issues/71)) ([dd68e01](https://github.com/KlangHaus/resonans/commit/dd68e011a341d07a46a2a776cc4e7056be51d6a3)), closes [#70](https://github.com/KlangHaus/resonans/issues/70)

# [1.15.0](https://github.com/KlangHaus/resonans/compare/v1.14.1...v1.15.0) (2026-09-24)


### Features

* make Resonans publishable as open source under AGPL-3.0 ([#69](https://github.com/KlangHaus/resonans/issues/69)) ([a940ebd](https://github.com/KlangHaus/resonans/commit/a940ebd21352c3a23c0e3e1d78634af65665b9df))

## [1.14.1](https://github.com/KlangHaus/resonans/compare/v1.14.0...v1.14.1) (2026-09-24)


### Bug Fixes

* **studio:** drop the one vulnerable brace-expansion resolution ([#70](https://github.com/KlangHaus/resonans/issues/70)) ([39a78da](https://github.com/KlangHaus/resonans/commit/39a78da5f17432c164546756efaa0c3e0eb6b4c2))

# [1.14.0](https://github.com/KlangHaus/resonans/compare/v1.13.0...v1.14.0) (2026-09-23)


### Features

* **media:** a disk backend so Resonans self-hosts without a CDN account ([#68](https://github.com/KlangHaus/resonans/issues/68)) ([68b967b](https://github.com/KlangHaus/resonans/commit/68b967beed0cabb1197846e4975f4048d1829f43))

# [1.13.0](https://github.com/KlangHaus/resonans/compare/v1.12.0...v1.13.0) (2026-09-23)


### Features

* soft delete — an editor can finally remove a document, and undo it ([#67](https://github.com/KlangHaus/resonans/issues/67)) ([2a81176](https://github.com/KlangHaus/resonans/commit/2a81176e52d9f22e3c4bae78f476aae9a4dcdc23))

# [1.12.0](https://github.com/KlangHaus/resonans/compare/v1.11.1...v1.12.0) (2026-09-20)


### Bug Fixes

* **ci:** release gate needs checks:read — 403 killed the release on main ([#64](https://github.com/KlangHaus/resonans/issues/64)) ([2176aa6](https://github.com/KlangHaus/resonans/commit/2176aa68a307d5e5b4027274c943df53f11ab08e)), closes [#62](https://github.com/KlangHaus/resonans/issues/62) [resonans#62](https://github.com/resonans/issues/62)


### Features

* **studio:** log in against BetterAuth and send the JWT the API requires ([#63](https://github.com/KlangHaus/resonans/issues/63)) ([f8abf34](https://github.com/KlangHaus/resonans/commit/f8abf3451d3e234d03e993cfa86b58369bcd592f))

## [1.11.1](https://github.com/KlangHaus/resonans/compare/v1.11.0...v1.11.1) (2026-09-19)


### Bug Fixes

* **studio:** make nuxt typecheck green (9 errors on main, 0 after) ([#61](https://github.com/KlangHaus/resonans/issues/61)) ([77b4878](https://github.com/KlangHaus/resonans/commit/77b4878fc1949d239e3395b8969841879b37a892))

# [1.11.0](https://github.com/KlangHaus/resonans/compare/v1.10.1...v1.11.0) (2026-09-19)


### Features

* **studio:** generisk fallback-preview + fuldskærm i visuel redigering ([#58](https://github.com/KlangHaus/resonans/issues/58)) ([435ed42](https://github.com/KlangHaus/resonans/commit/435ed42b54dcde711bc2af1b0c374c95d66728a0)), closes [#4](https://github.com/KlangHaus/resonans/issues/4) [#54](https://github.com/KlangHaus/resonans/issues/54)

## [1.10.1](https://github.com/KlangHaus/resonans/compare/v1.10.0...v1.10.1) (2026-09-17)


### Bug Fixes

* allow only [a-z0-9] slugs with single hyphens; add read-only slug-audit ([#54](https://github.com/KlangHaus/resonans/issues/54)) ([65ea6d0](https://github.com/KlangHaus/resonans/commit/65ea6d0751d31715dd948ab2e821c9a8777961b9))

# [1.10.0](https://github.com/KlangHaus/resonans/compare/v1.9.2...v1.10.0) (2026-09-17)


### Features

* sanitize-backfill for rich text stored before save-side sanitizing ([#53](https://github.com/KlangHaus/resonans/issues/53)) ([b734329](https://github.com/KlangHaus/resonans/commit/b7343296aa45ae06656f8ed7fcf2d8ab3b1492e4))

## [1.9.2](https://github.com/KlangHaus/resonans/compare/v1.9.1...v1.9.2) (2026-09-17)


### Bug Fixes

* stored XSS in Studio rich text — sanitize at render and at save ([#52](https://github.com/KlangHaus/resonans/issues/52)) ([0c2a034](https://github.com/KlangHaus/resonans/commit/0c2a034de6a9b928379d48520831097956108e7e)), closes [#39](https://github.com/KlangHaus/resonans/issues/39) [#39](https://github.com/KlangHaus/resonans/issues/39)

## [1.9.1](https://github.com/KlangHaus/resonans/compare/v1.9.0...v1.9.1) (2026-09-17)


### Bug Fixes

* dev auth requires RESONANS_DEV_AUTH=1; upload type is sniffed from content ([#51](https://github.com/KlangHaus/resonans/issues/51)) ([af29fd3](https://github.com/KlangHaus/resonans/commit/af29fd32c3d849b625932d4b4f10a11ca463e880))

# [1.9.0](https://github.com/KlangHaus/resonans/compare/v1.8.2...v1.9.0) (2026-09-17)


### Features

* visual editing — live preview in the Studio with two-way selection ([#50](https://github.com/KlangHaus/resonans/issues/50)) ([4558a6d](https://github.com/KlangHaus/resonans/commit/4558a6d671da0208b352154c1ddefdaa0a640af6))

## [1.8.2](https://github.com/KlangHaus/resonans/compare/v1.8.1...v1.8.2) (2026-09-16)


### Bug Fixes

* **ci:** declare and verify the go toolchain Jenkins fetches ([#48](https://github.com/KlangHaus/resonans/issues/48)) ([d5956d3](https://github.com/KlangHaus/resonans/commit/d5956d31ebeb8da1940bbd8c5e84574990d37415))

## [1.8.1](https://github.com/KlangHaus/resonans/compare/v1.8.0...v1.8.1) (2026-09-16)


### Bug Fixes

* **deps:** bump grpc to 1.83.2 and pin govulncheck to v1.7.0 ([#49](https://github.com/KlangHaus/resonans/issues/49)) ([67052dd](https://github.com/KlangHaus/resonans/commit/67052ddc3973db544ffc267126a7b4a05eff1b2d)), closes [#42](https://github.com/KlangHaus/resonans/issues/42)

# [1.8.0](https://github.com/KlangHaus/resonans/compare/v1.7.5...v1.8.0) (2026-09-08)


### Features

* **ci:** thin Jenkinsfile with a test stage ([#47](https://github.com/KlangHaus/resonans/issues/47)) ([64a7106](https://github.com/KlangHaus/resonans/commit/64a710673fd4a504ff70167899a23f48f59162c9)), closes [#34](https://github.com/KlangHaus/resonans/issues/34)

## [1.7.5](https://github.com/KlangHaus/resonans/compare/v1.7.4...v1.7.5) (2026-09-08)


### Bug Fixes

* **ci:** markér en udgivelse der kørte under fallback-identiteten (kur b + pipefail + tag-skelnen) ([#45](https://github.com/KlangHaus/resonans/issues/45)) ([dfd87e4](https://github.com/KlangHaus/resonans/commit/dfd87e4221dcaa28c7ebccc4c51a0fb33031776e)), closes [#38](https://github.com/KlangHaus/resonans/issues/38) [#46](https://github.com/KlangHaus/resonans/issues/46)

## [1.7.4](https://github.com/KlangHaus/resonans/compare/v1.7.3...v1.7.4) (2026-09-08)


### Bug Fixes

* **ci:** stop metadata-action i selv at sætte `latest` (ufuldstændig kur i [#43](https://github.com/KlangHaus/resonans/issues/43)) ([#46](https://github.com/KlangHaus/resonans/issues/46)) ([2869576](https://github.com/KlangHaus/resonans/commit/286957675a95dc820af2beacb10d4452fd88f20f))

## [1.7.3](https://github.com/KlangHaus/resonans/compare/v1.7.2...v1.7.3) (2026-09-07)


### Bug Fixes

* **ci:** derive image tags from the checkout, not the caller's context ([#43](https://github.com/KlangHaus/resonans/issues/43)) ([9aeaceb](https://github.com/KlangHaus/resonans/commit/9aeacebf87b7d847e68c3edb286826cd7771e2d7))

## [1.7.2](https://github.com/KlangHaus/resonans/compare/v1.7.1...v1.7.2) (2026-09-07)


### Bug Fixes

* **ci:** pin gofumpt to v0.11.0 — a floating tag broke CI without a commit ([#44](https://github.com/KlangHaus/resonans/issues/44)) ([1c6d90e](https://github.com/KlangHaus/resonans/commit/1c6d90ebdf4b1d730644f975314dd6de6c41620b))

## [1.7.1](https://github.com/KlangHaus/resonans/compare/v1.7.0...v1.7.1) (2026-08-24)


### Bug Fixes

* **ci:** derive semver tags from the called ref, not the caller's ([#40](https://github.com/KlangHaus/resonans/issues/40)) ([1673809](https://github.com/KlangHaus/resonans/commit/16738097d422f22d8af719e88ac3146d23408a3f)), closes [#39](https://github.com/KlangHaus/resonans/issues/39)

# [1.7.0](https://github.com/KlangHaus/resonans/compare/v1.6.2...v1.7.0) (2026-08-18)


### Features

* **ops:** build-identitet fra /healthz + release-kæden kalder buildet ([#38](https://github.com/KlangHaus/resonans/issues/38)) ([fde83e3](https://github.com/KlangHaus/resonans/commit/fde83e3b664531a1dd498d338df9e208f0a3a557)), closes [#37](https://github.com/KlangHaus/resonans/issues/37)

## [1.6.2](https://github.com/KlangHaus/resonans/compare/v1.6.1...v1.6.2) (2026-08-16)


### Bug Fixes

* **ci:** govulncheck must scan the toolchain we ship ([#36](https://github.com/KlangHaus/resonans/issues/36)) ([d5c0df9](https://github.com/KlangHaus/resonans/commit/d5c0df95f88b25b93a1be64cd83a95397c5e3e83)), closes [#35](https://github.com/KlangHaus/resonans/issues/35)

## [1.6.1](https://github.com/KlangHaus/resonans/compare/v1.6.0...v1.6.1) (2026-08-14)


### Bug Fixes

* **ci:** align Go toolchain — test and ship the same stdlib ([#34](https://github.com/KlangHaus/resonans/issues/34)) ([988ebd4](https://github.com/KlangHaus/resonans/commit/988ebd4dd5dd9452e92c4cb3ab06015eea3e9674)), closes [#2](https://github.com/KlangHaus/resonans/issues/2) [#33](https://github.com/KlangHaus/resonans/issues/33)

# [1.6.0](https://github.com/KlangHaus/resonans/compare/v1.5.0...v1.6.0) (2026-07-29)


### Bug Fixes

* space creation on insecure origins (crypto.randomUUID fallback) ([#26](https://github.com/KlangHaus/resonans/issues/26)) ([93d16bb](https://github.com/KlangHaus/resonans/commit/93d16bbb743c7d8e951e299566dcc258a7a0a452)), closes [#25](https://github.com/KlangHaus/resonans/issues/25)


### Features

* unified Content nav — types chosen at creation, not nav items ([#28](https://github.com/KlangHaus/resonans/issues/28)) ([33440a1](https://github.com/KlangHaus/resonans/commit/33440a1f6d819a3bb9047f59643392059bdc12a7)), closes [#27](https://github.com/KlangHaus/resonans/issues/27)

# [1.5.0](https://github.com/KlangHaus/resonans/compare/v1.4.0...v1.5.0) (2026-07-29)


### Features

* rail-polish mod designer-forlaeg — ikon-rail, rail-tokens, profil-badges ([#24](https://github.com/KlangHaus/resonans/issues/24)) ([cac566a](https://github.com/KlangHaus/resonans/commit/cac566a7b177cc8767d446fe5ca6ce43c1ad27dc)), closes [#23](https://github.com/KlangHaus/resonans/issues/23)

# [1.4.0](https://github.com/KlangHaus/resonans/compare/v1.3.0...v1.4.0) (2026-07-29)


### Features

* Storyblok-style Studio-skal — Spaces-forside + org m/ space-oprettelse ([#22](https://github.com/KlangHaus/resonans/issues/22)) ([93ea76f](https://github.com/KlangHaus/resonans/commit/93ea76f3bd896b927f0f5c9ed822eac410080005)), closes [#21](https://github.com/KlangHaus/resonans/issues/21)

# [1.3.0](https://github.com/KlangHaus/resonans/compare/v1.2.0...v1.3.0) (2026-07-28)


### Features

* per-tenant content profile — enable/disable doc-types in Settings ([#20](https://github.com/KlangHaus/resonans/issues/20)) ([5862316](https://github.com/KlangHaus/resonans/commit/586231650a8835a339730fc859cd46d372b5fd9c)), closes [#19](https://github.com/KlangHaus/resonans/issues/19)

# [1.2.0](https://github.com/KlangHaus/resonans/compare/v1.1.0...v1.2.0) (2026-07-28)


### Features

* ship Studio as its own image (resonans-studio) for staging deploy ([#18](https://github.com/KlangHaus/resonans/issues/18)) ([6a84295](https://github.com/KlangHaus/resonans/commit/6a84295faffbc233eb1bf255f2f40bdec57afc8e)), closes [#17](https://github.com/KlangHaus/resonans/issues/17)
* visuel blok-editor i Studio (Fase 4 core) ([#16](https://github.com/KlangHaus/resonans/issues/16)) ([d986c62](https://github.com/KlangHaus/resonans/commit/d986c62eae2a03f080ba2624eef6b307df68287e)), closes [#96](https://github.com/KlangHaus/resonans/issues/96) [#15](https://github.com/KlangHaus/resonans/issues/15)

# [1.1.0](https://github.com/KlangHaus/resonans/compare/v1.0.1...v1.1.0) (2026-07-28)


### Bug Fixes

* JWKS boot-priming is non-fatal (degrade-not-fail) ([#14](https://github.com/KlangHaus/resonans/issues/14)) ([7462b65](https://github.com/KlangHaus/resonans/commit/7462b6503bf99c8884df5bc3fd994d2530eb1116)), closes [#13](https://github.com/KlangHaus/resonans/issues/13)


### Features

* blog delivery-summary fields + idempotent seed + Journal posts ([#12](https://github.com/KlangHaus/resonans/issues/12)) ([c5152c5](https://github.com/KlangHaus/resonans/commit/c5152c5654a112df8f54ebf9ccf43d745c7eacc0)), closes [#11](https://github.com/KlangHaus/resonans/issues/11)

## [1.0.1](https://github.com/KlangHaus/resonans/compare/v1.0.0...v1.0.1) (2026-07-27)


### Bug Fixes

* sort delivery catalog by publishedAt desc ([#10](https://github.com/KlangHaus/resonans/issues/10)) ([b61410c](https://github.com/KlangHaus/resonans/commit/b61410c5409f121f6add81381cc8be06ecfd2bca)), closes [#9](https://github.com/KlangHaus/resonans/issues/9)

# 1.0.0 (2026-07-27)


### Bug Fixes

* **deps:** bump grpc 1.82.1 + x/text 0.39.0 (govulncheck GO-2026-6061 + 5970) ([87e5e1f](https://github.com/KlangHaus/resonans/commit/87e5e1fbd3dc38a6ebe8f059e02a7258f9b16468))


### Features

* `resonans migrate` one-shot subcommand (deploy-wiring) ([8e4b8b1](https://github.com/KlangHaus/resonans/commit/8e4b8b186ca0b379283ffa0432a0b96025b354bd)), closes [#1](https://github.com/KlangHaus/resonans/issues/1)
* **delivery:** §F block-shaping + quiz doc-type + /schema + ETag (Commit B) ([ad562ad](https://github.com/KlangHaus/resonans/commit/ad562adee66e0b227c05c24ed7426c61c493a7ab)), closes [#1](https://github.com/KlangHaus/resonans/issues/1)
* **delivery:** course-tree expand (?expand=modules.lessons) — kontrakt komplet ([3af95e3](https://github.com/KlangHaus/resonans/commit/3af95e312f7275a5296f8b8b106a9314f9b2f892)), closes [#1](https://github.com/KlangHaus/resonans/issues/1)
* **delivery:** envelope + locale + asset-resolution (Fase 2.1, etude-kontrakt) ([43de582](https://github.com/KlangHaus/resonans/commit/43de58249bf77d9784546506e2f4dfbe7255d3a2)), closes [#1](https://github.com/KlangHaus/resonans/issues/1)
* media pipeline — assets + Bunny storage + Vimeo plugin (Fase 3) ([1a5d836](https://github.com/KlangHaus/resonans/commit/1a5d836c2bc04726c5712e0284886a01f77d12da)), closes [#1](https://github.com/KlangHaus/resonans/issues/1)
* multi-tenant foundation — RLS + JWT auth (Fase 0) ([d857d1c](https://github.com/KlangHaus/resonans/commit/d857d1c9bf2f33ee6988277f20e60165260689d5)), closes [#1](https://github.com/KlangHaus/resonans/issues/1)
* scaffold Resonans Go headless CMS ([c64e6aa](https://github.com/KlangHaus/resonans/commit/c64e6aae2119cfb0319b8a3c571216d00597f037)), closes [#1](https://github.com/KlangHaus/resonans/issues/1)
* schema-registry + body-validering + schema-API (Fase 1) ([b0bd287](https://github.com/KlangHaus/resonans/commit/b0bd287c5fdfb471f0984963541327655b14b17a)), closes [#1](https://github.com/KlangHaus/resonans/issues/1)
* **seed:** explainers demo dataset + `resonans seed` subcommand ([b19def0](https://github.com/KlangHaus/resonans/commit/b19def02ea03da4e46576e23819154ee7b60dd9d)), closes [#1](https://github.com/KlangHaus/resonans/issues/1)
* **studio:** schema-driven editor-shell (Fase 4) ([b8eda31](https://github.com/KlangHaus/resonans/commit/b8eda31b89fecd7b7923d81bbf287468795833aa))
* tenant-scoped read/delivery-API (Fase 2 — Etude M5) ([bbcafb5](https://github.com/KlangHaus/resonans/commit/bbcafb51e250aa6ccbce5bc08fef3c80a2415d81)), closes [#1](https://github.com/KlangHaus/resonans/issues/1)
* version-history + rollback + preview (Fase 5 workflow+preview) ([71b6723](https://github.com/KlangHaus/resonans/commit/71b67231ccc82156a2921d3bf3c371b6fe6a6b07)), closes [#1](https://github.com/KlangHaus/resonans/issues/1)
* Vimeo oEmbed metadata — video-heavy authoring (Fase 3 deepening) ([05f1c10](https://github.com/KlangHaus/resonans/commit/05f1c10136b95473f9ce1f66a79ed4c8ee4db228)), closes [#1](https://github.com/KlangHaus/resonans/issues/1)
