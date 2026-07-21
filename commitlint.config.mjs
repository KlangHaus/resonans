// Conventional Commits — matches the KlangHaus org convention.
export default {
  extends: ['@commitlint/config-conventional'],
  ignores: [(message) => /Signed-off-by: dependabot\[bot\]/i.test(message)],
  rules: {
    // Proper nouns/acronyms (OpenTelemetry, JSON, REST, ...) are common in
    // subjects for a technical project; disable subject-case, keep the rest.
    'subject-case': [0],
  },
};
