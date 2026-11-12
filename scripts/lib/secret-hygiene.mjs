// Decision logic of the secret-hygiene guard. The script that prints, calls the API and picks
// the exit code is scripts/secret-hygiene.mjs, which carries the full rationale and review
// history. This file has no dependencies on purpose: the workflow runs it without an install.

import { readdirSync, readFileSync } from 'node:fs'
import { join } from 'node:path'
import process from 'node:process'

export const WORKFLOW_DIR = process.env.KH_WORKFLOW_DIR ?? '.github/workflows'

// GITHUB_TOKEN is minted per run, scoped by `permissions:`, and cannot be read out of the repo's
// settings. It is not a stored secret, so an environment gate would mean nothing for it.
const BUILTIN = new Set(['GITHUB_TOKEN'])

// Triggers whose `github.ref` can never match a deployment branch policy: a pull_request run has
// `refs/pull/N/merge`. A workflow triggered ONLY by these cannot be environment-gated at all, so
// demanding an environment of it is demanding the impossible.
const UNGATEABLE_TRIGGERS = new Set(['pull_request', 'pull_request_target'])

/**
 * Secrets allowed at repo level, each with the measured reason it cannot be environment-gated.
 * An entry is DEBT: it needs a reason a later reader can falsify, and the staleness checks below
 * fail when the debt is gone.
 */
export const SECRET_ALLOWLIST = {
  // sonar.yml runs on pull_request (and on push to main). A pull_request run's github.ref is
  // refs/pull/N/merge, which no branch policy can match, so an environment would not restrict the
  // scan, it would switch it off on every PR. Analysis token, not a publish key. Fork PRs get no
  // secrets because sonar.yml uses pull_request, not pull_request_target (measured 2026-11-12 on
  // resonans 9bfb7812: the only pull_request_target TRIGGER is cla.yml, which consumes no
  // non-builtin secret by this guard's own count).
  // Terminator: if the Sonar token ever becomes publish-capable, this entry stops being valid.
  SONAR_TOKEN: 'consumed on PR refs, which no branch policy can match; analysis-only',
  SONAR_HOST_URL: 'consumed on PR refs, which no branch policy can match; not a credential',
}

/** Environments allowed without a branch policy, each with a reason. */
export const ENV_ALLOWLIST = {
  // Terminator: holds 0 secrets and 0 variables (measured on resonans 2026-11-12). The day it
  // holds one this entry is wrong — the staleness check below fails when it grows a policy or
  // disappears.
  copilot: 'created by the Copilot integration; holds no secrets or variables',
}

export class Instrument extends Error {}

/**
 * Strip comments the way a reader would, not the way a careless regex would.
 * 🔴 This exists because the author of this guard grepped `secrets.SCORECARD_TOKEN` on 2026-10-22,
 * counted a commented-out line as a live reference, and reported a finding that was not there.
 * A search surface holds both the event and the description of it.
 * @param {string} line
 */
function isComment(line) {
  return line.trimStart().startsWith('#')
}

/**
 * Classify every `secrets` token on a line. TOKEN ACCOUNTING, not pattern-hunting
 * ([grundtone]'s idea, 2026-10-22): each occurrence must be attributed to a KNOWN form, and an
 * occurrence that is not becomes an instrument error. The alternative is whack-a-mole — v2 knew
 * only `secrets.X`, and three valid forms walked straight past it: `secrets: inherit`,
 * `secrets['X']` and `toJSON(secrets)`. Accounting makes the NEXT unknown form loud instead of
 * silent, which is the same backstop logic as the tab-indentation case.
 * @param {string} line
 * @returns {{named: string[], inherit: boolean, unknown: string|null}}
 */
