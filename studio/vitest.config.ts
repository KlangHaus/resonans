// Unit tests for code that must behave like a browser (the editor's sanitizer).
// jsdom parses HTML with parse5, which implements the same HTML-standard parsing
// algorithm browsers do — the property a sanitizer test depends on.
import { defineConfig } from 'vitest/config'

export default defineConfig({
  test: {
    environment: 'jsdom',
    include: ['app/**/*.test.ts'],
  },
})
