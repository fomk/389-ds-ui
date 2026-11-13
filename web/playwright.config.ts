/**
 * End-to-end tests against a running ds-ui (make up, or the CI stack).
 *   E2E_BASE_URL   default http://127.0.0.1:8080
 *   E2E_PASSWORD   Directory Manager password; default: ../secrets/dm_password
 * The tests change the directory and restore what they change; run them
 * against a development or CI server only.
 */
import { defineConfig, devices } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  // One server: tests change shared settings, so they run one after another.
  workers: 1,
  fullyParallel: false,
  retries: process.env.CI ? 1 : 0,
  timeout: 60_000,
  expect: { timeout: 10_000 },
  reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : 'list',
  use: {
    baseURL: process.env.E2E_BASE_URL ?? 'http://127.0.0.1:8080',
    locale: 'en-US',
    actionTimeout: 15_000,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'], viewport: { width: 1400, height: 900 } } }],
})
