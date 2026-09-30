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
import fs from 'node:fs';
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

// Every package whose version can change what the release publishes. [sikkerhed]
// measured that listing only the two rendering packages left the three release-tool
// pins unguarded, so a drift on those was invisible: two names were the whole
// denominator. Five now.
const PINNED_PACKAGES = [
  '@semantic-release/release-notes-generator',
  'conventional-changelog-angular',
  'semantic-release',
  '@semantic-release/changelog',
  '@semantic-release/git',
];

// 🔴 NAME THE TREE, AND ASSERT IT IS THE ONE THE RELEASE USES.
// [sikkerhed] measured that this guard read the repo's pinned packages while the
// release step ran `npx -p semantic-release@24 …`, which built its own prefix and
// resolved its own ranges at run time. The guard's green looked like it covered the
// release and did not. The workflow drops `-p` and pins all three exactly.
//
// 🔴 [sikkerhed] measured that an earlier version of this comment was HALF FALSE. It
// claimed this block catches "a reintroduced `-p` or a drifted pin". The drift half
// holds. The `-p` half did NOT: this block compares node_modules against
// package.json, so a reintroduced `npx -p semantic-release@24 …` would leave it
// reading the repo's tree, finding a match, and going GREEN — while the release ran
// from npx's own prefix again. The CHOICE of tree lives in the workflow YAML, not in
// node_modules, so no amount of version comparison can see it.
//
// Rather than narrow the sentence, the guard checks the workflow too, below. A
// comment claiming a guard covers something it cannot is the exact defect this whole
// change exists to remove.
const pinned =
  JSON.parse(fs.readFileSync(path.resolve(process.cwd(), 'package.json'), 'utf8')).devDependencies ?? {};
const resolved = {};
for (const name of PINNED_PACKAGES) {
  // Read node_modules directly rather than through `require`: these packages
  // declare `exports` without './package.json', so a resolver cannot reach it. And
  // node_modules is the object this block is about — the tree the release will load
  // from, not whatever a resolver would pick.
  const file = path.resolve(process.cwd(), 'node_modules', name, 'package.json');
  try {
    resolved[name] = JSON.parse(fs.readFileSync(file, 'utf8')).version;
  } catch (err) {
    console.error(
      `GUARD CANNOT MEASURE: cannot read ${file} — ${err.message}. Either \`npm ci\` did not ` +
        'run, or this package is not installed in the repo tree, which is exactly the ' +
        'condition that made this guard measure a different tree than the release.',
    );
    process.exit(2);
  }
}
let driftCount = 0;
for (const [name, version] of Object.entries(resolved)) {
  const want = pinned[name];
  if (!want) {
    console.error(
      `GUARD CANNOT MEASURE: ${name} resolves to ${version} but package.json pins nothing ` +
        'for it, so this guard cannot say which tree it just measured.',
    );
    process.exit(2);
  }
  const ok = want === version;
  if (!ok) driftCount++;
  console.log(`  ${ok ? 'ok  ' : 'FAIL'}  ${name} resolved ${version}, pinned ${want}`);
}
if (driftCount > 0) {
  console.error(
    `\n${driftCount} package(s) resolve to something other than the exact pin. The notes this ` +
      'guard renders come from a different tree than the release will, so a green here ' +
      'would not transfer.',
  );
  process.exit(1);
}

// The tree CHOICE, measured where it actually lives. `npx -p <pkg>` builds its own
// prefix and resolves its own ranges at run time, so the release would render notes
// with packages this guard never saw. Both directions are asserted: a pinned
// invocation must be present, and no `-p` form may be.
//
// A text scan with comments stripped, not a YAML parse — stated as the limit rather
// than left implied. It is enough because the only thing matched is the shape of one
// command line, and adding a YAML parser to read one line would grow the very surface
// this change pinned down.
// foldContinuations joins a block scalar's continuation lines into one logical line,
// keyed on indentation, then trims.
//
// 🔴 WHY THIS EXISTS. [sikkerhed] measured that matching line by line missed the form
// this file actually uses. YAML folded scalars are its native idiom — five today — and
// the invocation this change removes was written folded, with the option on its own
// continuation line:
//
//     run: >
//       npx --no-install semantic-release
//       --package @semantic-release/git@10
//
// A line-bound matcher sees the anchor on one line and the option on the next, and
// passes. My own register says to fold continuations FIRST and I wrote a line-bound
// matcher anyway.
//
// Folding is done rather than applying the option check to every line, because that
// variant produced a FALSE POSITIVE on this repo's own workflow: `node -p "..."` is
// Node's --print, not npx's --package. Broadening a denial needs the accept set rerun,
// which is how that surfaced.
//
// Not a YAML parse, and the limit is stated: a `|`/`>` block is folded by indentation,
// and a more-indented line that is NOT a continuation would be joined too. That can
// only merge text into one logical line, which makes the option check see MORE, never
// less.
function foldContinuations(rawLines) {
  const out = [];
  let buf = null;
  let blockIndent = null;
  for (const raw of rawLines) {
    if (!raw.trim()) continue;
    const indent = raw.length - raw.trimStart().length;
    if (blockIndent !== null && indent > blockIndent) {
      buf = `${buf} ${raw.trim()}`;
      continue;
    }
    if (buf !== null) {
      out.push(buf.trim());
      buf = null;
      blockIndent = null;
    }
    if (/:\s*[>|][-+]?\d*\s*$/.test(raw)) {
      buf = raw.replace(/:\s*[>|][-+]?\d*\s*$/, ': ').trim();
      blockIndent = indent;
      continue;
    }
    out.push(raw.trim());
  }
  if (buf !== null) out.push(buf.trim());
  return out.filter(Boolean);
}

