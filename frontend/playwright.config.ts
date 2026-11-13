import { defineConfig, devices } from '@playwright/test';

/**
 * Two servers, both real:
 *  - the Go API with LLM_MOCK=true, because the audit spec exists to catch
 *    contract drift between the server's JSON and the Zod schema. Fixtures
 *    would let that bug pass.
 *  - the built SPA via `vite preview`, so the tests exercise the production
 *    bundle and its CSP rather than the dev server.
 */
export default defineConfig({
  testDir: './tests',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  // The API rate limiter (a few audits per minute per client) can refuse the
  // last audits of a fast run; one retry lets its Retry-After window pass.
  retries: 1,
  workers: process.env.CI ? 1 : undefined,
  reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : 'list',
  use: {
    baseURL: 'http://localhost:4173',
    trace: 'on-first-retry'
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  webServer: [
    {
      command: 'go run ./cmd/server',
      cwd: '../backend',
      url: 'http://localhost:8080/healthz',
      reuseExistingServer: !process.env.CI,
      timeout: 120_000,
      // The domain-age lookup would call real registries; it is tested
      // against a fake one in Go, so e2e stays offline.
      env: { SERVER_PORT: '8080', LLM_MOCK: 'true', ENVIRONMENT: 'development', DOMAIN_AGE_LOOKUP: 'false' }
    },
    {
      command: 'npm run preview -- --port 4173 --strictPort',
      url: 'http://localhost:4173',
      reuseExistingServer: !process.env.CI,
      timeout: 120_000,
      // The built SPA calls the same-origin /api/v1; preview forwards it to the Go API above.
      env: { E2E_API_PROXY: 'http://localhost:8080' }
    }
  ]
});
