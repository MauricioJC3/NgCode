import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    // Pure-logic unit tests only (no DOM/CodeMirror rendering is exercised
    // by this suite) — 'node' avoids pulling in a jsdom dependency for
    // tests that don't need one. Testable frontend logic is intentionally
    // extracted into small dependency-injected modules (see
    // src/update.ts) precisely so it never needs a real DOM to test.
    environment: 'node',
    include: ['src/**/*.test.ts'],
  },
});
