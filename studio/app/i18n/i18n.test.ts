// Keeps the interface language honest: every string in the source has Danish, no
// translation is left over from a removed string, and no Danish stays in the source.
import { readFileSync, readdirSync, statSync } from 'node:fs'
import { join, relative } from 'node:path'
import { describe, expect, it, vi } from 'vitest'
import { da } from './da'

vi.stubGlobal('useRuntimeConfig', () => ({ public: {} }))
const { translate } = await import('../composables/useT')

const STUDIO = join(__dirname, '..', '..')
const ROOTS = ['app', 'server']
const SOURCE = /\.(vue|ts|css)$/
const IS_THIS_FEATURE = (path: string) => path.startsWith('app/i18n/')

function sources(dir: string): string[] {
  return readdirSync(dir).flatMap((name) => {
    const path = join(dir, name)
    if (statSync(path).isDirectory()) return sources(path)
    return SOURCE.test(name) ? [path] : []
  })
}

const files = ROOTS.flatMap((root) => sources(join(STUDIO, root)))
  .map((path) => ({ path: relative(STUDIO, path), text: readFileSync(path, 'utf8') }))
  .filter((f) => !IS_THIS_FEATURE(f.path))

// A t('...') or t("...") call with a literal key. Handles escaped quotes.
const CALL = /\bt\(\s*(['"])((?:\\.|(?!\1).)*)\1/g
const keys = new Set(
  files.flatMap((f) => [...f.text.matchAll(CALL)].map((m) => m[2]!.replace(/\\(['"])/g, '$1'))),
)

describe('the source', () => {
  it('uses t() at all, so the checks below are not passing on nothing', () => {
    expect(files.length).toBeGreaterThan(20)
    expect(keys.size).toBeGreaterThan(50)
  })

  it('has no Danish letters outside the dictionary', () => {
    const offenders = files.filter((f) => /[æøåÆØÅ]/.test(f.text)).map((f) => f.path)
    expect(offenders).toEqual([])
  })

  it('has Danish for every string', () => {
    expect([...keys].filter((k) => !(k in da))).toEqual([])
  })

  it('has no Danish for a string that is gone', () => {
    expect(Object.keys(da).filter((k) => !keys.has(k))).toEqual([])
  })

  it('keeps the same placeholders in both languages', () => {
    const names = (s: string) => [...s.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort()
    const differ = Object.entries(da).filter(([k, v]) => names(k).join() !== names(v).join())
    expect(differ).toEqual([])
  })
})

describe('translate', () => {
  it('returns English by default', () => {
    expect(translate('en', 'Settings')).toBe('Settings')
  })

  it('falls back to the English text when a key has no translation', () => {
    expect(translate('da', 'not a key anywhere')).toBe('not a key anywhere')
    expect(translate('xx', 'Settings')).toBe('Settings')
  })

  it('fills placeholders and leaves an unknown one visible', () => {
    expect(translate('en', 'Delete {name} from {place}?', { name: 'a' })).toBe('Delete a from {place}?')
  })

  it('fills a zero', () => {
    expect(translate('en', '{n} items', { n: 0 })).toBe('0 items')
  })
})
