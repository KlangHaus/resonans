// A component's `var(--rz-x, #fff)` with no --rz-x in the theme always takes the
// fallback, in dark mode too: that gave white cards with light text.
import { readFileSync, readdirSync, statSync } from 'node:fs'
import { join } from 'node:path'
import { expect, it } from 'vitest'

const APP = join(__dirname, '..')

function sources(dir: string): string[] {
  return readdirSync(dir).flatMap((name) => {
    const path = join(dir, name)
    if (statSync(path).isDirectory()) return sources(path)
    return /\.(vue|css)$/.test(name) ? [path] : []
  })
}

it('defines every theme token the editor uses', () => {
  const theme = readFileSync(join(APP, 'assets', 'theme.css'), 'utf8')
  const defined = new Set([...theme.matchAll(/(--rz-[\w-]+)\s*:/g)].map((m) => m[1]))
  const used = new Set(
    sources(APP).flatMap((path) =>
      [...readFileSync(path, 'utf8').matchAll(/var\(\s*(--rz-[\w-]+)/g)].map((m) => m[1]),
    ),
  )
  expect(used.size).toBeGreaterThan(10)
  expect([...used].filter((token) => !defined.has(token))).toEqual([])
})
