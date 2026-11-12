// Ported from KlangHaus/grundtone@36f9227c (vitest there); node:test here, so the workflow runs
// it without an install. The cells are the same.
import assert from 'node:assert/strict'
import { execFile } from 'node:child_process'
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import { createServer } from 'node:http'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { after, before, describe, it } from 'node:test'
import { fileURLToPath } from 'node:url'

/**
 * End-to-end cells: they run the REAL guard as a process against a local stand-in for the GitHub
 * API and read exit code and printed line. The decisions are tested in-process in
 * scripts/lib/secret-hygiene.test.mjs; these cover what a library test cannot see — that every
 * way the environments lookup can fail ends in exit 2 with a message, never in a pass.
 */

const GUARD = fileURLToPath(new URL('./secret-hygiene.mjs', import.meta.url))

// A clean corpus for surface A: one gated secret, both allowlisted SONAR_* secrets consumed
// on a pull_request-only workflow, so the allowlist's staleness check stays quiet, and the one
// finding the live EXEMPTIONS list names, so that list is not stale here. When the exemption is
// dropped, drop semantic-release.yml from this corpus too.
const WORKFLOWS = {
  'semantic-release.yml':
    'on:\n  push:\njobs:\n  release:\n    steps:\n      - with:\n          private-key: ${{ secrets.SEMANTIC_RELEASE_APP_PRIVATE_KEY }}\n',
  'deploy.yml':
    'on:\n  push:\njobs:\n  deploy:\n    environment: prod\n    env:\n      K: ${{ secrets.DEPLOY_KEY }}\n',
  'sonar.yml':
    'on:\n  pull_request:\njobs:\n  scan:\n    env:\n      T: ${{ secrets.SONAR_TOKEN }}\n      H: ${{ secrets.SONAR_HOST_URL }}\n',
}
const GOOD = {
  total_count: 2,
  environments: [
    { name: 'copilot', deployment_branch_policy: null },
    { name: 'prod', deployment_branch_policy: { custom_branch_policies: true } },
  ],
}

let server
let apiUrl
let reply = { status: 200, body: GOOD }
let dir

before(async () => {
  dir = mkdtempSync(join(tmpdir(), 'secret-hygiene-'))
  mkdirSync(join(dir, 'workflows'))
  for (const [name, text] of Object.entries(WORKFLOWS)) {
    writeFileSync(join(dir, 'workflows', name), text)
  }
  server = createServer((req, res) => {
    // `body` may be a function of the requested page, for the paging cell.
    const page = Number(new URL(req.url, 'http://localhost').searchParams.get('page'))
    const body = typeof reply.body === 'function' ? reply.body(page) : reply.body
    res.writeHead(reply.status, { 'content-type': 'application/json' })
    res.end(JSON.stringify(body))
  })
  await new Promise((resolve) => server.listen(0, 'localhost', resolve))
  apiUrl = `http://localhost:${server.address().port}`
})

after(async () => {
  await new Promise((resolve) => server.close(resolve))
  rmSync(dir, { recursive: true, force: true })
})

function run(token) {
  return new Promise((resolve) => {
    execFile(
      process.execPath,
      [GUARD],
      {
        env: {
          ...process.env,
          GITHUB_TOKEN: token,
          GITHUB_API_URL: apiUrl,
          GITHUB_REPOSITORY: 'o/r',
          KH_WORKFLOW_DIR: join(dir, 'workflows'),
        },
        encoding: 'utf8',
      },
      (error, stdout) => resolve({ code: error ? error.code : 0, stdout }),
    )
  })
}

const thirtyOfThirtyOne = (page) => ({
  total_count: 31,
  environments:
    page === 1
      ? Array.from({ length: 30 }, (_, i) => ({
          name: `e${i}`,
          deployment_branch_policy: { custom_branch_policies: true },
        }))
      : [],
})

describe('secret-hygiene.mjs exit codes', () => {
  // The control first: without it, the exit-2 cells could pass on a guard that always exits 2.
  it('exits 0 on a clean corpus and a complete environment list', async () => {
    reply = { status: 200, body: GOOD }
    const r = await run('t')
    assert.match(r.stdout, /^EXEMPT: semantic-release\.yml:7 SH-A1 secrets\.SEMANTIC_RELEASE_APP_PRIVATE_KEY — tracked in ClickUp /m)
    assert.match(r.stdout, /result: passed/)
    assert.equal(r.code, 0)
  })

  const failures = [
    ['an empty token', '', { status: 200, body: GOOD }, /GITHUB_TOKEN is empty/],
    ['HTTP 403', 't', { status: 403, body: {} }, /answered HTTP 403/],
    ['a response with no array', 't', { status: 200, body: { total_count: 1 } }, /carries no array/],
    ['0 environments', 't', { status: 200, body: { total_count: 0, environments: [] } }, /0 environments returned/],
    ['total_count 31 with only 30 served', 't', { status: 200, body: thirtyOfThirtyOne }, /read 30 but total_count says 31/],
  ]
  for (const [label, token, served, message] of failures) {
    it(`exits 2 with a message on ${label}`, async () => {
      reply = served
      const r = await run(token)
      assert.match(r.stdout, message)
      assert.match(r.stdout, /result: not measured/)
      assert.equal(r.code, 2)
    })
  }
})
