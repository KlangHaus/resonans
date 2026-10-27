// Unit tests for code that must behave like a browser (the editor's sanitizer).
// jsdom parses HTML with parse5, which implements the same HTML-standard parsing
// algorithm browsers do — the property a sanitizer test depends on.
import { defineConfig } from 'vitest/config'

export default defineConfig({
  test: {
    environment: 'jsdom',
    include: ['app/**/*.test.ts'],
    // `pnpm test:coverage` writes coverage/lcov.info for SonarQube
    // (sonar-project.properties). Only the editor's own code is measured:
    // Nuxt's generated .nuxt/ and the tests themselves are not ours to cover.
    coverage: {
      provider: 'v8',
      reporter: ['lcov', 'text-summary'],
      reportsDirectory: 'coverage',
      include: ['app/**/*.ts', 'server/**/*.ts'],
      exclude: ['**/*.test.ts', '**/*.d.ts'],
    },
  },
})
