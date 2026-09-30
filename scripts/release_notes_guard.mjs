// Fail-closed guard for the release notes an operator actually reads.
//
// It loads release.config.mjs — the REAL config, not a copy of its settings — and
// generates notes from synthetic commits, so a change to that file cannot pass this
// guard while breaking the published surface.
//
// Why it is fail-closed and runs BEFORE semantic-release: an action written in a
// commit body was dropped from every published surface, and nothing went red. A
// security note that does not reach the release page is not delivered, however well
// it reads in git. This guard turns that silence into a red step.
//
//   node scripts/release_notes_guard.mjs
//
// Exit 0 pass, 1 an assertion failed, 2 the guard could not measure (which is also
// a failure — a guard that cannot see must not report a clean sweep).
import { generateNotes } from '@semantic-release/release-notes-generator';
import { pathToFileURL } from 'node:url';
import path from 'node:path';

const ACTION = 'ACTION REQUIRED';
const cfgPath = path.resolve(process.cwd(), 'release.config.mjs');

let notesOpts;
try {
  const cfg = (await import(pathToFileURL(cfgPath).href)).default;
  const entry = cfg.plugins.find(
    (p) => Array.isArray(p) && p[0] === '@semantic-release/release-notes-generator',
  );
  if (!entry) {
    console.error(
      `GUARD CANNOT MEASURE: ${cfgPath} has no configured ` +
        '@semantic-release/release-notes-generator entry. Either the plugin lost its ' +
        'options (in which case bodies are being dropped again) or this guard is ' +
        'reading the wrong shape. Both are failures, not a pass.',
    );
    process.exit(2);
  }
  notesOpts = entry[1];
} catch (err) {
  console.error(`GUARD CANNOT MEASURE: could not load ${cfgPath}: ${err.message}`);
  process.exit(2);
}

const ctx = {
  cwd: process.cwd(),
  options: { repositoryUrl: 'https://github.com/KlangHaus/resonans' },
  lastRelease: { gitTag: 'v0.0.0' },
  nextRelease: { gitTag: 'v0.0.1', version: '0.0.1' },
  commits: [
    {
      hash: 'a'.repeat(40),
      subject: 'fix(probe): CANARY_SUBJECT',
      message:
        `fix(probe): CANARY_SUBJECT\n\n${ACTION}: CANARY_ACTION_TEXT that an operator must do.\n`,
      committerDate: '2026-01-01',
    },
    {
      hash: 'b'.repeat(40),
      subject: 'feat(probe): CANARY_BREAKING_SUBJECT',
      message:
        'feat(probe): CANARY_BREAKING_SUBJECT\n\nBREAKING CHANGE: CANARY_BREAKING_TEXT.\n',
      committerDate: '2026-01-01',
    },
    {
      hash: 'c'.repeat(40),
      subject: 'fix(probe): CANARY_PLAIN_SUBJECT',
      message: 'fix(probe): CANARY_PLAIN_SUBJECT\n\nCANARY_PLAIN_BODY, an ordinary body line.\n',
      committerDate: '2026-01-01',
    },
  ],
};

const notes = await generateNotes(notesOpts, ctx);

// The guard's own denominator. Empty or trivially short output means the generator
// answered about nothing, which is not the same as the assertions holding.
if (!notes || notes.length < 80) {
  console.error(
    `GUARD CANNOT MEASURE: generated notes are ${notes ? notes.length : 0} bytes — ` +
      'too short to contain the three probe commits, so every assertion below would ' +
      'have been vacuous.',
  );
  process.exit(2);
}

const checks = [
  // The thing this guard exists for.
  [`"### ${ACTION}" is its own heading`, () => notes.includes(`### ${ACTION}`), true],
  ['the action text is published', () => notes.includes('CANARY_ACTION_TEXT'), true],
  // Positive control: ordinary rendering still works, so a pass above is not an
  // artefact of the generator emitting everything.
  ['subjects still render', () => notes.includes('CANARY_SUBJECT'), true],
  // Regression this change introduced once and removed: the preset's plural heading.
  ['"### BREAKING CHANGES" stays plural', () => notes.includes('### BREAKING CHANGES'), true],
  ['a real breaking change still publishes', () => notes.includes('CANARY_BREAKING_TEXT'), true],
  // Negative control: this must NOT start dumping every commit body onto the
  // release page. A plain body line is not a note and must stay unpublished.
  ['a plain body line is NOT published', () => notes.includes('CANARY_PLAIN_BODY'), false],
];

let failed = 0;
for (const [label, fn, want] of checks) {
  const got = fn();
  const ok = got === want;
  if (!ok) failed++;
  console.log(`  ${ok ? 'ok  ' : 'FAIL'}  ${label}${ok ? '' : `  (wanted ${want}, got ${got})`}`);
}

if (failed > 0) {
  console.error(
    `\n${failed} of ${checks.length} release-note assertions failed. A release published ` +
      'now would carry the fix and not the instruction — which is how the password-in-the-log ' +
      'action reached zero published surfaces. Fix release.config.mjs before releasing.',
  );
  console.error('\n--- generated notes ---\n' + notes);
  process.exit(1);
}
console.log(`\nall ${checks.length} assertions hold against release.config.mjs (${notes.length} bytes of notes)`);
