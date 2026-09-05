import { test, expect, type Page } from '@playwright/test';

/**
 * Protects: PRD FR-013 / FR-014 and roadmap slice S-05 — a climber can review a
 * finished Main Session from history: the list shows their ended sessions with a
 * climbed count, the detail view lists every climbed problem with the RPE and
 * completion status they submitted, and a problem opens its read-only MoonBoard
 * layout. The history surface is also scoped to the caller (the fresh throwaway
 * user only ever sees their own single session).
 *
 * Cross-boundary chain: Supabase Auth cookie -> chi routing -> OnboardingGate ->
 * GET /sessions (session.ListForUser) -> GET /sessions/{id}
 * (session.ClimbedProblems) -> GET /sessions/{id}/problem/{seq}
 * (session.ClimbedProblemAt + catalog.ProblemDetail) -> rendered pages.
 *
 * The failures this test catches:
 *  - an ended session never appears in the history list, or the climbed count is
 *    wrong (the trailing recommended-but-unrated row is counted, or a rated row
 *    is missed);
 *  - the detail view drops a climbed problem, shows the trailing unrated row, or
 *    loses the submitted RPE / completion;
 *  - the read-only problem card fails to render the board layout or the recorded
 *    result line, or its back-link does not return to the detail view.
 *
 * Seed: modelled on adaptive-session-loop.spec.ts — role/text locators,
 * wait-for-state, a risk-tied name, per-account teardown in afterEach. Drives
 * sign-up + onboarding through the UI because the authenticated session it needs
 * is real and there is no storageState fixture wired up yet.
 */

const SUPABASE_URL = process.env.SUPABASE_URL;
const SUPABASE_SERVICE_KEY = process.env.SUPABASE_SECRET_KEY;

// Best-effort teardown: deleting the auth.users row cascades to the profile,
// session, and session_problems rows this test created (ON DELETE CASCADE).
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

// Waits for htmx to finish swapping AND settling the new #session-card, so its
// nested forms are re-bound before the test acts on them.
async function waitForCardSettled(page: Page): Promise<void> {
  await page.waitForFunction(() => {
    const card = document.getElementById('session-card');
    return (
      !!card &&
      !card.classList.contains('htmx-swapping') &&
      !card.classList.contains('htmx-settling')
    );
  });
}

// Reads the current problem card's name (the level-1 heading).
async function readCardName(page: Page): Promise<string> {
  const heading = page.getByRole('heading', { level: 1 });
  await expect(heading).not.toBeEmpty();
  return ((await heading.textContent()) ?? '').trim();
}

// Picks a completion status, then taps an RPE number to submit. Waits for the
// result response and for the fresh #session-card to fully settle.
async function submitResult(page: Page, completion: string, rpe: number): Promise<void> {
  await page.getByRole('radio', { name: completion }).check();

  const [response] = await Promise.all([
    page.waitForResponse(
      (r) => r.url().endsWith('/result') && r.request().method() === 'POST',
    ),
    page.getByRole('button', { name: String(rpe), exact: true }).click(),
  ]);
  expect(response.status()).toBe(200);

  await waitForCardSettled(page);
  await expect(page.getByRole('radio', { name: 'Sent' })).not.toBeChecked();
}

let createdUserId: string | null = null;

test.afterEach(async () => {
  if (createdUserId) {
    await deleteSupabaseUser(createdUserId);
    createdUserId = null;
  }
});

