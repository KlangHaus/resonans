// The editor's own sanitizer against the SAME corpus the server's policy runs
// (internal/schema/sanitize_corpus_test.go). If either side drifts, one of the
// two suites goes red.
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'
import { sanitizeRichText } from './sanitize'

interface CorpusCase {
  name: string
  input: string
  mustContain: string[]
  mustNotContain: string[]
}

// Vitest runs from studio/; under jsdom import.meta.url is not a file URL, so
// the path is resolved from the working directory instead.
const corpusPath = resolve(process.cwd(), '../internal/schema/testdata/sanitize-corpus.json')
const corpus = JSON.parse(readFileSync(corpusPath, 'utf8')) as { cases: CorpusCase[] }

describe('sanitizeRichText (own, not DOMPurify)', () => {
  it('loads a real corpus', () => {
    // A corpus that loaded nothing would pass every case below.
    expect(corpus.cases.length).toBeGreaterThanOrEqual(10)
  })

  for (const c of corpus.cases) {
    it(c.name, () => {
      const out = sanitizeRichText(c.input)
      for (const want of c.mustContain) expect(out, `lost ${want}`).toContain(want)
      for (const bad of c.mustNotContain) expect(out.toLowerCase(), `kept ${bad}`).not.toContain(bad.toLowerCase())
      // Idempotent: what it returns is already clean, so a second pass changes nothing.
      expect(sanitizeRichText(out)).toBe(out)
    })
  }
})
