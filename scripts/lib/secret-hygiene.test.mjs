// Ported from KlangHaus/grundtone@36f9227c (vitest there); node:test here, so the workflow runs
// it without an install. The cells are the same.
import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

import { fetchEnvironments, Instrument, runCell, selfTestCells } from './secret-hygiene.mjs'

/**
 * In-process cells for the DECISION. These are the guard's own self-test cells — CI also runs them
 * via `node scripts/secret-hygiene.mjs --self-test`, so there is one list of cells and two
 * runners, not two suites that can drift apart.
 */
describe('secret-hygiene self-test cells', () => {
  const cells = selfTestCells()

  // A list that silently shrank to a handful would still pass every cell below.
  it('carries the full cell list', () => {
    assert.ok(cells.length >= 38, `only ${cells.length} cells`)
  })

  for (const c of cells) {
    it(c.label, () => {
      assert.equal(runCell(c), c.expect)
    })
  }
})

describe('runCell', () => {
  it('reads an instrument error as INSTRUMENT, not as a verdict', () => {
    const fn = () => {
      throw new Instrument('cannot measure')
    }
    assert.equal(runCell({ fn }), 'INSTRUMENT')
  })

  it('reports any other exception as THREW, so a crash cannot pass as a colour', () => {
    const fn = () => {
      throw new TypeError('boom')
    }
    assert.equal(runCell({ fn }), 'THREW TypeError: boom')
  })
})

/**
 * A stand-in for `fetch` that serves the given pages in order and records the URLs it was asked
 * for. `pages` holds JSON bodies, or a number for an HTTP error status.
 */
function stubFetch(pages) {
  const urls = []
  const fetchImpl = async (url) => {
    urls.push(url)
    const page = pages[urls.length - 1]
    if (typeof page === 'number') return { ok: false, status: page }
    return { ok: true, status: 200, json: async () => page }
  }
  return { fetchImpl, urls }
}

const env = (name) => ({ name, deployment_branch_policy: { custom: true } })
const opts = (fetchImpl) => ({ token: 't', repo: 'o/r', apiUrl: 'https://api.example', fetchImpl })

describe('fetchEnvironments', () => {
  it('reads every page until total_count is reached', async () => {
    const { fetchImpl, urls } = stubFetch([
      { total_count: 3, environments: [env('a'), env('b')] },
      { total_count: 3, environments: [env('c')] },
    ])
    const envs = await fetchEnvironments(opts(fetchImpl))
    assert.deepEqual(
      envs.map((e) => e.name),
      ['a', 'b', 'c'],
    )
    assert.deepEqual(urls, [
      'https://api.example/repos/o/r/environments?per_page=100&page=1',
      'https://api.example/repos/o/r/environments?per_page=100&page=2',
    ])
  })

  // One more environment than GitHub's default page of 30.
  it('refuses 30 environments when total_count says 31', async () => {
    const thirty = Array.from({ length: 30 }, (_, i) => env(`e${i}`))
    const { fetchImpl } = stubFetch([
      { total_count: 31, environments: thirty },
      { total_count: 31, environments: [] },
    ])
    await assert.rejects(fetchEnvironments(opts(fetchImpl)), /read 30 but total_count says 31/)
  })

  it('accepts 30 environments when total_count says 30, in one request', async () => {
    const thirty = Array.from({ length: 30 }, (_, i) => env(`e${i}`))
    const { fetchImpl, urls } = stubFetch([{ total_count: 30, environments: thirty }])
    assert.equal((await fetchEnvironments(opts(fetchImpl))).length, 30)
    assert.equal(urls.length, 1)
  })

  it('refuses a list shorter than its own total_count', async () => {
    const { fetchImpl } = stubFetch([
      { total_count: 3, environments: [env('a')] },
      { total_count: 3, environments: [] },
    ])
    await assert.rejects(fetchEnvironments(opts(fetchImpl)), /read 1 but total_count says 3/)
  })

  it('refuses a response without total_count', async () => {
    const { fetchImpl } = stubFetch([{ environments: [env('a')] }])
    await assert.rejects(fetchEnvironments(opts(fetchImpl)), Instrument)
  })

  it('refuses an empty token before asking anything', async () => {
    const { fetchImpl, urls } = stubFetch([])
    await assert.rejects(fetchEnvironments({ ...opts(fetchImpl), token: '' }), /GITHUB_TOKEN is empty/)
    assert.deepEqual(urls, [])
  })
})