export function classify(line) {
  /** @type {string[]} */
  const named = []

  // 🔴 ONLY EXPRESSION CONTEXTS COUNT. v3's first draft ran the accounting over the whole line and
  // refused to read deploy-web.yml because a STEP NAME said "deploy-secrets findes" — \b treats the
  // hyphen as a word boundary, so prose contained the token. A guard that cannot read a file whose
  // step name mentions secrets is a guard someone switches off. GitHub resolves `secrets` only
  // inside ${{ }}, plus the implicit expression in an `if:` value, so those are the only places a
  // token can mean anything. Found by the regression run against the real workflows, not by the
  // self-test — every fixture was synthetic and none of them contained prose.
  const spans = [...line.matchAll(/\$\{\{([\s\S]*?)\}\}/g)].map((m) => m[1] ?? '')
  // `\s*(.*)` would let two quantifiers compete for the same spaces (super-linear backtracking,
  // Sonar S5852); capture everything after the colon and trim instead.
  const ifValue = /^\s*if:(.*)$/.exec(line)
  if (ifValue) spans.push((ifValue[1] ?? '').trim())
  let rest = spans.join(' ')

  // `secrets: inherit` on a caller job passes the WHOLE set to the called workflow without naming
  // anything. Handled before the key-form below, because it is a key AND a hazard.
  // These two are YAML KEYS on the raw line, not expressions, so they are matched before the
  // expression spans are isolated above.
  // An inline comment after `inherit` is still YAML for `inherit` (Bugbot on #291: v4 read
  // `secrets: inherit # all of them` as no inherit at all, and the ungated caller passed).
  const inherit = /(^|\s)secrets:\s*inherit\s*(#.*)?$/.test(line)

  // `secrets.NAME`
  for (const m of rest.matchAll(/secrets\.(\w+)/g)) named.push(m[1] ?? '')
  rest = rest.replace(/secrets\.\w+/g, '')

  // `secrets['NAME']` / `secrets["NAME"]` — valid expression syntax that v2 could not see.
  for (const m of rest.matchAll(/secrets\[\s*['"](\w+)['"]\s*\]/g)) named.push(m[1] ?? '')
  rest = rest.replace(/secrets\[\s*['"]\w+['"]\s*\]/g, '')

  // Anything left is a `secrets` token in a form this guard does not understand — `toJSON(secrets)`
  // being the obvious one. Flagging the unknown beats trying to parse it.
  const leftover = /\bsecrets\b/.test(rest) ? line.trim() : null
  return { named, inherit, unknown: leftover }
}

/** @param {string} s */
function secretsIn(s) {
  return classify(s).named
}

/**
 * Indentation of the first meaningful key in a block, or null when the first meaningful line is
 * not a key matching `pattern`. With `stopAtTopLevel`, an unindented line ends the search.
 * @param {string[]} body
 * @param {RegExp} pattern its first group must capture the leading spaces
 * @param {boolean} stopAtTopLevel
 * @returns {number|null}
 */
function firstKeyIndent(body, pattern, stopAtTopLevel) {
  for (const line of body) {
    if (!line.trim() || isComment(line)) continue
    if (stopAtTopLevel && /^\S/.test(line)) return null
    const m = pattern.exec(line)
    return m ? (m[1] ?? '').length : null
  }
  return null
}

/** @param {string} line an `on: ...` line with its value inline */
function inlineTriggers(line) {
  const rest = line.replace(/^on:\s*/, '').trim()
  const names = rest.replace(/[[\]]/g, '').split(',').map((x) => x.trim()).filter(Boolean)
  return names.length > 0 ? new Set(names) : null
}

/** @param {string[]} body the lines after a bare `on:` */
function blockTriggers(body) {
  // 🔴 Derive the trigger indentation and match ONLY that level. v2's first draft used
  // /^\s{1,4}(\w+):/ and therefore counted NESTED keys: `branches: [main]` under
  // `pull_request:` came back as a trigger named 'branches', so a pull_request-only workflow
  // looked like it had two triggers and the "cannot be gated" message never fired. Caught by the
  // self-test, which is the only reason it is not in the version handed over.
  const indent = firstKeyIndent(body, /^( +)[A-Za-z_]+:/, true)
  if (indent === null) return null
  const key = new RegExp(`^ {${indent}}([A-Za-z_]+):`)
  const out = new Set()
  for (const line of body) {
    if (!line.trim() || isComment(line)) continue
    if (/^\S/.test(line)) break // next top-level key ends the block
    const m = key.exec(line)
    if (m) out.add(m[1] ?? '')
  }
  return out.size > 0 ? out : null
}

/**
 * The event names a workflow triggers on. Returns null when the `on:` block could not be parsed —
 * deliberately NOT an error: an unknown trigger set only costs us the sharper message, and the
 * generic one ("gate it or allowlist it") asserts nothing false. Failing closed here would make
 * the guard brittle on a file shape that is otherwise fine.
 * @param {string} text
 * @returns {Set<string>|null}
 */
export function triggers(text) {
  const lines = text.split('\n')
  const inline = lines.find((l) => /^on:\s*\S/.test(l))
  if (inline) return inlineTriggers(inline)
  const at = lines.findIndex((l) => /^on:\s*$/.test(l))
  if (at === -1) return null
  return blockTriggers(lines.slice(at + 1))
}

/**
 * Collapse every `${{ ... }}` span onto one line before any line-bound analysis.
 * 🔴 [grundtone]'s fourth hole, 2026-10-22, and it is the fleet rule in its purest form: a
 * line-bound pattern cannot see a sentence that spans lines. YAML folds a plain scalar, so
 *     K: ${{
 *       secrets.LEAKY_KEY }}
 * is exactly `${{ secrets.LEAKY_KEY }}` to Actions — actionlint accepts it — while a per-line
 * reader never sees a context that opens on one line and closes on the next. v3 answered
 * "passed", exit 0.
 * @param {string} text
 */
export function foldExpressions(text) {
  return text.replace(/\$\{\{[\s\S]*?\}\}/g, (m) => m.replace(/\n\s*/g, ' '))
}

/**
 * 🔴 The fold has a failure mode of its own: an UNCLOSED `${{` makes the non-greedy match run on
 * to the next `}}` far below, swallowing whatever lies between — job keys included. So count the
 * job keys before and after and refuse if the fold ate structure. Folding is a repair; a repair
 * that can destroy what it repairs has to be measured too.
 * @param {string} name
 * @param {string} rawText
 * @param {string} text the folded text
 */
function assertFoldKeptKeys(name, rawText, text) {
  const jobKeyCount = (t) =>
    t.split('\n').filter((l) => /^ +[A-Za-z0-9_-]+:\s*(#.*)?$/.test(l)).length
  if (jobKeyCount(text) < jobKeyCount(rawText)) {
    throw new Instrument(
      `${name}: folding \`\${{ ... }}\` spans removed ${jobKeyCount(rawText) - jobKeyCount(text)} ` +
        `key line(s) — there is probably an unclosed \`\${{\`, and the structure can no longer be ` +
        `trusted`,
    )
  }
}

/** @param {string} value a YAML scalar, minus a trailing ` # comment` */
function stripInlineComment(value) {
  return value.replace(/\s#.*$/, '').trim()
}

/**
 * The environment name a job-level `environment:` value names, or null when it cannot be
 * resolved statically (an expression, or a mapping without `name:`).
 * @param {string} raw
 */
function environmentName(raw) {
  let v = stripInlineComment(raw)
  if (v.startsWith('{')) v = /\bname:([^,}]+)/.exec(v)?.[1]?.trim() ?? ''
  // Unquote BEFORE the emptiness check (Bugbot on #293): `environment: ""` was a non-empty
  // string until the quotes came off, so it resolved to the name '' and counted as a gate. GitHub
  // reads an empty name as no environment at all.
  v = v.replace(/^['"]|['"]$/g, '').trim()
  if (!v || v.includes('${{')) return null
  return v
}

/**
 * Track the job-level `environment:` and the environment it names.
 * 🔴 Only at the job's OWN child indentation (Bugbot on #291). v4 accepted `environment:` at any
 * depth below the job key, so a step input (`with: environment: prod`) or a matrix key named
 * `environment` marked an ungated job as gated, and its secrets passed. The child indentation is
 * DERIVED from the job's first line, like the job indentation itself, not assumed.
 * @param {Job} job
 * @param {string} line
 */
function readEnvironment(job, line) {
  if (!line.trim()) return
  const indent = line.length - line.trimStart().length
  job.childIndent ??= indent
  if (job.envIndent !== null && indent > job.envIndent) {
    const name = /^\s*name:(.*)$/.exec(line)
    if (name) job.environment = environmentName(name[1] ?? '')
    return
  }
  job.envIndent = null
  if (indent !== job.childIndent) return
  const env = /^\s*environment:(.*)$/.exec(line)
  if (!env) return
  job.hasEnvironment = true
  const value = stripInlineComment(env[1] ?? '')
  if (value) job.environment = environmentName(value)
  else job.envIndent = indent
}

/**
 * @typedef {{job: string, secrets: Set<string>, hasEnvironment: boolean,
 *   environment: string|null|undefined, inherit: boolean,
 *   childIndent: number|null, envIndent: number|null}} Job
 *   `environment` is undefined when the job names none, null when the name cannot be resolved.
 */

/** @param {string} job */
function newJob(job) {
  return {
    job,
    secrets: new Set(),
    hasEnvironment: false,
    environment: undefined,
    inherit: false,
    childIndent: null,
    envIndent: null,
  }
}

/**
 * Attribute one line under `jobs:` to the current job, or record why it cannot be.
 * @param {Job|null} current
 * @param {string} line
 * @param {{unattributed: string[], unknownForms: string[]}} acc
 */
function attributeLine(current, line, acc) {
  const c = classify(line)
  if (c.unknown) acc.unknownForms.push(c.unknown)
  if (!current) {
    // 🔴 THE BACKSTOP. In v1 this line read `if (!current) continue`, and a four-space job block
    // therefore vanished in silence. A secret under `jobs:` that belongs to no job we recognised
    // means the parser is wrong about this file, and a wrong parser must not produce a verdict.
    acc.unattributed.push(...c.named)
    return
  }
  if (c.inherit) current.inherit = true
  readEnvironment(current, line)
  for (const s of c.named) current.secrets.add(s)
}

/**
 * @param {string} name
 * @param {number} indent
 * @param {{unattributed: string[], unknownForms: string[]}} acc
 */
function assertFullyAttributed(name, indent, acc) {
  if (acc.unknownForms.length > 0) {
    throw new Instrument(
      `${name}: a \`secrets\` token in a form this guard does not understand — ` +
        `${JSON.stringify(acc.unknownForms[0])}. Known forms are secrets.NAME, secrets['NAME'] and ` +
        `secrets: inherit. An unknown form must be loud, not silently skipped.`,
    )
  }
  if (acc.unattributed.length > 0) {
    throw new Instrument(
      `${name}: consumes ${[...new Set(acc.unattributed)]} under \`jobs:\` but it could not be ` +
        `attributed to any job at indentation ${indent} — the parser does not understand this ` +
        `file's shape, so its verdict would be worthless`,
    )
  }
}

/**
 * Jobs of a workflow file, each with the secrets it consumes and whether it declares an
 * environment. The job-key indentation is DERIVED from the first key under `jobs:` rather than
 * assumed, and anything consuming a secret that cannot be attributed to a job is an instrument
 * error — see the review note in the file header.
 * @param {string} name
 * @param {string} text
 * @returns {Job[]}
 */
export function parseJobs(name, rawText) {
  const text = foldExpressions(rawText)
  assertFoldKeptKeys(name, rawText, text)
  const lines = text.split('\n')
  const jobsAt = lines.findIndex((l) => /^jobs:\s*$/.test(l))

  // Anything above `jobs:` (a top-level `env:`, for instance) belongs to no job and can therefore
  // never be environment-gated. Reported under the pseudo-job "<file-level>".
  const head = lines.slice(0, jobsAt === -1 ? lines.length : jobsAt)
  const headSecrets = new Set(head.filter((l) => !isComment(l)).flatMap(secretsIn))

  if (jobsAt === -1) {
    if (headSecrets.size > 0) {
      throw new Instrument(`${name} consumes ${[...headSecrets]} but has no \`jobs:\` block`)
    }
    return []
  }

  const body = lines.slice(jobsAt + 1)

  // Derive the job-key indentation from the first meaningful line under `jobs:`.
  const indent = firstKeyIndent(body, /^( +)[A-Za-z0-9_-]+:/, false)
  if (indent === null) {
    throw new Instrument(`${name}: could not derive the job indentation from the first key under \`jobs:\``)
  }

  // A job key may carry an inline comment (`deploy: # prod only`) — Bugbot on #291: v4 matched only
  // a bare key, so a commented job never began and its secrets were attributed to the job above,
  // inheriting THAT job's environment.
  const jobKey = new RegExp(String.raw`^ {${indent}}([A-Za-z0-9_-]+):\s*(#.*)?$`)
  // 🔴 And the backstop for the next shape nobody thought of: any other line at the job
  // indentation is something this parser does not understand, so it refuses instead of guessing.
  const atJobLevel = new RegExp(String.raw`^ {${indent}}\S`)

  /** @type {Job[]} */
  const out = []
  if (headSecrets.size > 0) out.push({ ...newJob('<file-level>'), secrets: headSecrets })

  let current = null
  const acc = { unattributed: [], unknownForms: [] }
  for (const line of body) {
    const m = jobKey.exec(line)
    if (m) {
      current = newJob(m[1] ?? '')
      out.push(current)
      continue
    }
    if (isComment(line)) continue
    if (atJobLevel.test(line)) {
      throw new Instrument(
        `${name}: a line at job indentation ${indent} is not a job key this guard understands — ` +
          `${JSON.stringify(line.trim())}`,
      )
    }
    attributeLine(current, line, acc)
  }
  assertFullyAttributed(name, indent, acc)
  return out
}

// ───────────────────────────────────────────── surface A: what the workflows consume

/**
 * Why a job's secrets are NOT behind a working gate, or null when they are.
 * 🔴 `environment:` alone is not the gate (Bugbot on #291). An environment in ENV_ALLOWLIST has
 * no branch policy by definition — surface B allows exactly that — so naming it gated nothing
 * while both surfaces stayed green. A name this guard cannot resolve cannot be confirmed either.
 * @param {Job} job
 * @param {Set<string>} ungatedEnvs
 * @returns {string|null}
 */
function gateGap(job, ungatedEnvs) {
  if (!job.hasEnvironment) return 'without an `environment:`'
  if (typeof job.environment !== 'string') {
    return 'behind an `environment:` whose name this guard cannot resolve (an expression, or no `name:`)'
  }
  if (ungatedEnvs.has(job.environment)) {
    return `behind environment '${job.environment}', which is in ENV_ALLOWLIST and so has no branch policy`
  }
  return null
}

/**
 * @typedef {{rule: string, file: string, line: number, job: string, secret: string, text: string}} Finding
 *   A violation with the location and rule id it is reported under. `text` is NEUTRAL by
 *   decision (Sikkerhed 2026-11-12, resonans is public and the text lands in a public Actions
 *   log): file:line, the rule id and what the configuration lacks, never what that exposes.
 *   `file`, `job` and `secret` are '' where they do not apply.
 */

/**
 * The 1-based line of the first non-comment line after `job`'s key that satisfies `hit`, falling
 * back to the job key's own line. Reads the RAW text: folding `${{ }}` spans shifts line numbers.
 * @param {string} rawText
 * @param {string} job
 * @param {(line: string) => boolean} hit
 */
function lineOf(rawText, job, hit) {
  const lines = rawText.split('\n')
  const key = new RegExp(String.raw`^ +${job.replace(/[^A-Za-z0-9_-]/g, '')}:\s*(#.*)?$`)
  const at = job === '<file-level>' ? -1 : lines.findIndex((l) => key.test(l))
  for (let i = at + 1; i < lines.length; i += 1) {
    if (!isComment(lines[i]) && hit(lines[i])) return i + 1
  }
  return at + 1
}

/** @param {string} secret */
const consumes = (secret) => (l) =>
  l.includes(`secrets.${secret}`) || new RegExp(String.raw`secrets\[\s*['"]${secret}['"]\s*\]`).test(l)

/**
 * @param {string} name
 * @param {string} job
 * @param {string} secret
 * @param {string} gap from gateGap
 * @param {Set<string>|null} ungateable the triggers, when ALL of them are ungateable
 * @param {number} line
 * @returns {Finding}
 */
function ungatedFinding(name, job, secret, gap, ungateable, line) {
  const head = `${name}:${line} SH-A1 job '${job}' consumes secrets.${secret} ${gap}`
  const tail = ungateable
    ? ` — the workflow triggers only on ${[...ungateable].join(', ')}, so it CANNOT be ` +
      `environment-gated; add it to SECRET_ALLOWLIST with that reason, or remove the reference`
    : ` — gate it with an environment that has a branch policy, or add it to SECRET_ALLOWLIST ` +
      `with the measured reason it cannot be gated`
  return { rule: 'SH-A1', file: name, line, job, secret, text: head + tail }
}

/**
 * @param {string} name
 * @param {string} rawText
 * @param {Job} job
 * @param {Record<string,string>} allow
 * @param {Set<string>} ungatedEnvs
 * @param {Set<string>|null} ungateable
 * @returns {{seen: number, bad: Finding[]}}
 */
function jobFindings(name, rawText, job, allow, ungatedEnvs, ungateable) {
  /** @type {Finding[]} */
  const bad = []
  let seen = 0
  const gap = gateGap(job, ungatedEnvs)
  // `secrets: inherit` hands the called workflow EVERY secret the caller can see, so there is
  // no name to gate. [grundtone]'s scope, deliberately narrow: flag it when the job has no
  // environment. Residual NOT asserted here: inherit WITH an environment is still broader than
  // any single job needs, but that is a design finding, not this guard's claim.
  if (job.inherit && gap) {
    seen += 1
    const line = lineOf(rawText, job.job, (l) => /secrets:\s*inherit/.test(l))
    bad.push({
      rule: 'SH-A2',
      file: name,
      line,
      job: job.job,
      secret: '',
      text:
        `${name}:${line} SH-A2 job '${job.job}' uses \`secrets: inherit\` ${gap} — name the ` +
        `secrets it needs, or gate the job with an environment that has a branch policy`,
    })
  }
  for (const secret of job.secrets) {
    if (BUILTIN.has(secret)) continue
    seen += 1
    if (secret in allow || !gap) continue
    const line = lineOf(rawText, job.job, consumes(secret))
    bad.push(ungatedFinding(name, job.job, secret, gap, ungateable, line))
  }
  return { seen, bad }
}

/**
 * An allowlist entry must not outlive its debt.
 * @param {[string,string][]} files
 * @param {Record<string,string>} allow
 * @returns {Finding[]}
 */
function allowlistDebt(files, allow) {
  /** @type {Finding[]} */
  const bad = []
  const debt = (secret, text) => bad.push({ rule: 'SH-A3', file: '', line: 0, job: '', secret, text })
  for (const [secret, reason] of Object.entries(allow)) {
    if (!reason.trim()) debt(secret, `SH-A3 SECRET_ALLOWLIST['${secret}'] has an empty reason`)
    const used = files.some(([, t]) =>
      t.split('\n').some((l) => !isComment(l) && l.includes(`secrets.${secret}`)),
    )
    if (!used) debt(secret, `SH-A3 SECRET_ALLOWLIST['${secret}'] is stale — no workflow consumes it; drop it`)
  }
  return bad
}

/**
 * @param {[string,string][]} files
 * @param {Record<string,string>} allow injectable so a self-test cell is not contaminated by the
 *   live allowlist — the staleness check would otherwise turn every small fixture red.
 * @param {string[]} ungatedEnvs environments known to have NO branch policy; naming one gates nothing.
 * @returns {Finding[]}
 */
export function findingsA(files, allow = SECRET_ALLOWLIST, ungatedEnvs = Object.keys(ENV_ALLOWLIST)) {
  const noPolicy = new Set(ungatedEnvs)
  /** @type {Finding[]} */
  const bad = []
  let seen = 0
  for (const [name, text] of files) {
    const trig = triggers(text)
    const ungateable = trig !== null && [...trig].every((t) => UNGATEABLE_TRIGGERS.has(t)) ? trig : null
    for (const job of parseJobs(name, text)) {
      const r = jobFindings(name, text, job, allow, noPolicy, ungateable)
      seen += r.seen
      bad.push(...r.bad)
    }
  }
  // Positive control: this repo is known to consume secrets. Zero means the parser broke, not that
  // the repo is clean — and a clean-looking zero is the most actionable wrong answer available.
  if (seen === 0) {
    throw new Instrument(
      `parsed ${files.length} workflow file(s) and found 0 non-builtin secret references — ` +
        `the parser is broken, not the repo`,
    )
  }
  bad.push(...allowlistDebt(files, allow))
  return bad
}

/**
 * The violation texts of surface A, before exemptions. The self-test cells read this form.
 * @param {[string,string][]} files
 * @param {Record<string,string>} [allow]
 * @param {string[]} [ungatedEnvs]
 * @returns {string[]}
 */
export function surfaceA(files, allow, ungatedEnvs) {
  return findingsA(files, allow, ungatedEnvs).map((f) => f.text)
}

// ───────────────────────────────────────────── surface B: environments and branch policies

/**
 * Every environment of the repo, all pages. Each way this can fail is an instrument error, never
 * an empty list: an empty list would read as "no environment lacks a policy".
 * 🔴 Paginated (Bugbot on #291). v4 made one request with GitHub's default page size of 30 and
 * never compared it with `total_count`, so an unprotected 31st environment would never have been
 * seen while the run exited 0. Measured on grundtone with per_page=2: total_count=5, returned=2.
 * @param {{token: string|undefined, repo: string, apiUrl: string, fetchImpl?: typeof fetch}} o
 */
export async function fetchEnvironments({ token, repo, apiUrl, fetchImpl = globalThis.fetch }) {
  if (!token) throw new Instrument('GITHUB_TOKEN is empty — SETUP failure, not a finding')
  /** @type {{name:string, deployment_branch_policy:unknown}[]} */
  const envs = []
  let total = Infinity
  let page = 1
  while (envs.length < total) {
    const res = await fetchImpl(`${apiUrl}/repos/${repo}/environments?per_page=100&page=${page}`, {
      headers: { authorization: `Bearer ${token}`, accept: 'application/vnd.github+json' },
    })
    if (!res.ok) {
      throw new Instrument(
        `GET /repos/${repo}/environments answered HTTP ${res.status} as the workflow token — ` +
          `SETUP failure, not a finding. Needs repository read; check \`permissions:\`.`,
      )
    }
    const body = await res.json()
    const batch = body?.environments
    if (!Array.isArray(batch)) throw new Instrument('environments response carries no array')
    if (typeof body.total_count !== 'number') {
      throw new Instrument('environments response carries no total_count — cannot tell if the list is whole')
    }
    total = body.total_count
    // An empty page before the count is reached would loop forever; it is also a list that
    // contradicts its own total, which is exactly what this function must not pass on.
    if (batch.length === 0) break
    envs.push(...batch)
    page += 1
  }
  if (envs.length !== total) {
    throw new Instrument(`environments: read ${envs.length} but total_count says ${total} — incomplete list`)
  }
  if (envs.length === 0) throw new Instrument('0 environments returned — cannot discriminate')
  return envs
}

/**
 * @param {{name:string, deployment_branch_policy:unknown}[]} envs
 * @param {Record<string,string>} allow injectable, same reason as findingsA.
 * @returns {Finding[]}
 */
export function findingsB(envs, allow = ENV_ALLOWLIST) {
  /** @type {Finding[]} */
  const bad = []
  const add = (rule, text) => bad.push({ rule, file: '', line: 0, job: '', secret: '', text })
  for (const env of envs) {
    const name = env.name
    const hasPolicy = env.deployment_branch_policy != null
    if (name in allow) {
      if (!allow[name].trim()) add('SH-B2', `SH-B2 ENV_ALLOWLIST['${name}'] has an empty reason`)
      if (hasPolicy) add('SH-B2', `SH-B2 ENV_ALLOWLIST['${name}'] is stale — it now has a policy; drop it`)
      continue
    }
    if (!hasPolicy) add('SH-B1', `SH-B1 environment '${name}' has no deployment branch policy`)
  }
  for (const name of Object.keys(allow)) {
    if (!envs.some((e) => e.name === name)) {
      add('SH-B2', `SH-B2 ENV_ALLOWLIST['${name}'] names an environment that does not exist; drop it`)
    }
  }
  return bad
}

/**
 * The violation texts of surface B. The self-test cells read this form.
 * @param {{name:string, deployment_branch_policy:unknown}[]} envs
 * @param {Record<string,string>} [allow]
 * @returns {string[]}
 */
export function surfaceB(envs, allow) {
  return findingsB(envs, allow).map((f) => f.text)
}

// ───────────────────────────────────────────── exemptions

/**
 * A finding that may stand for a LIMITED time, by name. Unlike SECRET_ALLOWLIST (a secret that
 * cannot be gated, for as long as the reason holds), an exemption covers ONE finding (file, job,
 * rule, secret) that CAN be fixed and is not yet: it names the task that fixes it, an owner, and
 * an expiry at most MAX_EXEMPTION_DAYS after it was added. On the day after expiry the finding
 * is red again, so the debt cannot go quiet. Text stays neutral (see Finding).
 */
export const EXEMPTIONS = [
  {
    file: 'semantic-release.yml',
    job: 'release',
    rule: 'SH-A1',
    secret: 'SEMANTIC_RELEASE_APP_PRIVATE_KEY',
    task: '1amk32fnp6n',
    owner: 'Allan',
    added: '2026-11-12',
    expires: '2026-11-26',
  },
]

export const MAX_EXEMPTION_DAYS = 14

const DAY_MS = 24 * 60 * 60 * 1000

/** @param {string} d YYYY-MM-DD; NaN when it is not a real calendar day */
function dayNumber(d) {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(d)) return Number.NaN
  const t = Date.parse(`${d}T00:00:00Z`)
  return Number.isNaN(t) || new Date(t).toISOString().slice(0, 10) !== d ? Number.NaN : t / DAY_MS
}

/** @param {Date} date */
export function isoDay(date) {
  return date.toISOString().slice(0, 10)
}

/**
 * Why an exemption is not usable, or null.
 * @param {typeof EXEMPTIONS[number]} e
 */
function exemptionDefect(e) {
  if (!e.task?.trim() || !e.owner?.trim()) return 'has no task or no owner'
  const added = dayNumber(e.added)
  const expires = dayNumber(e.expires)
  if (Number.isNaN(added) || Number.isNaN(expires)) return 'has an added or expires value that is not a YYYY-MM-DD day'
  if (expires < added) return 'expires before it was added'
  if (expires - added > MAX_EXEMPTION_DAYS) return `runs longer than ${MAX_EXEMPTION_DAYS} days`
  return null
}

/**
 * Split findings into violations and exempted findings. An exemption that is malformed, expired
 * or matches nothing is itself a violation, and a malformed one suppresses nothing.
 * @param {Finding[]} findings
 * @param {typeof EXEMPTIONS} exemptions injectable, so self-test cells do not read the live list.
 * @param {string} today YYYY-MM-DD (UTC)
 * @returns {{violations: Finding[], exempted: Finding[]}}
 */
export function applyExemptions(findings, exemptions = EXEMPTIONS, today = isoDay(new Date())) {
  /** @type {Finding[]} */
  const violations = []
  /** @type {Finding[]} */
  const exempted = []
  const usable = []
  for (const e of exemptions) {
    const where = `exemption for ${e.file} job '${e.job}' ${e.rule} secrets.${e.secret}`
    const defect = exemptionDefect(e)
    if (defect) {
      violations.push({ rule: 'SH-X3', file: '', line: 0, job: '', secret: e.secret, text: `SH-X3 ${where} ${defect}` })
    } else if (!findings.some((f) => matches(f, e))) {
      violations.push({
        rule: 'SH-X2',
        file: '',
        line: 0,
        job: '',
        secret: e.secret,
        text: `SH-X2 ${where} matches no finding — drop it (tracked in ClickUp ${e.task})`,
      })
    } else {
      usable.push(e)
    }
  }
  for (const f of findings) {
    const e = usable.find((x) => matches(f, x))
    if (!e) {
      violations.push(f)
      continue
    }
    const what = `${f.rule} secrets.${f.secret} — tracked in ClickUp ${e.task}`
    if (dayNumber(today) > dayNumber(e.expires)) {
      violations.push({ ...f, rule: 'SH-X1', text: `${f.file}:${f.line} SH-X1 ${what}, expired ${e.expires}` })
    } else {
      exempted.push({ ...f, text: `${f.file}:${f.line} ${what}, expires ${e.expires}` })
    }
  }
  return { violations, exempted }
}

/**
 * @param {Finding} f
 * @param {typeof EXEMPTIONS[number]} e
 */
function matches(f, e) {
  return f.file === e.file && f.job === e.job && f.rule === e.rule && f.secret === e.secret
}

// ───────────────────────────────────────────── workflow corpus

export function readWorkflows() {
  let names
  try {
    names = readdirSync(WORKFLOW_DIR).filter((f) => /\.ya?ml$/.test(f)).sort((a, b) => a.localeCompare(b))
  } catch (e) {
    throw new Instrument(`cannot read ${WORKFLOW_DIR}: ${e.message}`)
  }
  if (names.length === 0) throw new Instrument(`${WORKFLOW_DIR} holds 0 workflow files`)
  return names.map((n) => [n, readFileSync(join(WORKFLOW_DIR, n), 'utf8')])
}

// ───────────────────────────────────────────── self-test

/**
 * The self-test cells, one axis per cell. Run by `--self-test` in CI (no install) and by
 * secret-hygiene.test.mjs under vitest, so both paths execute the same cells.
 * @returns {{label: string, fn: () => string[], expect: string}[]}
 */
export function selfTestCells() {
  /** @type {{label: string, fn: () => string[], expect: string}[]} */
  const cells = []
  const cell = (label, fn, expect) => cells.push({ label, fn, expect })

  const wf = (body) => [['t.yml', body]]
  const gated = 'jobs:\n  a:\n    environment:\n      name: x\n    env:\n      K: ${{ secrets.MY_KEY }}\n'
  const bare = 'jobs:\n  a:\n    env:\n      K: ${{ secrets.MY_KEY }}\n'

  cell('secret in a job with environment:', () => surfaceA(wf(gated), {}), 'GREEN')
  cell('secret in a job without environment:', () => surfaceA(wf(bare), {}), 'RED')
  cell('allowlisted secret needs no gate', () => surfaceA(wf(bare), { MY_KEY: 'reason' }), 'GREEN')
  cell('allowlist entry with an empty reason', () => surfaceA(wf(bare), { MY_KEY: '  ' }), 'RED')
  cell('allowlist entry nothing consumes is stale', () =>
    surfaceA(wf(gated), { MY_KEY: 'r', GONE_KEY: 'r' }), 'RED')

  // GITHUB_TOKEN must be ignored even in an ungated job. MY_KEY in the same job is the
  // discriminator: without it the cell would pass on an empty result and prove nothing.
  cell('secrets.GITHUB_TOKEN needs no gate', () => {
    const v = surfaceA(
      wf('jobs:\n  a:\n    env:\n      T: ${{ secrets.GITHUB_TOKEN }}\n      K: ${{ secrets.MY_KEY }}\n'),
      {},
    )
    if (v.length !== 1) throw new Instrument(`expected only MY_KEY reported, got ${v.length}`)
    return v.filter((x) => x.includes('GITHUB_TOKEN'))
  }, 'GREEN')

  cell('commented-out reference does not count', () =>
    surfaceA(wf('jobs:\n  a:\n      # K: ${{ secrets.MY_KEY }}\n    env:\n      G: ${{ secrets.GITHUB_TOKEN }}\n'), {}),
    'INSTRUMENT') // 0 real references -> parser-broken, the correct refusal
  cell('file-level secret can never be gated', () =>
    surfaceA([['t.yml', 'env:\n  K: ${{ secrets.MY_KEY }}\njobs:\n  a:\n    environment:\n      name: x\n']], {}),
    'RED')
  cell('secrets but no jobs: block -> instrument', () =>
    surfaceA(wf('env:\n  K: ${{ secrets.MY_KEY }}\n'), {}), 'INSTRUMENT')
  cell('0 references anywhere -> instrument, not clean', () =>
    surfaceA(wf('jobs:\n  a:\n    runs-on: x\n'), {}), 'INSTRUMENT')

  // 🔴 [grundtone]'s fixture, verbatim in shape: four-space job keys. v1 answered GREEN here and
  // dropped the secret in silence. The second file is what made the miss invisible: it satisfied
  // the `seen > 0` positive control on its own.
  const fourSpace = 'jobs:\n    pub:\n        env:\n            K: ${{ secrets.LEAKY_KEY }}\n'
  // v1 answered GREEN here. v2 DERIVES the indentation, so it now understands the file and reports
  // the real violation — strictly better than the backstop's refusal. My own cell expected
  // INSTRUMENT and was wrong: the backstop is for shapes that stay unparsable, not for this one.
  cell('four-space job keys (grundtone fixture)', () => surfaceA(wf(fourSpace), {}), 'RED')
  cell('four-space job + a second file that satisfies the positive control', () =>
    surfaceA([['a.yml', fourSpace], ['b.yml', bare]], {}), 'RED')
  // 🔴 The cell above cannot tell v1 from v2: `bare` is red on its own, so it stays RED even if
  // a.yml is dropped. This is the review's exact shape — the second file consumes only an
  // ALLOWLISTED secret, so it is green alone and LEAKY_KEY is the only thing that can turn it red.
  // The cell also requires the violation to NAME LEAKY_KEY, so a red for any other reason fails.
  cell('four-space LEAKY_KEY + an allowlisted SONAR_* file (review shape)', () => {
    const sonar = 'jobs:\n  scan:\n    env:\n      T: ${{ secrets.SONAR_TOKEN }}\n'
    const v = surfaceA([['a.yml', fourSpace], ['b.yml', sonar]], { SONAR_TOKEN: 'r' })
    if (!v.some((x) => x.includes('LEAKY_KEY'))) {
      throw new Instrument(`LEAKY_KEY not reported; got ${JSON.stringify(v)}`)
    }
    return v
  }, 'RED')
  // The backstop still has to exist: a shape whose job keys are NOT a consistent indent must
  // refuse rather than drop. Tab-indented keys are the cheapest such shape.
  cell('tab-indented job keys -> instrument, not dropped', () =>
    surfaceA(wf('jobs:\n\tpub:\n\t\tenv:\n\t\t\tK: ${{ secrets.LEAKY_KEY }}\n'), {}), 'INSTRUMENT')
  cell('four-space job keys, gated, parses as one job', () => {
    const jobs = parseJobs('t.yml', 'jobs:\n    pub:\n        environment:\n            name: x\n        env:\n            K: ${{ secrets.MY_KEY }}\n')
    if (jobs.length !== 1) throw new Instrument(`expected 1 job, got ${jobs.length}`)
    if (!jobs[0].hasEnvironment) throw new Instrument('environment: not recognised at this indent')
    return []
  }, 'GREEN')

  // Trigger detection: a pull_request-only workflow cannot be gated, so the message must say that.
  cell('pull_request-only workflow says CANNOT be gated', () => {
    const v = surfaceA(wf('on:\n  pull_request:\n    branches: [main]\n' + bare), {})
    if (v.length !== 1) throw new Instrument(`expected 1 finding, got ${v.length}`)
    if (!v[0].includes('CANNOT be')) throw new Instrument(`wrong message: ${v[0]}`)
    return v
  }, 'RED')
  cell('push-triggered workflow keeps the generic message', () => {
    const v = surfaceA(wf('on:\n  push:\n    branches: [develop]\n' + bare), {})
    if (v.length !== 1) throw new Instrument(`expected 1 finding, got ${v.length}`)
    if (v[0].includes('CANNOT be')) throw new Instrument('claimed ungateable for a push trigger')
    return v
  }, 'RED')
  cell('mixed pull_request + push is gateable', () => {
    const t = triggers('on:\n  pull_request:\n  push:\n    branches: [develop]\n')
    if (t?.size !== 2) throw new Instrument(`expected 2 triggers, got ${t ? [...t] : null}`)
    return []
  }, 'GREEN')
  cell('inline on: [pull_request] is recognised', () => {
    const t = triggers('on: [pull_request]\n')
    if (t?.size !== 1 || !t.has('pull_request')) throw new Instrument(`got ${t ? [...t] : null}`)
    return []
  }, 'GREEN')

  // 🔴 [grundtone]'s three provoked holes, 2026-10-22. v2 answered "passed", exit 0, on all three.
  cell('(a) secrets: inherit without environment', () =>
    surfaceA(wf('jobs:\n  call:\n    uses: ./.github/workflows/deploy.yml\n    secrets: inherit\n'), {}), 'RED')
  cell('(a) secrets: inherit WITH environment is not flagged', () =>
    surfaceA([
      ['a.yml', 'jobs:\n  call:\n    environment:\n      name: x\n    uses: ./x.yml\n    secrets: inherit\n'],
      ['b.yml', bare],
    ], { MY_KEY: 'r' }), 'GREEN')
  cell("(b) secrets['NAME'] bracket form is seen", () =>
    surfaceA(wf("jobs:\n  a:\n    env:\n      K: ${{ secrets['LEAKY_KEY'] }}\n"), {}), 'RED')
  cell('(b) bracket form with double quotes', () =>
    surfaceA(wf('jobs:\n  a:\n    env:\n      K: ${{ secrets["LEAKY_KEY"] }}\n'), {}), 'RED')
  cell('(b) bracket form inside a gated job is fine', () =>
    surfaceA(wf("jobs:\n  a:\n    environment:\n      name: x\n    env:\n      K: ${{ secrets['K'] }}\n"), {}), 'GREEN')
  cell('(c) toJSON(secrets) -> instrument, not a pass', () =>
    surfaceA(wf('jobs:\n  a:\n    env:\n      ALL: ${{ toJSON(secrets) }}\n'), {}), 'INSTRUMENT')
  cell('(c) an unknown form is loud even next to a known one', () =>
    surfaceA([
      ['a.yml', 'jobs:\n  a:\n    env:\n      ALL: ${{ toJSON(secrets) }}\n'],
      ['b.yml', bare],
    ], { MY_KEY: 'r' }), 'INSTRUMENT')
  // 🔴 The regression that the synthetic fixtures could not show: prose is not a reference.
  cell('a step NAME mentioning deploy-secrets is not a reference', () =>
    surfaceA([
      ['a.yml', 'jobs:\n  a:\n    steps:\n      - name: Assert that deploy-secrets exist (fail-loud)\n        run: echo ok\n'],
      ['b.yml', bare],
    ], { MY_KEY: 'r' }), 'GREEN')
  cell('prose plus a real reference still reports the reference', () =>
    surfaceA(wf('jobs:\n  a:\n    steps:\n      - name: check deploy-secrets\n        env:\n          K: ${{ secrets.MY_KEY }}\n'), {}), 'RED')
  cell('explicit secrets: mapping is structural, not unknown', () =>
    surfaceA(wf('jobs:\n  call:\n    environment:\n      name: x\n    uses: ./x.yml\n    secrets:\n      INNER: ${{ secrets.MY_KEY }}\n'), { MY_KEY: 'r' }), 'GREEN')

  // 🔴 [grundtone]'s fourth fixture: an expression folded across two lines. v3 said "passed".
  cell('(d) ${{ opened on one line, closed on the next', () =>
    surfaceA(wf('jobs:\n  a:\n    env:\n      K: ${{\n        secrets.LEAKY_KEY }}\n'), {}), 'RED')
  cell('(d) folded expression inside a gated job is fine', () =>
    surfaceA(wf('jobs:\n  a:\n    environment:\n      name: x\n    env:\n      K: ${{\n        secrets.MY_KEY }}\n'), {}), 'GREEN')
  cell('(d) an UNCLOSED ${{ must not let the fold eat job keys', () =>
    surfaceA(wf('jobs:\n  a:\n    env:\n      K: ${{ secrets.MY_KEY\n  b:\n    env:\n      L: ${{ secrets.OTHER }}\n'), {}), 'INSTRUMENT')
  // [grundtone]'s own negative control: a reusable workflow DECLARES the secrets it accepts under
  // `on: workflow_call:`. A declaration is not a consumption and must not be counted as one.
  cell('workflow_call secret DECLARATION is not consumption', () =>
    surfaceA([
      ['a.yml', 'on:\n  workflow_call:\n    secrets:\n      DEPLOY_KEY:\n        required: true\njobs:\n  a:\n    environment:\n      name: x\n    env:\n      K: ${{ secrets.DEPLOY_KEY }}\n'],
      ['b.yml', bare],
    ], { MY_KEY: 'r' }), 'GREEN')

  // 🔴 Cursor Bugbot's review of #291, 2026-10-26: five shapes that v4 answered GREEN. Each cell is
  // the shape that passed; every one was provoked against v4 before it was fixed.
  cell('Bugbot: a step input named environment: is not the gate', () =>
    surfaceA(wf('jobs:\n  a:\n    steps:\n      - uses: x/y@v1\n        with:\n          environment: prod\n        env:\n          K: ${{ secrets.LEAKY_KEY }}\n'), {}), 'RED')
  cell('Bugbot: a matrix key named environment: is not the gate', () =>
    surfaceA(wf('jobs:\n  a:\n    strategy:\n      matrix:\n        environment: [a, b]\n    env:\n      K: ${{ secrets.LEAKY_KEY }}\n'), {}), 'RED')
  cell('Bugbot: a job key with an inline comment starts its own job', () =>
    surfaceA(wf('jobs:\n  a:\n    environment: prod\n  b: # deploy job\n    env:\n      K: ${{ secrets.LEAKY_KEY }}\n'), {}), 'RED')
  cell('an unknown line at job indentation -> instrument, not guessed', () =>
    surfaceA(wf('jobs:\n  a:\n    environment: prod\n  b: { runs-on: x }\n    env:\n      K: ${{ secrets.LEAKY_KEY }}\n'), {}), 'INSTRUMENT')
  cell('Bugbot: an environment with no branch policy gates nothing', () =>
    surfaceA(wf('jobs:\n  a:\n    environment: copilot\n    env:\n      K: ${{ secrets.LEAKY_KEY }}\n'), {}), 'RED')
  cell('an environment named by an expression cannot be confirmed', () =>
    surfaceA(wf('jobs:\n  a:\n    environment: ${{ inputs.target }}\n    env:\n      K: ${{ secrets.LEAKY_KEY }}\n'), {}), 'RED')
  cell('Bugbot: secrets: inherit with an inline comment is still inherit', () =>
    surfaceA(wf('jobs:\n  call:\n    uses: ./x.yml\n    secrets: inherit # all of them\n'), {}), 'RED')
  cell('Bugbot (#293): environment: "" is no environment', () =>
    surfaceA(wf('jobs:\n  a:\n    environment: ""\n    env:\n      K: ${{ secrets.LEAKY_KEY }}\n'), {}), 'RED')
  cell("Bugbot (#293): environment: { name: '' } is no environment", () =>
    surfaceA(wf("jobs:\n  a:\n    environment: { name: '' }\n    env:\n      K: ${{ secrets.LEAKY_KEY }}\n"), {}), 'RED')
  // The repairs must not turn the valid forms red: the inline name with a comment, the mapping
  // form deploy-web.yml uses, and the flow mapping.
  cell('environment: NAME # comment still gates', () =>
    surfaceA(wf('jobs:\n  a:\n    environment: prod # gated\n    env:\n      K: ${{ secrets.MY_KEY }}\n'), {}), 'GREEN')
  cell('environment: { name: NAME } still gates', () =>
    surfaceA(wf("jobs:\n  a:\n    environment: { name: 'prod', url: x }\n    env:\n      K: ${{ secrets.MY_KEY }}\n"), {}), 'GREEN')

  // 🔴 THE REAL-CORPUS CELL. The lesson of v3: 32 synthetic cells could not find the prose false
  // positive, because every fixture was written by the author of the parser. A test suite with no
  // case drawn from the real corpus measures the author's imagination. This cell requires the
  // repo's own workflows to parse without an instrument error — it is meant to run from the repo
  // root, and failing loudly elsewhere is the point.
  cell('the repo\'s OWN workflows parse without an instrument error', () => {
    const files = readWorkflows()
    let total = 0
    for (const [n, t] of files) for (const j of parseJobs(n, t)) total += j.secrets.size
    if (total === 0) throw new Instrument(`${files.length} real files yielded 0 secrets`)
    console.log(`       (real corpus: ${files.length} files, ${total} attributed secret references)`)
    return []
  }, 'GREEN')

  const env = (name, policy) => ({ name, deployment_branch_policy: policy })
  cell('environment with a policy', () => surfaceB([env('a', { custom: true }), env('copilot', null)]), 'GREEN')
  cell('environment without a policy', () => surfaceB([env('a', null), env('copilot', null)]), 'RED')
  cell('allowlisted env that grew a policy is stale', () =>
    surfaceB([env('a', { custom: true }), env('copilot', { custom: true })]), 'RED')
  cell('allowlisted env that no longer exists', () => surfaceB([env('a', { custom: true })]), 'RED')

  // Location and rule id (resonans): every surface-A finding names file:line and its rule.
  cell('a finding names file:line and its rule id', () => {
    const v = surfaceA(wf(bare), {})
    if (!v[0]?.startsWith("t.yml:4 SH-A1 job 'a' consumes secrets.MY_KEY")) {
      throw new Instrument(`wrong location or rule: ${v[0]}`)
    }
    return v
  }, 'RED')
  cell('a folded expression is located on its secrets line', () => {
    const v = surfaceA(wf('jobs:\n  a:\n    env:\n      K: ${{\n        secrets.LEAKY_KEY }}\n'), {})
    if (!v[0]?.startsWith('t.yml:5 SH-A1')) throw new Instrument(`wrong line: ${v[0]}`)
    return v
  }, 'RED')

  // Exemptions (resonans): one finding, a task, an owner, at most MAX_EXEMPTION_DAYS.
  const ex = (o = {}) => ({
    file: 't.yml', job: 'a', rule: 'SH-A1', secret: 'MY_KEY',
    task: 'T1', owner: 'O', added: '2026-01-01', expires: '2026-01-15', ...o,
  })
  const exempt = (body, exemptions, today) =>
    applyExemptions(findingsA(wf(body), {}), exemptions, today).violations.map((f) => f.text)
  const twoKeys = 'jobs:\n  a:\n    env:\n      K: ${{ secrets.MY_KEY }}\n      L: ${{ secrets.OTHER }}\n'

  cell('an active exemption suppresses its finding', () => exempt(bare, [ex()], '2026-01-10'), 'GREEN')
  cell('an exemption holds on its expiry day', () => exempt(bare, [ex()], '2026-01-15'), 'GREEN')
  cell('an expired exemption is red, neutral, and names the task', () => {
    const v = exempt(bare, [ex()], '2026-01-16')
    if (v.length !== 1 || v[0] !== 't.yml:4 SH-X1 SH-A1 secrets.MY_KEY — tracked in ClickUp T1, expired 2026-01-15') {
      throw new Instrument(`got ${JSON.stringify(v)}`)
    }
    return v
  }, 'RED')
  cell('an exemption covers only its own secret', () => {
    const v = exempt(twoKeys, [ex()], '2026-01-10')
    if (v.length !== 1 || !v[0].includes('secrets.OTHER')) throw new Instrument(`got ${JSON.stringify(v)}`)
    return v
  }, 'RED')
  cell('an exemption covers only its own job', () => exempt(bare, [ex({ job: 'b' })], '2026-01-10'), 'RED')
  cell('an exemption that matches no finding is stale', () => {
    const v = exempt(twoKeys, [ex(), ex({ secret: 'GONE' })], '2026-01-10')
    if (!v.some((x) => x.startsWith('SH-X2'))) throw new Instrument(`got ${JSON.stringify(v)}`)
    return v
  }, 'RED')
  cell('an exemption longer than the maximum suppresses nothing', () => {
    const v = exempt(bare, [ex({ expires: '2026-01-16' })], '2026-01-10')
    if (v.length !== 2 || !v.some((x) => x.startsWith('SH-X3'))) throw new Instrument(`got ${JSON.stringify(v)}`)
    return v
  }, 'RED')
  cell('an exemption without a task suppresses nothing', () => exempt(bare, [ex({ task: ' ' })], '2026-01-10'), 'RED')
  cell('an exemption with an impossible date suppresses nothing', () =>
    exempt(bare, [ex({ expires: '2026-02-30' })], '2026-01-10'), 'RED')
  // The live list must be well-formed and must still match the repo's own workflows.
  cell("the repo's OWN exemptions are well-formed and each matches a finding", () => {
    const v = applyExemptions(findingsA(readWorkflows()), EXEMPTIONS, '2000-01-01').violations
    return v.filter((f) => f.rule === 'SH-X2' || f.rule === 'SH-X3').map((f) => f.text)
  }, 'GREEN')

  return cells
}

/**
 * @param {{fn: () => string[]}} c
 * @returns {string} 'RED' | 'GREEN' | 'INSTRUMENT' | 'THREW …'
 */
export function runCell(c) {
  try {
    return c.fn().length > 0 ? 'RED' : 'GREEN'
  } catch (e) {
    return e instanceof Instrument ? 'INSTRUMENT' : `THREW ${e}`
  }
}
