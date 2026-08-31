import { defineConfig, devices } from '@playwright/test';
import * as dotenv from 'dotenv';

// The Go app reads its own config from .env via godotenv; the E2E tests read the
// same file for the Supabase admin key used to tear down the accounts they create.
dotenv.config({ path: '.env' });

const PORT = process.env.PORT ?? '2137';
const BASE_URL = process.env.E2E_BASE_URL ?? `http://localhost:${PORT}`;

export default defineConfig({
  testDir: './tests/e2e',
  // Each spec creates a real Supabase account against the shared project; keep
  // runs serial so a failure is never a parallel-signup race.
  workers: 1,
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [['github'], ['html', { open: 'never' }]] : 'list',
  timeout: 60_000,
  expect: { timeout: 10_000 },

  use: {
    baseURL: BASE_URL,
    trace: 'on-first-retry',
    // The product NFR: usable one-handed on a phone in portrait. Drive the flow
    // in that viewport so the tests exercise what the persona actually sees.
    ...devices['Pixel 7'],
  },

  projects: [{ name: 'chromium', use: { browserName: 'chromium' } }],

  // Start the real Go server (which self-loads .env) unless E2E_BASE_URL points
  // at an already-running instance.
  webServer: process.env.E2E_BASE_URL
    ? undefined
    : {
        command: 'go run ./cmd/server',
        url: `http://localhost:${PORT}/healthz`,
        reuseExistingServer: !process.env.CI,
        timeout: 120_000,
        stdout: 'ignore',
        stderr: 'pipe',
      },
});