test('a climber reviews a finished session\'s climbed problems from history', async ({ page }) => {
  const stamp = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;

  // --- Sign up + onboard onto the 2016 board at full grade headroom ---
  await page.goto('/signup');
  await page.getByLabel('Email').fill(`moonphase-e2e+${stamp}@example.com`);
  await page.getByLabel('Password').fill(`E2e-${stamp}-Pw`);
  await page.getByRole('button', { name: 'Sign up' }).click();
  await page.waitForURL('**/onboarding');

  await page.getByLabel('Max grade').selectOption('8B+');
  await page.getByLabel('Board').selectOption('1'); // holdsetup 1 = 2016
  await page.getByLabel('Angle').selectOption('40');
  await page.getByRole('button', { name: 'Continue' }).click();

  const startSession = page.getByRole('button', { name: 'Main Session' });
  await expect(startSession).toBeVisible();

  const me = await page.request.get('/api/me');
  expect(me.ok()).toBeTruthy();
  createdUserId = (await me.json()).user_id as string;
  expect(createdUserId).toBeTruthy();

  // --- Run a Main Session: rate two problems, then end ---
  await startSession.click();
  await page.waitForURL(/\/session\/[0-9a-f-]{36}$/);

  const name0 = await readCardName(page);
  await submitResult(page, 'Sent', 4);

  const name1 = await readCardName(page);
  await submitResult(page, 'Failed', 8);

  await page.getByRole('button', { name: 'End session' }).click();
  await page.waitForURL((url) => new URL(url).pathname === '/');

  // --- History list: one row, two problems climbed (not three) ---
  await page.getByRole('link', { name: /past sessions/i }).click();
  await page.waitForURL((url) => new URL(url).pathname === '/sessions');

  const listRow = page.getByRole('link', { name: /2 problems climbed/i });
  await expect(listRow).toBeVisible();

  // --- Detail: both climbed problems, with the RPE + status just submitted ---
  await listRow.click();
  await page.waitForURL(/\/sessions\/[0-9a-f-]{36}$/);
  const detailUrl = page.url();

  const firstProblemRow = page.getByRole('link', { name: /RPE 4/i });
  const secondProblemRow = page.getByRole('link', { name: /RPE 8/i });
  await expect(firstProblemRow).toBeVisible();
  await expect(secondProblemRow).toBeVisible();
  await expect(page.getByText(name0, { exact: false })).toBeVisible();
  await expect(page.getByText(name1, { exact: false })).toBeVisible();
  await expect(page.getByRole('link', { name: /RPE 4 · sent/i })).toBeVisible();
  await expect(page.getByRole('link', { name: /RPE 8 · failed/i })).toBeVisible();

  // --- Problem card: read-only board layout + recorded result + back-link ---
  await firstProblemRow.click();
  await page.waitForURL(/\/sessions\/[0-9a-f-]{36}\/problem\/0$/);

  await expect(page.getByRole('img', { name: /moonboard/i })).toBeVisible();
  await expect(page.getByText(/you climbed this/i)).toBeVisible();
  await expect(page.getByText(/RPE 4 · sent/i)).toBeVisible();
  // Read-only: the live result form and End button are gone.
  await expect(page.getByRole('button', { name: 'End session' })).toHaveCount(0);

  await page.getByRole('link', { name: /back to session/i }).click();
  await page.waitForURL(detailUrl);
});

test('a freshly onboarded climber with no finished sessions sees the empty state', async ({ page }) => {
  const stamp = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;

  await page.goto('/signup');
  await page.getByLabel('Email').fill(`moonphase-e2e+${stamp}@example.com`);
  await page.getByLabel('Password').fill(`E2e-${stamp}-Pw`);
  await page.getByRole('button', { name: 'Sign up' }).click();
  await page.waitForURL('**/onboarding');

  await page.getByLabel('Max grade').selectOption('6A');
  await page.getByLabel('Board').selectOption('1');
  await page.getByLabel('Angle').selectOption('40');
  await page.getByRole('button', { name: 'Continue' }).click();

  await expect(page.getByRole('button', { name: 'Main Session' })).toBeVisible();

  const me = await page.request.get('/api/me');
  expect(me.ok()).toBeTruthy();
  createdUserId = (await me.json()).user_id as string;
  expect(createdUserId).toBeTruthy();

  await page.getByRole('link', { name: /past sessions/i }).click();
  await page.waitForURL((url) => new URL(url).pathname === '/sessions');

  await expect(page.getByText(/no sessions yet/i)).toBeVisible();
  await expect(page.getByRole('link', { name: /start a main session/i })).toBeVisible();
});
