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
