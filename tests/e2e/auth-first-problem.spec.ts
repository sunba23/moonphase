import { test, expect } from '@playwright/test';

/**
 * Protects: test-plan.md §2 Risk #5 (auth / onboarding gate) together with the
 * cross-boundary chain a first session depends on — Supabase Auth sign-up →
 * cookie session → chi routing → OnboardingGate → recommender.FirstPick →
 * Postgres catalog read → rendered problem card.
 *
 * The failure this test catches: a brand-new climber signs up and onboards but
 * never reaches an authenticated surface — the gate bounces them to /signin, the
 * session never starts, or no problem is recommended. If any link in the chain
 * breaks, `waitForURL('**\/session/...')` times out or the problem card is
 * absent, and the test goes red.
 *
 * This spec logs in through the UI on purpose (contrary to the usual
 * storageState rule): the sign-up + onboarding flow IS the risk under test.
 */

const SUPABASE_URL = process.env.SUPABASE_URL;
const SUPABASE_SERVICE_KEY = process.env.SUPABASE_SECRET_KEY;

// Best-effort teardown: deleting the auth.users row cascades to the profile and
// session rows this test created (ON DELETE CASCADE in migrations 0006/0007).
async function deleteSupabaseUser(userId: string): Promise<void> {
  if (!SUPABASE_URL || !SUPABASE_SERVICE_KEY) {
    throw new Error(
      'cannot clean up: SUPABASE_URL / SUPABASE_SECRET_KEY not in env (.env)',
    );
  }
  const res = await fetch(`${SUPABASE_URL}/auth/v1/admin/users/${userId}`, {
    method: 'DELETE',
    headers: {
      apikey: SUPABASE_SERVICE_KEY,
      Authorization: `Bearer ${SUPABASE_SERVICE_KEY}`,
    },
  });
  if (!res.ok && res.status !== 404) {
    throw new Error(`admin delete user ${userId}: ${res.status} ${await res.text()}`);
  }
}

let createdUserId: string | null = null;

test.afterEach(async () => {
  if (createdUserId) {
    await deleteSupabaseUser(createdUserId);
    createdUserId = null;
  }
});

test('a new account can sign up, onboard, and get a first problem recommended', async ({ page }) => {
  const stamp = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
  const email = `moonphase-e2e+${stamp}@example.com`;
  const password = `E2e-${stamp}-Pw`;

  // --- Sign up ---
  await page.goto('/signup');
  await page.getByLabel('Email').fill(email);
  await page.getByLabel('Password').fill(password);
  await page.getByRole('button', { name: 'Sign up' }).click();

  // A fresh account is HX-redirected straight to onboarding.
  await page.waitForURL('**/onboarding');

  // --- Onboard: declare max grade, board, and angle ---
  await page.getByLabel('Max grade').selectOption('6A');
  await page.getByLabel('Board').selectOption('1'); // holdsetup 1 = 2016
  await page.getByLabel('Angle').selectOption('40');
  await page.getByRole('button', { name: 'Continue' }).click();

  // Onboarding HX-redirects to the hub. The "Main Session" button being visible
  // is the real signal that the gate let the now-onboarded user through.
  const startSession = page.getByRole('button', { name: 'Main Session' });
  await expect(startSession).toBeVisible();

  // Capture the user id now (via the authenticated API, reusing the browser
  // cookies) so afterEach can tear the account down even if the rest fails.
  const me = await page.request.get('/api/me');
  expect(me.ok()).toBeTruthy();
  createdUserId = (await me.json()).user_id as string;
  expect(createdUserId).toBeTruthy();

  // --- Start the session and receive the first recommendation ---
  await startSession.click();
  await page.waitForURL(/\/session\/[0-9a-f-]{36}$/);

  // The first recommended problem is on screen: its name (heading), its catalog
  // detail line reflecting the board + angle chosen at onboarding, and its
  // per-hold breakdown.
  await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
  await expect(page.getByRole('heading', { level: 1 })).not.toBeEmpty();
  await expect(page.getByText(/40°\s*·\s*2016/)).toBeVisible();
  await expect(page.getByRole('listitem').first()).toBeVisible();
});
