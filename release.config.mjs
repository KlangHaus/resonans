// semantic-release configuration.
//
// This was .releaserc.json until 2026-09-30. It is JavaScript now for exactly one
// reason: the notes generator needs a `writerOpts.transform`, which JSON cannot
// hold. Everything else is unchanged from the JSON, field for field.
//
// 🔴 WHY: the default preset renders ONLY the subject line, so a commit body is
// dropped from every published surface. v1.18.3 fixed a password written in
// cleartext to the postgres container log, and the action an operator had to take
// — recreate the container, because upgrading the image does not remove the value
// from an existing log — was in the commit body. Measured on the published
// artefacts: 0 occurrences on the GitHub release note, 0 in CHANGELOG.md, 1 in the
// git commit. The one surface that carried it requires a clone and a `git log` of
// a specific commit. A reader of the release note saw "stop echoing the app
// password" and would reasonably conclude "fixed, upgrade and carry on", which is
// the inference the sentence existed to prevent.
//
// So: an `ACTION REQUIRED:` footer now renders as its own section on the release
// note and in CHANGELOG.md. Write it as a conventional-commit FOOTER — the token,
// a colon, then the text — because a plain sentence in the body is not a note and
// is still dropped:
//
//   ACTION REQUIRED: recreate your postgres container with `docker compose down
//   -v` and a new password. Upgrading the image does not remove it.
//
// MEASURED, with @semantic-release/release-notes-generator@14 and
// conventional-changelog-angular@8, two synthetic commits in one run:
//
//   no config (what we shipped)   ACTION REQUIRED absent, body-only canary absent
//   noteKeywords alone            text present, but under the heading
//                                 "BREAKING CHANGES" — a false label
//   + this transform              "### ACTION REQUIRED" its own heading, and
//                                 "### BREAKING CHANGES" (plural) still correct
//                                 for a real breaking change
//
// The `!== 'BREAKING CHANGE'` guard below is not cosmetic: without it the preset's
// plural heading became singular, which is a regression this change introduced and
// then removed. The negative control matters too — a plain body sentence is STILL
// dropped, so this does not quietly start publishing every commit body.
import angular from 'conventional-changelog-angular';

const preset = await angular();
const presetTransform = preset.writer.transform;

export default {
  branches: ['main'],
  plugins: [
    '@semantic-release/commit-analyzer',
    [
      '@semantic-release/release-notes-generator',
      {
        parserOpts: {
          // BREAKING CHANGE must stay first and stay listed: replacing the array
          // rather than appending to it would stop breaking changes from being
          // recognised at all, which is a silent loss.
          noteKeywords: ['BREAKING CHANGE', 'BREAKING-CHANGE', 'ACTION REQUIRED'],
        },
        writerOpts: {
          transform(commit, context) {
            // The preset rewrites EVERY note title to 'BREAKING CHANGES'. Keep each
            // note's own token so a non-breaking operator action is not published
            // under a heading that says the release breaks something.
            const tokens = (commit.notes || []).map((n) => n.title);
            const out = presetTransform.call(this, commit, context);
            if (out && out.notes) {
              out.notes.forEach((n, i) => {
                if (tokens[i] && tokens[i] !== 'BREAKING CHANGE' && tokens[i] !== 'BREAKING-CHANGE') {
                  n.title = tokens[i];
                }
              });
            }
            return out;
          },
        },
      },
    ],
    ['@semantic-release/changelog', { changelogFile: 'CHANGELOG.md' }],
    '@semantic-release/github',
    [
      '@semantic-release/git',
      {
        assets: ['CHANGELOG.md'],
        message: 'chore(release): ${nextRelease.version} [skip ci]\n\n${nextRelease.notes}',
      },
    ],
  ],
};