const WORKFLOW = path.resolve(process.cwd(), '.github', 'workflows', 'semantic-release.yml');
let wfLines;
try {
  wfLines = foldContinuations(
    fs
      .readFileSync(WORKFLOW, 'utf8')
      .split('\n')
      // A YAML comment is `#` at line start or preceded by whitespace. An earlier
      // form cut at ANY `#`, which could hide a match rather than only a comment —
      // [sikkerhed] noted that is a different sentence than "comments are stripped".
      // Still not a YAML parse; a `#` inside a quoted string preceded by a space is
      // the residual, and it can only make this stricter by truncating a line early,
      // never more permissive, because the anchor below requires a positive form.
      .map((l) => l.replace(/(^|\s)#.*$/, '')),
  );
} catch (err) {
  console.error(
    `GUARD CANNOT MEASURE: cannot read ${WORKFLOW} — ${err.message}. This guard has to see ` +
      'how the release invokes semantic-release; without it, it can only speak for the tree ' +
      'it happens to read.',
  );
  process.exit(2);
}

const invocations = wfLines.filter((l) => l.includes('npx') && l.includes('semantic-release'));
if (invocations.length === 0) {
  console.error(
    `GUARD CANNOT MEASURE: found no \`npx … semantic-release\` line in ${WORKFLOW}. Either the ` +
      'release is invoked some other way now — in which case this check is looking at the wrong ' +
      'thing — or the step is gone. Both are failures, not a pass.',
  );
  process.exit(2);
}
// 🔴 AN ALLOWLIST, NOT A LIST OF FORBIDDEN SPELLINGS.
//
// The first version of this check rejected `/(^|\s)-p\s/`. [sikkerhed] measured that
// it caught ONE spelling of at least four: `--package <spec>` is npx's long form for
// `-p` with identical effect, and `--package=<spec>` and `-p=<spec>` also slipped,
// because the pattern demanded a space on both sides. Counting the denominator from
// `npx --help` rather than from memory turns up more than the four:
//
//   -p <spec>            -p=<spec>            --package <spec>
//   --package=<spec>     npx <pkg>@<version>  (a specifier on the command name)
//   -c / --call '<cmd>'  (an arbitrary embedded command)
//
// So enumerating the unsafe forms loses to the next one nobody thought of — the same
// shape as a pattern covering `*.yml` but not `*.yaml`. A port opens only for a
// positively declared safe value, so this asserts the invocation IS the pinned form
// and rejects everything else, including spellings that do not exist yet.
// Two conditions, because the property is "resolution comes from node_modules" and
// NOT "the line is exactly this string". An exact-string allowlist rejected a
// legitimate `--dry-run`, which is the self-camouflaging direction: a guard that
// refuses what it should let in looks like work. So the accept set was run too, not
// only the reject set.
//
//   ANCHOR  an invocation line must START with the pinned form, so trailing flags
//           on that line are fine
//   DENY    no line in the whole workflow may carry a package-selecting option
//
// 🔴 The DENY half is applied to EVERY line, not to the invocation lines, and that is
// [sikkerhed]'s third finding on this guard rather than a choice. YAML folded scalars
// (`run: >`) are this file's native idiom — it uses five today — and the very form
// this change removes was written folded, with `-p` on its own continuation lines:
//
//     run: >
//       npx --no-install semantic-release
//       --package @semantic-release/git@10
//
// Line-bound matching sees the anchor on one line and the option on the next, so it
// PASSED. An earlier version of this comment said "no option … anywhere", which was
// not true of the code: it was "on the invocation line". Third time in this change
// that a comment claimed wider coverage than the measurement — the exact defect the
// whole change exists to remove.
//
// Applying DENY to every line rather than folding continuations first, because no
// legitimate line in this workflow needs `-p`, `--package` or `-c`, and comments are
// already stripped — verified as behaviour, since the file still carries
// `-p semantic-release@24` inside a comment and the guard passes.
const SAFE_PREFIX = /^npx --no-install semantic-release(\s|$)/;
const PACKAGE_SELECTING = /(^|\s)(-p|--package|-c|--call)([\s=])/;
// Applied to the FOLDED invocation lines, so an option on a continuation is now part
// of the same logical line the anchor is on.
const unsafe = invocations
  .map((l) => l.replace(/^run:\s*/, '').trim())
  .filter((l) => !SAFE_PREFIX.test(l) || PACKAGE_SELECTING.test(l));
if (unsafe.length > 0) {
  console.error(
    'FAIL  the release does not invoke semantic-release in the pinned form:\n' +
      unsafe.map((l) => `        ${l}`).join('\n') +
      '\n\nExpected exactly `npx --no-install semantic-release`. Any of `-p`, `-p=`, ' +
      '`--package`, `--package=`, a version specifier on the command name, or `-c/--call` ' +
      'makes npx build its own prefix and resolve its own ranges at run time — so the notes ' +
      'this guard renders would come from a different tree than the release uses, and every ' +
      'assertion below would be about the wrong object. Keep the packages as exact ' +
      'devDependencies and let node_modules answer.',
  );
  process.exit(1);
}
console.log(
  `  ok    the release resolves from node_modules (${invocations.length} npx invocation(s) in the ` +
    `pinned form, folded from ${wfLines.length} logical workflow lines)`,
);

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
console.log(
  `\nall ${checks.length} assertions hold against release.config.mjs (${notes.length} bytes of notes), ` +
    `rendered by release-notes-generator ${resolved['@semantic-release/release-notes-generator']} ` +
    `and angular preset ${resolved['conventional-changelog-angular']}; ` +
    `${PINNED_PACKAGES.length} packages checked against their exact pins, and the release ` +
    'invocation is the pinned form',
);
