import { test, expect } from '@playwright/test';

/**
 * Seed exemplar — the shape every spec in this directory copies.
 *
 * Protects: test-plan.md §2 Risk #5 (the auth/onboarding gate must be mounted on
 * private routes; a logged-out request must never reach one).
 *
 * Patterns it demonstrates:
 *  - role/URL-based locators, never CSS
 *  - wait for state (`waitForURL`, `toBeVisible`), never `waitForTimeout`
 *  - a name that binds the test to its risk
 *  - self-contained: no shared state, and nothing to clean up (read-only flow)
 */
test('unauthenticated visit to the hub redirects to sign-in', async ({ page }) => {
  await page.goto('/');

  await page.waitForURL('**/signin');
  await expect(page.getByRole('button', { name: 'Sign in' })).toBeVisible();
});
